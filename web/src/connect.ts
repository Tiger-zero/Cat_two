import { timestampDate } from "@bufbuild/protobuf/wkt";
import { Code, ConnectError, createClient, type Interceptor } from "@connectrpc/connect";
import { createConnectTransport } from "@connectrpc/connect-web";
import {
  accessSessionID,
  adoptSharedAccessToken,
  endAccessSession,
  getAccessSessionToken,
  getAccessToken,
  hasStoredToken,
  isTokenExpired,
  mayRefreshAccessSession,
  REQUEST_TOKEN_EXPIRY_BUFFER_MS,
  setAccessToken,
} from "./auth-state";
import { hasIdleDeadlineElapsed, observeIdleDeadlineHeader, SESSION_IDLE_EXPIRED_MESSAGE } from "./lib/session-idle";
import { AttachmentService } from "./types/proto/api/attachment_service_pb";
import { AuthService } from "./types/proto/api/auth_service_pb";
import { InstanceService } from "./types/proto/api/instance_service_pb";
import { MemoService } from "./types/proto/api/memo_service_pb";
import { UserService } from "./types/proto/api/user_service_pb";
import { redirectOnAuthFailure } from "./utils/auth-redirect";
import { AUTH_REASON_IDLE_SESSION } from "./utils/redirect-safety";

interface RequestWithHeader {
  header: Headers;
}

// ============================================================================
// Constants
// ============================================================================

const RETRY_HEADER = "X-Retry";
const RETRY_HEADER_VALUE = "true";
const TOKEN_REFRESH_LOCK = "memos-token-refresh";

// ============================================================================
// Token Refresh State Management
// ============================================================================

const createTokenRefreshManager = () => {
  let isRefreshing = false;
  let refreshPromise: Promise<void> | null = null;

  return {
    async refresh(refreshFn: () => Promise<void>): Promise<void> {
      if (isRefreshing && refreshPromise) {
        return refreshPromise;
      }

      isRefreshing = true;
      refreshPromise = refreshFn().finally(() => {
        isRefreshing = false;
        refreshPromise = null;
      });

      return refreshPromise;
    },
  };
};

const tokenRefreshManager = createTokenRefreshManager();

// ============================================================================
// Token Refresh
// ============================================================================

const fetchWithCredentials: typeof globalThis.fetch = (input, init) => {
  return globalThis.fetch(input, {
    ...init,
    credentials: "include",
  });
};

// Separate transport without auth interceptor to prevent recursion
const refreshTransport = createConnectTransport({
  baseUrl: window.location.origin,
  useBinaryFormat: true,
  fetch: fetchWithCredentials,
  interceptors: [],
});

const refreshAuthClient = createClient(AuthService, refreshTransport);

async function doRefreshAccessToken(previousToken: string | null): Promise<void> {
  const sessionID = accessSessionID(previousToken);
  let headers: Headers | undefined;
  const response = await refreshAuthClient.refreshToken(
    {},
    {
      headers: sessionID ? { "x-memos-session-id": sessionID } : undefined,
      onHeader: (value) => {
        headers = value;
      },
    },
  );

  if (!response.accessToken) {
    throw new ConnectError("Refresh token response missing access token", Code.Internal);
  }
  const activeSession = accessSessionID(getAccessToken());
  if (!mayRefreshAccessSession() || (activeSession && activeSession !== accessSessionID(response.accessToken))) {
    throw new ConnectError("Session changed during refresh", Code.Unauthenticated);
  }

  const expiresAt = response.accessTokenExpireTime ? timestampDate(response.accessTokenExpireTime) : undefined;
  if (headers) observeIdleDeadlineHeader(response.accessToken, headers);
  setAccessToken(response.accessToken, expiresAt);
}

export function isSessionIdleExpiredError(error: unknown): boolean {
  return error instanceof ConnectError && error.code === Code.Unauthenticated && error.rawMessage === SESSION_IDLE_EXPIRED_MESSAGE;
}

// The manager deduplicates within this tab. Web Locks serialize rotation
// of the shared refresh cookie across tabs on supported origins.
export async function refreshAccessToken(previousToken = getAccessSessionToken()): Promise<void> {
  return tokenRefreshManager.refresh(async () => {
    const refresh = async () => {
      if (!mayRefreshAccessSession()) throw new ConnectError("Session signed out", Code.Unauthenticated);
      if (!adoptSharedAccessToken(previousToken)) {
        await doRefreshAccessToken(previousToken);
      }
    };
    try {
      if (navigator.locks) {
        await navigator.locks.request(TOKEN_REFRESH_LOCK, refresh);
      } else {
        await refresh();
      }
    } catch (error) {
      if (
        isSessionIdleExpiredError(error) ||
        (error instanceof ConnectError && error.code === Code.Unauthenticated && hasIdleDeadlineElapsed(previousToken))
      ) {
        if (previousToken) endAccessSession(previousToken, AUTH_REASON_IDLE_SESSION);
        throw new ConnectError(SESSION_IDLE_EXPIRED_MESSAGE, Code.Unauthenticated);
      }
      // LAN HTTP can lack Web Locks. A 401 may race a successful refresh in another tab.
      if (!navigator.locks && error instanceof ConnectError && error.code === Code.Unauthenticated && adoptSharedAccessToken(previousToken))
        return;
      throw error;
    }
  });
}

// ============================================================================
// Authentication Interceptor Helpers
// ============================================================================

function setAuthorizationHeader(req: RequestWithHeader, token: string | null) {
  if (!token) return;
  req.header.set("Authorization", `Bearer ${token}`);
}

function shouldHandleUnauthenticatedRetry(error: unknown, isRetryAttempt: boolean): boolean {
  if (!(error instanceof ConnectError)) {
    return false;
  }
  if (error.code !== Code.Unauthenticated) {
    return false;
  }
  if (isRetryAttempt) {
    return false;
  }
  return true;
}

async function refreshAndGetAccessToken(previousToken?: string | null): Promise<string> {
  await refreshAccessToken(previousToken);
  const token = getAccessToken();
  if (!token) {
    throw new ConnectError("Token refresh succeeded but no token available", Code.Internal);
  }
  return token;
}

export async function getRequestToken(): Promise<string | null> {
  let token = getAccessToken();
  if (!token) {
    if (!hasStoredToken()) return null;
    try {
      token = await refreshAndGetAccessToken(getAccessSessionToken());
    } catch {
      return null;
    }
    return token;
  }

  // Preflight refresh: avoid sending requests with expired access tokens.
  // This is especially important for public endpoints (e.g. ListMemos), where
  // an expired token could otherwise be treated as anonymous and return
  // guest-scoped data before the reactive 401 refresh path runs.
  if (isTokenExpired(REQUEST_TOKEN_EXPIRY_BUFFER_MS)) {
    try {
      token = await refreshAndGetAccessToken();
    } catch (error) {
      if (isSessionIdleExpiredError(error)) return null;
      // Keep existing reactive 401 flow as fallback.
      // Protected methods still trigger refresh/redirect in the catch block below.
    }
  }

  return token;
}

function handleRefreshFailure(error: unknown, previousToken: string | null): void {
  if (!(error instanceof ConnectError) || error.code !== Code.Unauthenticated || isSessionIdleExpiredError(error)) return;
  if (!mayRefreshAccessSession() || accessSessionID(getAccessSessionToken()) !== accessSessionID(previousToken)) return;
  redirectOnAuthFailure();
}

/** Makes a same-origin HTTP request using the API client's session and refresh handling. */
export async function authenticatedFetch(path: string, init: RequestInit = {}, expectedSession?: string): Promise<Response> {
  const url = new URL(path, window.location.origin);
  if (url.origin !== window.location.origin) throw new Error("Authenticated requests must use the current origin");

  const send = (token: string | null) => {
    if (expectedSession && accessSessionID(token) !== expectedSession) {
      throw new ConnectError("Session changed before request", Code.Canceled);
    }
    const header = new Headers(init.headers);
    setAuthorizationHeader({ header }, token);
    return fetchWithCredentials(url, { ...init, headers: header, redirect: "error" });
  };
  const previousToken = await getRequestToken();
  const response = await send(previousToken);
  if (response.status !== 401) return response;

  let token: string;
  try {
    token = await refreshAndGetAccessToken(previousToken);
  } catch (error) {
    handleRefreshFailure(error, previousToken);
    throw error;
  }
  return send(token);
}

// ============================================================================
// Authentication Interceptor
// ============================================================================

const authInterceptor: Interceptor = (next) => async (req) => {
  const isRetryAttempt = req.header.get(RETRY_HEADER) === RETRY_HEADER_VALUE;
  const token = await getRequestToken();
  setAuthorizationHeader(req, token);

  try {
    const response = await next(req);
    if (req.service.typeName === AuthService.typeName && !response.stream) {
      const message = response.message;
      if (req.method.name === "SignIn" && "accessToken" in message && typeof message.accessToken === "string") {
        observeIdleDeadlineHeader(message.accessToken, response.header);
      } else if (req.method.name === "GetCurrentUser") {
        observeIdleDeadlineHeader(token, response.header);
      }
    }
    return response;
  } catch (error) {
    if (!shouldHandleUnauthenticatedRetry(error, isRetryAttempt)) {
      throw error;
    }

    let newToken: string;
    try {
      newToken = await refreshAndGetAccessToken(token ?? getAccessSessionToken());
    } catch (refreshError) {
      handleRefreshFailure(refreshError, token);
      throw refreshError;
    }
    setAuthorizationHeader(req, newToken);
    req.header.set(RETRY_HEADER, RETRY_HEADER_VALUE);
    const response = await next(req);
    if (req.service.typeName === AuthService.typeName && req.method.name === "GetCurrentUser") {
      observeIdleDeadlineHeader(newToken, response.header);
    }
    return response;
  }
};

// ============================================================================
// Transport & Service Clients
// ============================================================================

const transport = createConnectTransport({
  baseUrl: window.location.origin,
  useBinaryFormat: true,
  fetch: fetchWithCredentials,
  interceptors: [authInterceptor],
});

// Core service clients
export const instanceServiceClient = createClient(InstanceService, transport);
export const authServiceClient = createClient(AuthService, transport);
export const userServiceClient = createClient(UserService, transport);

// Content service clients
export const memoServiceClient = createClient(MemoService, transport);
export const attachmentServiceClient = createClient(AttachmentService, transport);
