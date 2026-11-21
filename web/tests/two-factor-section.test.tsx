import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import TwoFactorSection from "@/components/Settings/TwoFactorSection";

const authenticatedFetch = vi.hoisted(() => vi.fn());
const logout = vi.hoisted(() => vi.fn());
const navigateTo = vi.hoisted(() => vi.fn());

vi.mock("@/connect", () => ({ authenticatedFetch }));
vi.mock("@/contexts/AuthContext", () => ({ useAuth: () => ({ logout }) }));
vi.mock("@/hooks/useCurrentUser", () => ({ default: () => ({ name: "users/account" }) }));
vi.mock("@/hooks/useNavigateTo", () => ({ default: () => navigateTo }));
vi.mock("@/utils/i18n", () => ({ useTranslate: () => (key: string) => key }));
vi.mock("@/lib/toast", () => ({ showErrorToast: vi.fn(), toast: { add: vi.fn() } }));

const renderSection = () =>
  render(
    <QueryClientProvider client={new QueryClient()}>
      <TwoFactorSection />
    </QueryClientProvider>,
  );

const jsonResponse = (value: unknown, status = 200) => new Response(JSON.stringify(value), { status });
const fillField = (label: string, value: string) => fireEvent.change(screen.getByLabelText(label), { target: { value } });

const openForm = async (name: string) => {
  fireEvent.click(await screen.findByRole("button", { name }));
  return within(await screen.findByRole("dialog"));
};

describe("<TwoFactorSection>", () => {
  beforeEach(() => {
    logout.mockResolvedValue(undefined);
  });

  it("requires the current password before revealing the setup key and enables only on a valid code", async () => {
    authenticatedFetch
      .mockResolvedValueOnce(jsonResponse({ enabled: false }))
      .mockResolvedValueOnce(jsonResponse({ message: "bad password" }, 400))
      .mockResolvedValueOnce(jsonResponse({ secret: "MYAUTHENTICATORKEY" }))
      .mockResolvedValueOnce(jsonResponse({ enabled: true }));
    renderSection();
    const form = await openForm("two-factor.enable");

    expect(screen.queryByText("MYAUTHENTICATORKEY")).not.toBeInTheDocument();
    expect(form.getByRole("button", { name: "two-factor.get-setup-key" })).toBeDisabled();
    fillField("two-factor.current-password", "first-password");
    fireEvent.click(form.getByRole("button", { name: "two-factor.get-setup-key" }));
    expect(await form.findByText("two-factor.setup-error")).toBeInTheDocument();
    expect(screen.queryByText("MYAUTHENTICATORKEY")).not.toBeInTheDocument();

    fillField("two-factor.current-password", "correct-password");
    fireEvent.click(form.getByRole("button", { name: "two-factor.get-setup-key" }));
    expect(await form.findByText("MYAUTHENTICATORKEY")).toBeVisible();
    expect(form.getByRole("button", { name: "two-factor.enable" })).toBeDisabled();
    expect(logout).not.toHaveBeenCalled();

    fillField("two-factor.code", "123456");
    fireEvent.click(form.getByRole("button", { name: "two-factor.enable" }));
    await waitFor(() => expect(navigateTo).toHaveBeenCalledWith("/auth", { replace: true }));
    expect(logout).toHaveBeenCalledOnce();
    expect(authenticatedFetch.mock.calls.map(([path, request]) => [path, request.body])).toEqual([
      ["/api/auth/totp", undefined],
      ["/api/auth/totp/setup", JSON.stringify({ password: "first-password" })],
      ["/api/auth/totp/setup", JSON.stringify({ password: "correct-password" })],
      ["/api/auth/totp/enable", JSON.stringify({ password: "correct-password", code: "123456" })],
    ]);
    expect(screen.queryByText("MYAUTHENTICATORKEY")).not.toBeInTheDocument();
  });

  it("forgets a setup key when enrollment is cancelled", async () => {
    authenticatedFetch
      .mockResolvedValueOnce(jsonResponse({ enabled: false }))
      .mockResolvedValueOnce(jsonResponse({ secret: "MYAUTHENTICATORKEY" }));
    renderSection();
    const form = await openForm("two-factor.enable");
    fillField("two-factor.current-password", "current-password");
    fireEvent.click(form.getByRole("button", { name: "two-factor.get-setup-key" }));
    await form.findByText("MYAUTHENTICATORKEY");
    fireEvent.click(form.getByRole("button", { name: "common.cancel" }));

    const reopened = await openForm("two-factor.enable");
    expect(reopened.getByLabelText("two-factor.current-password")).toHaveValue("");
    expect(reopened.queryByText("MYAUTHENTICATORKEY")).not.toBeInTheDocument();
    expect(logout).not.toHaveBeenCalled();
  });

  it("requires both proofs before disabling and leaves the user signed in after rejection", async () => {
    authenticatedFetch
      .mockResolvedValueOnce(jsonResponse({ enabled: true }))
      .mockResolvedValueOnce(jsonResponse({ message: "bad code" }, 400))
      .mockResolvedValueOnce(jsonResponse({ enabled: false }));
    renderSection();
    const form = await openForm("two-factor.disable");
    fillField("two-factor.code", "123456");
    expect(form.getByRole("button", { name: "two-factor.disable" })).toBeDisabled();
    fillField("two-factor.current-password", "current-password");
    fireEvent.click(form.getByRole("button", { name: "two-factor.disable" }));

    expect(await form.findByText("two-factor.change-error")).toBeInTheDocument();
    expect(logout).not.toHaveBeenCalled();
    fillField("two-factor.code", "654321");
    fireEvent.click(form.getByRole("button", { name: "two-factor.disable" }));
    await waitFor(() => expect(logout).toHaveBeenCalledOnce());
    expect(authenticatedFetch).toHaveBeenLastCalledWith(
      "/api/auth/totp/disable",
      expect.objectContaining({ method: "POST", body: JSON.stringify({ password: "current-password", code: "654321" }) }),
    );
  });
});
