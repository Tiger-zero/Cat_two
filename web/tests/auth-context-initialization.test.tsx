import { create } from "@bufbuild/protobuf";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { type ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const authState = vi.hoisted(() => ({ hasToken: false, clearAccessToken: vi.fn() }));
const clients = vi.hoisted(() => ({
  getCurrentUser: vi.fn(),
  listUserSettings: vi.fn(),
}));

vi.mock("@/auth-state", () => ({
  clearAccessToken: authState.clearAccessToken,
  onAccessSessionEnd: () => () => undefined,
  onAccessTokenChange: () => () => undefined,
  getAccessToken: () => (authState.hasToken ? "token" : undefined),
  getAccessSessionToken: () => null,
  accessSessionID: () => "",
}));

vi.mock("@/connect", () => ({
  authServiceClient: {
    getCurrentUser: clients.getCurrentUser,
    signOut: vi.fn(),
  },
  refreshAccessToken: vi.fn(async () => undefined),
  userServiceClient: {
    listUserSettings: clients.listUserSettings,
  },
}));

import { AuthProvider, useAuth } from "@/contexts/AuthContext";
import { type UserSetting, UserSettingSchema } from "@/types/proto/api/user_service_pb";

const Probe = () => {
  const { currentUser, initialize, isInitialized, isUserSettingsInitialized, refetchSettings, userGeneralSetting, userSettingsLoadError } =
    useAuth();
  return (
    <div>
      <span data-testid="initialized">{isInitialized ? "yes" : "no"}</span>
      <span data-testid="user-settings-initialized">{isUserSettingsInitialized ? "yes" : "no"}</span>
      <span data-testid="user">{currentUser?.name ?? "none"}</span>
      <span data-testid="theme">{userGeneralSetting?.theme ?? "none"}</span>
      {userSettingsLoadError && <p role="alert">Settings unavailable</p>}
      <button type="button" onClick={() => void initialize()}>
        initialize
      </button>
      <button type="button" onClick={() => void refetchSettings()}>
        fetch settings
      </button>
    </div>
  );
};

const wrapper = ({ children }: { children: ReactNode }) => (
  <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
    <AuthProvider>{children}</AuthProvider>
  </QueryClientProvider>
);

const generalSettings = (theme: string) => ({
  userSettings: [create(UserSettingSchema, { value: { case: "generalSetting", value: { theme } } })],
});

describe("AuthProvider initialization", () => {
  beforeEach(() => {
    authState.hasToken = false;
    clients.getCurrentUser.mockReset();
    clients.listUserSettings.mockReset();
  });

  it("resets full readiness while post-sign-in settings are pending", async () => {
    let resolveSettings!: (value: { userSettings: [] }) => void;
    clients.getCurrentUser.mockResolvedValue({ user: { name: "users/alice", username: "alice" } });
    clients.listUserSettings.mockImplementation(() => new Promise<{ userSettings: [] }>((resolve) => (resolveSettings = resolve)));

    render(<Probe />, { wrapper });

    // Settle the initial unauthenticated pass; this reproduces the state from
    // which PasswordSignInForm and AuthCallback invoke initialize again.
    fireEvent.click(screen.getByRole("button", { name: "initialize" }));
    await waitFor(() => expect(screen.getByTestId("initialized")).toHaveTextContent("yes"));

    authState.hasToken = true;
    fireEvent.click(screen.getByRole("button", { name: "initialize" }));
    await waitFor(() => expect(screen.getByTestId("user")).toHaveTextContent("users/alice"));
    expect(screen.getByTestId("initialized")).toHaveTextContent("no");
    expect(screen.getByTestId("user-settings-initialized")).toHaveTextContent("no");

    resolveSettings({ userSettings: [] });
    await waitFor(() => expect(screen.getByTestId("user-settings-initialized")).toHaveTextContent("yes"));
    expect(screen.getByTestId("initialized")).toHaveTextContent("yes");
  });

  it("keeps verified identity while failed settings are retried", async () => {
    authState.hasToken = true;
    clients.getCurrentUser.mockResolvedValue({ user: { name: "users/alice", username: "alice" } });
    clients.listUserSettings.mockRejectedValueOnce(new Error("Offline")).mockResolvedValueOnce(generalSettings("default-dark"));
    vi.spyOn(console, "error").mockImplementationOnce(() => undefined);

    render(<Probe />, { wrapper });
    fireEvent.click(screen.getByRole("button", { name: "initialize" }));

    expect(await screen.findByRole("alert")).toHaveTextContent("Settings unavailable");
    expect(screen.getByTestId("user")).toHaveTextContent("users/alice");
    expect(screen.getByTestId("user-settings-initialized")).toHaveTextContent("no");
    expect(authState.clearAccessToken).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "fetch settings" }));

    await waitFor(() => expect(screen.getByTestId("theme")).toHaveTextContent("default-dark"));
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(screen.getByTestId("initialized")).toHaveTextContent("yes");
    expect(clients.getCurrentUser).toHaveBeenCalledOnce();
  });

  it("commits only the latest settings request and disregards responses from an older session with the same account", async () => {
    authState.hasToken = true;
    clients.getCurrentUser.mockResolvedValue({ user: { name: "users/alice", username: "alice" } });
    const pending: ((response: { userSettings: UserSetting[] }) => void)[] = [];
    clients.listUserSettings
      .mockResolvedValueOnce(generalSettings("default"))
      .mockImplementation(() => new Promise<{ userSettings: UserSetting[] }>((resolve) => pending.push(resolve)));
    render(<Probe />, { wrapper });
    const initialize = screen.getByRole("button", { name: "initialize" });
    const refetch = screen.getByRole("button", { name: "fetch settings" });
    fireEvent.click(initialize);
    await waitFor(() => expect(screen.getByTestId("initialized")).toHaveTextContent("yes"));

    fireEvent.click(refetch);
    fireEvent.click(refetch);
    await act(() => pending[1](generalSettings("default-dark")));
    await act(() => pending[0](generalSettings("system")));
    expect(screen.getByTestId("theme")).toHaveTextContent("default-dark");

    fireEvent.click(refetch);
    fireEvent.click(initialize);
    await waitFor(() => expect(pending).toHaveLength(4));
    await act(() => pending[2](generalSettings("system")));
    expect(screen.getByTestId("user-settings-initialized")).toHaveTextContent("no");
    await act(() => pending[3](generalSettings("default")));
    expect(screen.getByTestId("theme").textContent).toBe("default");
  });
});
