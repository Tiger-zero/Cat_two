import { Code, ConnectError } from "@connectrpc/connect";
import { useQueryClient } from "@tanstack/react-query";
import { createContext, type ReactNode, useCallback, useContext, useEffect, useMemo, useRef, useState } from "react";
import {
  type AccessSessionEndReason,
  accessSessionID,
  clearAccessToken,
  endAccessSession,
  getAccessToken,
  onAccessSessionEnd,
} from "@/auth-state";
import { authServiceClient, isSessionIdleExpiredError, refreshAccessToken, userServiceClient } from "@/connect";
import { userKeys } from "@/hooks/useUserQueries";
import { monitorIdleSession } from "@/lib/session-idle";
import { ROUTES } from "@/router/routes";
import type {
  User,
  UserSetting_EditorSetting,
  UserSetting_GeneralSetting,
  UserSetting_TagsSetting,
} from "@/types/proto/api/user_service_pb";
import { AUTH_REASON_IDLE_SESSION, AUTH_REASON_PARAM } from "@/utils/redirect-safety";

interface AuthState {
  currentUser: User | undefined;
  userGeneralSetting: UserSetting_GeneralSetting | undefined;
  userTagsSetting: UserSetting_TagsSetting | undefined;
  /** Unset until the user first changes an editor preference. */
  userEditorSetting: UserSetting_EditorSetting | undefined;
  /** Authentication identity has settled, while user settings may still be loading. */
  isIdentityInitialized: boolean;
  /** User settings that affect memo presentation are safe to consume. */
  isUserSettingsInitialized: boolean;
  userSettingsLoadError: boolean;
  isInitialized: boolean;
  isLoading: boolean;
  sessionEndReason?: AccessSessionEndReason;
}

interface AuthContextValue extends AuthState {
  initialize: () => Promise<void>;
  logout: () => Promise<void>;
  refetchSettings: () => Promise<void>;
  setCurrentUser: (user: User | undefined) => void;
  /** Applies an editor setting locally, ahead of or instead of a refetch. */
  setUserEditorSetting: (setting: UserSetting_EditorSetting | undefined) => void;
}

const AuthContext = createContext<AuthContextValue | null>(null);

/** Settled auth state for a request with no valid session (init finished, not loading). */
const UNAUTHENTICATED_STATE: AuthState = {
  currentUser: undefined,
  userGeneralSetting: undefined,
  userTagsSetting: undefined,
  userEditorSetting: undefined,
  isIdentityInitialized: true,
  isUserSettingsInitialized: true,
  userSettingsLoadError: false,
  isInitialized: true,
  isLoading: false,
};

export function AuthProvider({ children }: { children: ReactNode }) {
  const queryClient = useQueryClient();
  const initialization = useRef(0);
  const settingsRequest = useRef(0);
  const [state, setState] = useState<AuthState>({
    currentUser: undefined,
    userGeneralSetting: undefined,
    userTagsSetting: undefined,
    userEditorSetting: undefined,
    isIdentityInitialized: false,
    isUserSettingsInitialized: false,
    userSettingsLoadError: false,
    isInitialized: false,
    isLoading: true,
  });

  useEffect(
    () =>
      onAccessSessionEnd((sessionEndReason) => {
        initialization.current++;
        setState({ ...UNAUTHENTICATED_STATE, sessionEndReason });
        queryClient.clear();
      }),
    [queryClient],
  );
  useEffect(monitorIdleSession, []);

  const fetchUserSettings = useCallback(async (userName: string) => {
    const initializedAt = initialization.current;
    const request = ++settingsRequest.current;
    const isCurrent = (prev: AuthState) =>
      initializedAt === initialization.current && request === settingsRequest.current && prev.currentUser?.name === userName;
    setState((prev) => (isCurrent(prev) && prev.userSettingsLoadError ? { ...prev, isLoading: true } : prev));

    try {
      const { userSettings: settings } = await userServiceClient.listUserSettings({ parent: userName });
      const generalSetting = settings.find((s) => s.value.case === "generalSetting");
      const tagsSetting = settings.find((s) => s.value.case === "tagsSetting");
      const editorSetting = settings.find((s) => s.value.case === "editorSetting");

      setState((prev) =>
        isCurrent(prev)
          ? {
              ...prev,
              userGeneralSetting: generalSetting?.value.case === "generalSetting" ? generalSetting.value.value : undefined,
              userTagsSetting: tagsSetting?.value.case === "tagsSetting" ? tagsSetting.value.value : undefined,
              userEditorSetting: editorSetting?.value.case === "editorSetting" ? editorSetting.value.value : undefined,
              isUserSettingsInitialized: true,
              userSettingsLoadError: false,
              isInitialized: true,
              isLoading: false,
            }
          : prev,
      );
    } catch (error) {
      // A background refresh can keep the already loaded settings. During
      // initialization, wait for a successful retry before showing notes.
      setState((prev) =>
        isCurrent(prev) && !prev.isUserSettingsInitialized ? { ...prev, userSettingsLoadError: true, isLoading: false } : prev,
      );
      throw error;
    }
  }, []);

  const initialize = useCallback(async () => {
    const attempt = ++initialization.current;
    // `initialize` also runs after sign-in, when the previous unauthenticated
    // state is already marked initialized. Reset the full-readiness flag so
    // consumers cannot render with the new identity and stale/default settings.
    setState((prev) => ({
      ...prev,
      isUserSettingsInitialized: false,
      userSettingsLoadError: false,
      isInitialized: false,
      isLoading: true,
    }));

    // Try to get or refresh the access token.
    // This handles PWA isolated storage scenarios (e.g., iOS Safari) where localStorage
    // may be empty but a valid HTTP-only refresh token cookie still exists.
    // getAccessToken() returns a cached token or loads from localStorage if valid.
    const onAuthRoute = window.location.pathname === ROUTES.AUTH || window.location.pathname.startsWith(`${ROUTES.AUTH}/`);
    let sessionEndReason: AccessSessionEndReason | undefined;
    if (onAuthRoute && new URLSearchParams(window.location.search).get(AUTH_REASON_PARAM) === AUTH_REASON_IDLE_SESSION) {
      sessionEndReason = AUTH_REASON_IDLE_SESSION;
    }
    if (!getAccessToken() && !sessionEndReason) {
      try {
        await refreshAccessToken();
      } catch (error) {
        if (isSessionIdleExpiredError(error)) sessionEndReason = AUTH_REASON_IDLE_SESSION;
      }
    }

    if (initialization.current !== attempt) return;

    // If we still don't have a token after refresh attempt, skip getCurrentUser call
    // to avoid unnecessary network request for unauthenticated users.
    if (!getAccessToken()) {
      setState({ ...UNAUTHENTICATED_STATE, sessionEndReason });
      return;
    }

    let currentUser: User | undefined;
    try {
      ({ user: currentUser } = await authServiceClient.getCurrentUser({}));
    } catch (error) {
      if (initialization.current !== attempt) return;
      console.error("Failed to initialize auth:", error);
    }
    if (initialization.current !== attempt) return;
    if (!currentUser) {
      clearAccessToken();
      setState(UNAUTHENTICATED_STATE);
      return;
    }

    // Start route and data loading as soon as the identity is verified.
    const verifiedUser = currentUser;
    setState((prev) => ({ ...prev, currentUser: verifiedUser, sessionEndReason: undefined, isIdentityInitialized: true }));
    queryClient.setQueryData(userKeys.currentUser(), verifiedUser);
    queryClient.setQueryData(userKeys.detail(verifiedUser.name), verifiedUser);

    try {
      await fetchUserSettings(verifiedUser.name);
    } catch (error) {
      if (initialization.current === attempt) console.error("Failed to load user settings:", error);
    }
  }, [fetchUserSettings, queryClient]);

  const logout = useCallback(async () => {
    const token = getAccessToken();
    try {
      await authServiceClient.signOut({});
    } catch (error) {
      if (!(error instanceof ConnectError) || error.code !== Code.Unauthenticated) throw error;
    }
    // During the upgrade the interceptor can obtain a session-bound token by
    // refreshing the old browser credentials before completing sign-out.
    endAccessSession(accessSessionID(token) ? token : (getAccessToken() ?? token));
  }, []);

  const refetchSettings = useCallback(async () => {
    const currentUserName = state.currentUser?.name;
    if (currentUserName) await fetchUserSettings(currentUserName);
  }, [fetchUserSettings, state.currentUser?.name]);

  // Sync the updated user to AuthContext and React Query cache after profile changes
  const setCurrentUser = useCallback(
    (user: User | undefined) => {
      const previousUser = queryClient.getQueryData<User>(userKeys.currentUser());
      setState((prev) => ({ ...prev, currentUser: user }));
      if (user) {
        queryClient.setQueryData(userKeys.currentUser(), user);
        queryClient.setQueryData(userKeys.detail(user.name), user);
      } else {
        queryClient.removeQueries({ queryKey: userKeys.currentUser(), exact: true });
        if (previousUser?.name) {
          queryClient.removeQueries({ queryKey: userKeys.detail(previousUser.name), exact: true });
        }
      }
    },
    [queryClient],
  );

  const setUserEditorSetting = useCallback((setting: UserSetting_EditorSetting | undefined) => {
    setState((prev) => ({ ...prev, userEditorSetting: setting }));
  }, []);

  // Memoize context value to prevent unnecessary re-renders of consumers
  const value = useMemo(
    () => ({
      ...state,
      initialize,
      logout,
      refetchSettings,
      setCurrentUser,
      setUserEditorSetting,
    }),
    [state, initialize, logout, refetchSettings, setCurrentUser, setUserEditorSetting],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth() {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error("useAuth must be used within AuthProvider");
  }
  return context;
}

// Convenience hook for just the current user
export function useCurrentUserFromAuth() {
  const { currentUser } = useAuth();
  return currentUser;
}
