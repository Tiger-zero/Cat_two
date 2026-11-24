import { ROUTES } from "@/router/routes";

/** Query parameter used to preserve the intended destination across the auth flow. */
export const AUTH_REDIRECT_PARAM = "redirect";

/** Query parameter used to surface why the user was sent to the auth page. */
export const AUTH_REASON_PARAM = "reason";

/** Reason code signalling that the user hit a memo that requires authentication. */
export const AUTH_REASON_PROTECTED_MEMO = "protected-memo";

export const AUTH_REASON_IDLE_SESSION = "idle-session";

/** Reason code for a visitor sent from the sign-up page while registration is closed. */
export const AUTH_REASON_SIGNUPS_CLOSED = "signups-closed";

/**
 * Validates a post-authentication redirect target.
 *
 * Returns the path when it is a safe same-origin internal destination, otherwise `undefined`.
 * Rejected targets include: non-string / empty, protocol-relative URLs (`//host`), absolute URLs,
 * and any auth-family route (`/auth`, `/auth/admin`, …) which must not be a landing target
 * after sign-in.
 */
export function getSafeRedirectPath(path: string | null | undefined): string | undefined {
  if (!path) {
    return undefined;
  }

  if (!path.startsWith("/") || path.startsWith("//")) {
    return undefined;
  }

  // Never let a redirect target point back into the auth flow and create a redirect loop.
  if (path === ROUTES.AUTH || path.startsWith(`${ROUTES.AUTH}/`) || path.startsWith(`${ROUTES.AUTH}?`)) {
    return undefined;
  }

  return path;
}

/**
 * Builds a URL pointing at the auth entry page, optionally embedding a validated
 * `redirect` target and a machine-readable `reason` code.
 */
export function buildAuthRoute(options?: { redirect?: string | null; reason?: string | null }): string {
  const searchParams = new URLSearchParams();
  const redirectPath = getSafeRedirectPath(options?.redirect);

  if (redirectPath) {
    searchParams.set(AUTH_REDIRECT_PARAM, redirectPath);
  }

  if (options?.reason) {
    searchParams.set(AUTH_REASON_PARAM, options.reason);
  }

  const search = searchParams.toString();
  return search ? `${ROUTES.AUTH}?${search}` : ROUTES.AUTH;
}

const PUBLIC_ROUTE_PREFIXES = [
  ROUTES.AUTH, // Authentication pages
  "/u/", // Legacy author links redirect to Home with a creator filter
] as const;

/**
 * Reports whether a given pathname corresponds to a page that unauthenticated
 * visitors are allowed to view without being bounced to the auth page.
 */
export function isPublicRoute(path: string): boolean {
  return PUBLIC_ROUTE_PREFIXES.some((route) => path.startsWith(route));
}

/**
 * Reports whether an anonymous visitor to a private instance should be redirected
 * to the sign-in page. Authenticated visitors and public instances are never gated.
 */
export function shouldGatePrivateInstance(params: { isPrivateInstance: boolean; isAuthenticated: boolean }): boolean {
  return params.isPrivateInstance && !params.isAuthenticated;
}
