import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import AccessTokenSection from "@/components/Settings/AccessTokenSection";

const { authenticatedFetch, copy, listPersonalAccessTokens, profile } = vi.hoisted(() => ({
  authenticatedFetch: vi.fn<(url: string, init?: RequestInit) => Promise<Response>>(),
  copy: vi.fn(),
  listPersonalAccessTokens: vi.fn(),
  profile: { instanceUrl: "" },
}));

vi.mock("copy-to-clipboard", () => ({ default: copy }));
vi.mock("@/connect", () => ({ authenticatedFetch, userServiceClient: { listPersonalAccessTokens } }));
vi.mock("@/contexts/InstanceContext", () => ({ useInstance: () => ({ profile }) }));
vi.mock("@/hooks/useCurrentUser", () => ({ default: () => ({ name: "users/note-owner" }) }));

const connectionsPath = "/api/mcpConnections";
const connection = {
  id: "workstation-connection",
  label: "Workstation",
  status: "approved",
  createdAt: 1795276800,
  requestedAt: 0,
  expiresAt: 0,
  remoteIp: "",
};
const usedConnection = { ...connection, requestedAt: connection.createdAt + 60, remoteIp: "192.0.2.14" };
const decisionPath = `${connectionsPath}/${connection.id}/decision`;

function renderSettings() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <AccessTokenSection />
    </QueryClientProvider>,
  );
}

describe("MCP and REST setup in access token settings", () => {
  beforeEach(() => {
    authenticatedFetch.mockReset().mockImplementation(async (url) => {
      expect(url).toBe(connectionsPath);
      return Response.json({ connections: [] });
    });
    copy.mockClear();
    copy.mockResolvedValue(true);
    listPersonalAccessTokens.mockResolvedValue({ personalAccessTokens: [] });
    profile.instanceUrl = "";
  });

  it("copies MCP and REST examples with their respective credentials and the canonical instance URL", async () => {
    profile.instanceUrl = "https://notes.example.com/notes";
    renderSettings();
    await screen.findByRole("heading", { name: "MCP & API" });
    expect(screen.getByText(/leave the bearer token environment variable blank/i)).toBeVisible();

    fireEvent.click(screen.getByRole("button", { name: "Copy MCP client configuration" }));
    expect(JSON.parse(copy.mock.lastCall?.[0])).toEqual({
      mcpServers: {
        "cat-two": {
          type: "http",
          url: "https://notes.example.com/notes/mcp",
          headers: { Authorization: "Bearer <YOUR_MCP_TOKEN>" },
        },
      },
    });

    fireEvent.click(screen.getByRole("button", { name: "Copy Create a private test note" }));
    expect(copy.mock.lastCall?.[0]).toContain("POST 'https://notes.example.com/notes/api/memos'");
    expect(copy.mock.lastCall?.[0]).toContain("Authorization: Bearer <YOUR_PAT>");
    expect(copy.mock.lastCall?.[0]).toContain('"visibility":"PRIVATE"');
  });

  it("copies direct endpoints for the current origin with no canonical URL", async () => {
    renderSettings();
    await screen.findByRole("heading", { name: "MCP & API" });

    fireEvent.click(screen.getByRole("button", { name: "Copy MCP server URL" }));
    expect(copy).toHaveBeenLastCalledWith(`${window.location.origin}/mcp`);
    fireEvent.click(screen.getByRole("button", { name: "Copy REST API base URL" }));
    expect(copy).toHaveBeenLastCalledWith(`${window.location.origin}/api`);
    expect(listPersonalAccessTokens).toHaveBeenCalledWith({ parent: "users/note-owner" });
  });

  it.each([
    { choice: "24 hours", duration: "day", expiresAt: connection.createdAt + 86_400 },
    { choice: "Permanent", duration: "permanent", expiresAt: 0 },
  ])("creates a $choice credential and only shows it until the dialog closes", async ({ choice, duration, expiresAt }) => {
    const token = "fixture-mcp-token";
    const issuedConnection = { ...connection, expiresAt };
    let connections: (typeof connection)[] = [];
    authenticatedFetch.mockImplementation(async (url, init) => {
      expect(url).toBe(connectionsPath);
      if (init?.method === "POST") {
        expect(JSON.parse(String(init.body))).toEqual({ label: connection.label, duration });
        connections = [issuedConnection];
        return Response.json({ connection: issuedConnection, token }, { status: 201 });
      }
      return Response.json({ connections });
    });
    renderSettings();

    const createButton = screen.getByRole("button", { name: "Create MCP connection" });
    await waitFor(() => expect(createButton).toBeEnabled());
    fireEvent.click(createButton);
    const dialog = await screen.findByRole("dialog");
    const createDialog = within(dialog);
    const durationGroup = within(createDialog.getByRole("radiogroup", { name: "Token duration" }));
    expect(durationGroup.getByRole("radio", { name: "24 hours" })).toBeChecked();
    if (duration === "permanent") {
      const permanent = durationGroup.getByRole("radio", { name: choice });
      fireEvent.click(permanent);
      expect(permanent).toBeChecked();
    }
    fireEvent.change(createDialog.getByRole("textbox", { name: "Connection name" }), { target: { value: connection.label } });
    fireEvent.click(createDialog.getByRole("button", { name: "Create" }));

    expect(await createDialog.findByText(`Bearer ${token}`)).toBeVisible();
    expect(dialog).toHaveAccessibleDescription(/appears only once.*authorization header/i);
    fireEvent.click(createDialog.getByRole("button", { name: "Copy Bearer header value" }));
    expect(copy).toHaveBeenLastCalledWith(`Bearer ${token}`);
    fireEvent.click(createDialog.getByText("Close").closest("button")!);

    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(await screen.findByText(connection.label)).toBeVisible();
    expect(screen.getByText("Active")).toBeVisible();
    expect(screen.getByText("Not used yet")).toBeVisible();
    expect(screen.queryByText(`Bearer ${token}`)).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Create MCP connection" }));
    const freshDialog = within(await screen.findByRole("dialog"));
    expect(freshDialog.getByRole("textbox", { name: "Connection name" })).toHaveValue("");
    expect(freshDialog.getByRole("radio", { name: "24 hours" })).toBeChecked();
    expect(freshDialog.queryByRole("button", { name: "Copy Bearer header value" })).not.toBeInTheDocument();
  });

  it("polls for first use and stops when a permanent token has been used", async () => {
    vi.useFakeTimers({ toFake: ["setInterval", "clearInterval"] });
    let listedConnection = connection;
    authenticatedFetch.mockImplementation(async (url, init) => {
      expect(url).toBe(connectionsPath);
      expect(init?.method ?? "GET").toBe("GET");
      return Response.json({ connections: [listedConnection] });
    });
    const { unmount } = renderSettings();

    try {
      expect(await screen.findByText("Not used yet")).toBeVisible();
      listedConnection = usedConnection;

      await act(async () => vi.advanceTimersByTimeAsync(14_999));
      expect(authenticatedFetch).toHaveBeenCalledTimes(1);
      await act(async () => vi.advanceTimersByTimeAsync(1));

      expect(await screen.findByText(usedConnection.remoteIp, { exact: false })).toBeVisible();
      expect(screen.getByText(/^First request /)).toBeVisible();
      expect(screen.queryByText("Not used yet")).not.toBeInTheDocument();
      await act(async () => vi.advanceTimersByTimeAsync(15_000));
      expect(authenticatedFetch).toHaveBeenCalledTimes(2);
    } finally {
      unmount();
      vi.useRealTimers();
    }
  });

  it("continues polling a used 24-hour token until it expires", async () => {
    vi.useFakeTimers({ toFake: ["setInterval", "clearInterval"] });
    let listedConnection = { ...usedConnection, expiresAt: connection.createdAt + 86_400 };
    authenticatedFetch.mockImplementation(async (url) => {
      expect(url).toBe(connectionsPath);
      return Response.json({ connections: [listedConnection] });
    });
    const { unmount } = renderSettings();

    try {
      expect(await screen.findByText("Active")).toBeVisible();
      listedConnection = { ...listedConnection, status: "expired" };
      await act(async () => vi.advanceTimersByTimeAsync(15_000));

      expect(await screen.findByText("Expired")).toBeVisible();
      expect(screen.queryByRole("button", { name: "Revoke" })).not.toBeInTheDocument();
      await act(async () => vi.advanceTimersByTimeAsync(15_000));
      expect(authenticatedFetch).toHaveBeenCalledTimes(2);
    } finally {
      unmount();
      vi.useRealTimers();
    }
  });

  it.each([
    { status: "awaiting", statusLabel: "New token required" },
    { status: "pending", statusLabel: "New token required" },
    { status: "approved", statusLabel: "Active" },
  ])("shows $statusLabel for $status connections and requires confirmation before revoking", async ({ status, statusLabel }) => {
    let listedConnection = { ...connection, status };
    const revoke = vi.fn();
    authenticatedFetch.mockImplementation(async (url, init) => {
      if (url === decisionPath) {
        expect(init?.method).toBe("POST");
        revoke(JSON.parse(String(init?.body)));
        listedConnection = { ...connection, status: "revoked" };
        return new Response(null, { status: 204 });
      }
      expect(url).toBe(connectionsPath);
      return Response.json({ connections: [listedConnection] });
    });
    renderSettings();

    expect(await screen.findByText(statusLabel)).toBeVisible();
    expect(screen.queryByRole("button", { name: /approve|deny/i })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Revoke" }));
    const dialogName = `Revoke ${connection.label}?`;
    const confirmation = within(await screen.findByRole("alertdialog", { name: dialogName }));
    expect(revoke).not.toHaveBeenCalled();
    fireEvent.click(confirmation.getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument());
    expect(revoke).not.toHaveBeenCalled();

    fireEvent.click(screen.getByRole("button", { name: "Revoke" }));
    const confirmedDialog = within(await screen.findByRole("alertdialog", { name: dialogName }));
    fireEvent.click(confirmedDialog.getByRole("button", { name: "Revoke" }));

    expect(await screen.findByText("Revoked")).toBeVisible();
    await waitFor(() => expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument());
    expect(revoke).toHaveBeenCalledExactlyOnceWith({ action: "revoke" });
    expect(screen.queryByRole("button", { name: "Revoke" })).not.toBeInTheDocument();
  });
});
