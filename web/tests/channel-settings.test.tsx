import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import NetworkSection from "@/components/Settings/NetworkSection";
import NotificationSection from "@/components/Settings/NotificationSection";

const authenticatedFetch = vi.hoisted(() => vi.fn<(path: string, init?: RequestInit) => Promise<Response>>());
vi.mock("@/connect", () => ({ authenticatedFetch }));
vi.mock("@/hooks/useCurrentUser", () => ({ default: () => ({ name: "users/admin" }) }));

const notificationSettings = {
  telegram: { enabled: true, configured: true, source: "environment", userId: "9007199254740993", hasBotToken: true },
  wecom: {
    enabled: false,
    configured: false,
    source: "",
    userId: "",
    corpId: "",
    agentId: "",
    callbackUrl: "",
    hasCallbackToken: false,
    hasEncodingAesKey: false,
  },
  wecomCallbackPath: "/integrations/wecom",
  wecomDefaultCallbackUrl: "https://notes.example.com/integrations/wecom",
};

function renderSettings(section: ReactNode) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={client}>{section}</QueryClientProvider>);
}

function lastRequestBody() {
  return JSON.parse(String(authenticatedFetch.mock.lastCall?.[1]?.body));
}

describe("Notification and network settings", () => {
  beforeEach(() => authenticatedFetch.mockReset());

  it("saves only the Telegram channel without replacing an existing token", async () => {
    const updated = { ...notificationSettings, telegram: { ...notificationSettings.telegram, enabled: false, source: "settings" } };
    authenticatedFetch.mockResolvedValueOnce(Response.json(notificationSettings)).mockResolvedValueOnce(Response.json(updated));
    renderSettings(<NotificationSection />);

    expect(await screen.findByText(notificationSettings.wecomDefaultCallbackUrl)).toBeVisible();
    const switchControl = screen.getByRole("switch", { name: "Enable Telegram" });
    const token = screen.getByLabelText("Bot token");
    expect(switchControl).toBeChecked();
    expect(token).toHaveAttribute("type", "password");
    expect(token).toHaveValue("");
    expect(token).toHaveAttribute("placeholder", "Already saved. Leave blank to keep it.");

    fireEvent.click(switchControl);
    fireEvent.click(screen.getByRole("button", { name: "Save Telegram" }));
    expect(authenticatedFetch.mock.lastCall?.[0]).toBe("/internal/notificationSettings");
    expect(lastRequestBody()).toEqual({ telegram: { enabled: false, userId: "9007199254740993" } });
    expect(await screen.findByText("Disabled")).toBeVisible();
  });

  it("saves WeCom credentials without sending Telegram values and clears the secret fields", async () => {
    const updated = {
      ...notificationSettings,
      wecom: {
        enabled: true,
        configured: true,
        source: "settings",
        userId: "alice",
        corpId: "corp-id",
        agentId: "1000002",
        callbackUrl: "",
        hasCallbackToken: true,
        hasEncodingAesKey: true,
      },
    };
    authenticatedFetch.mockResolvedValueOnce(Response.json(notificationSettings)).mockResolvedValueOnce(Response.json(updated));
    renderSettings(<NotificationSection />);

    fireEvent.change(await screen.findByLabelText("WeCom company ID"), { target: { value: updated.wecom.corpId } });
    fireEvent.change(screen.getByLabelText("Agent ID"), { target: { value: updated.wecom.agentId } });
    fireEvent.change(screen.getByLabelText("WeCom member UserID"), { target: { value: updated.wecom.userId } });
    const token = screen.getByLabelText("Callback token");
    const key = screen.getByLabelText("EncodingAESKey");
    fireEvent.change(token, { target: { value: "new-token" } });
    fireEvent.change(key, { target: { value: "new-key" } });
    fireEvent.click(screen.getByRole("switch", { name: "Enable WeCom" }));
    fireEvent.click(screen.getByRole("button", { name: "Save WeCom" }));

    expect(lastRequestBody()).toEqual({
      wecom: {
        enabled: true,
        userId: "alice",
        agentId: "1000002",
        corpId: "corp-id",
        callbackToken: "new-token",
        encodingAesKey: "new-key",
      },
    });
    await waitFor(() => expect(token).toHaveValue(""));
    expect(key).toHaveValue("");
    expect(key).toHaveAttribute("type", "password");
  });

  it.each([
    ["notes.example.com:8443", "https://notes.example.com:8443/integrations/wecom"],
    ["http://notes.example.com:8080/app/integrations/wecom", "http://notes.example.com:8080/app/integrations/wecom"],
  ])("saves and clears %s while WeCom is disabled", async (address, callbackUrl) => {
    authenticatedFetch
      .mockResolvedValueOnce(Response.json(notificationSettings))
      .mockResolvedValueOnce(Response.json({ ...notificationSettings, wecom: { ...notificationSettings.wecom, callbackUrl } }))
      .mockResolvedValueOnce(Response.json(notificationSettings));
    renderSettings(<NotificationSection />);

    const input = await screen.findByLabelText("Public callback address");
    const save = screen.getByRole("button", { name: "Save WeCom" });
    expect(screen.getByRole("switch", { name: "Enable WeCom" })).not.toBeChecked();
    expect(input).toHaveValue("");
    expect(input).toHaveAttribute("placeholder", "https://notes.example.com:8443");
    expect(screen.getByText(notificationSettings.wecomDefaultCallbackUrl)).toBeVisible();

    fireEvent.change(input, { target: { value: address } });
    fireEvent.click(save);
    expect(lastRequestBody()).toEqual({
      wecom: { enabled: false, userId: "", corpId: "", agentId: "", callbackUrl: address },
    });
    expect(await screen.findByText(callbackUrl)).toBeVisible();
    expect(input).toHaveValue(callbackUrl);
    expect(screen.queryByText(notificationSettings.wecomDefaultCallbackUrl)).not.toBeInTheDocument();

    fireEvent.change(input, { target: { value: "" } });
    fireEvent.click(save);
    expect(lastRequestBody()).toEqual({ wecom: { enabled: false, userId: "", corpId: "", agentId: "", callbackUrl: "" } });
    expect(await screen.findByText(notificationSettings.wecomDefaultCallbackUrl)).toBeVisible();
    expect(input).toHaveValue("");
  });

  it("does not copy a local browser origin and gives a field error for invalid callbacks", async () => {
    authenticatedFetch.mockResolvedValueOnce(Response.json({ ...notificationSettings, wecomDefaultCallbackUrl: "" }));
    renderSettings(<NotificationSection />);

    const input = await screen.findByLabelText("Public callback address");
    expect(input).toHaveValue("");
    expect(screen.queryByRole("button", { name: /copy.*wecom callback url/i })).not.toBeInTheDocument();
    for (const address of [
      "ftp://notes.example.com",
      "192.168.1.10:8443",
      "http://192.168.1.10:5230",
      "https://notes.example.com/wrong-path",
    ]) {
      fireEvent.change(input, { target: { value: address } });
      fireEvent.click(screen.getByRole("button", { name: "Save WeCom" }));
      expect(
        screen.getByText("Enter a public domain with an optional port or an HTTP or HTTPS URL ending in /integrations/wecom."),
      ).toBeVisible();
      expect(input).toHaveAttribute("aria-invalid", "true");
      expect(authenticatedFetch).toHaveBeenCalledTimes(1);
    }
  });

  it("can set a proxy, keeps its URL masked, and can disable it without replacing it", async () => {
    const enabled = { enabled: true, hasProxyUrl: true, source: "settings" };
    authenticatedFetch
      .mockResolvedValueOnce(Response.json({ enabled: false, hasProxyUrl: false, source: "" }))
      .mockResolvedValueOnce(Response.json(enabled))
      .mockResolvedValueOnce(Response.json({ ...enabled, enabled: false }));
    renderSettings(<NetworkSection />);

    const control = await screen.findByRole("switch", { name: "Use a proxy for Telegram" });
    const proxy = screen.getByLabelText("Proxy URL");
    const save = screen.getByRole("button", { name: "Save" });
    expect(proxy).toHaveAttribute("type", "password");
    expect(screen.getByText("Using the container environment")).toBeVisible();
    fireEvent.click(control);
    fireEvent.click(save);
    expect(screen.getByRole("alert")).toHaveTextContent("Enter a full proxy URL");
    expect(authenticatedFetch).toHaveBeenCalledTimes(1);

    const proxyUrl = "socks5h://proxy-user:proxy-password@proxy.example.test:1080";
    fireEvent.change(proxy, { target: { value: proxyUrl } });
    fireEvent.click(save);
    expect(authenticatedFetch.mock.lastCall?.[0]).toBe("/internal/networkSettings");
    expect(lastRequestBody()).toEqual({ enabled: true, proxyUrl });
    expect(await screen.findByText("Connecting through the saved proxy")).toBeVisible();
    expect(proxy).toHaveValue("");
    expect(proxy).toHaveAttribute("placeholder", "Already saved. Leave blank to keep it.");

    fireEvent.click(control);
    fireEvent.click(save);
    expect(lastRequestBody()).toEqual({ enabled: false });
    expect(await screen.findByText("Connecting directly")).toBeVisible();
  });
});
