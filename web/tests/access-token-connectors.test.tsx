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
  createdAt: Date.parse("2026-11-21T16:00:00Z") / 1_000,
  requestedAt: 0,
  expiresAt: 0,
  remoteIp: "",
};
const usedConnection = { ...connection, requestedAt: connection.createdAt + 60, remoteIp: "192.0.2.14" };
const deletePath = `${connectionsPath}/${connection.id}`;

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
    expect(screen.getByRole("button", { name: "Create MCP connection" })).toBeVisible();
    expect(screen.getByRole("button", { name: "Create your first REST API token" })).toBeVisible();
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
    { choice: "7 days", duration: "7d", expiry: "2026-11-28T16:00:00Z", shanghaiExpiry: "2026-11-29 00:00:00" },
    { choice: "14 days", duration: "14d", expiry: "2026-12-05T16:00:00Z", shanghaiExpiry: "2026-12-06 00:00:00" },
    { choice: "1 year", duration: "1y", expiry: "2027-11-21T16:00:00Z", shanghaiExpiry: "2027-11-22 00:00:00" },
    { choice: "Permanent", duration: "permanent", expiry: null, shanghaiExpiry: null },
  ])("creates a $choice credential and only shows it until the dialog closes", async ({ choice, duration, expiry, shanghaiExpiry }) => {
    const token = "fixture-mcp-token";
    const issuedConnection = { ...connection, expiresAt: expiry ? Date.parse(expiry) / 1_000 : 0 };
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
    expect(durationGroup.getByRole("radio", { name: "7 days" })).toBeChecked();
    if (duration !== "7d") {
      const option = durationGroup.getByRole("radio", { name: choice });
      fireEvent.click(option);
      expect(option).toBeChecked();
    }
    fireEvent.change(createDialog.getByRole("textbox", { name: "Connection name" }), { target: { value: connection.label } });
    fireEvent.click(createDialog.getByRole("button", { name: "Create" }));

    expect(await createDialog.findByText(`Bearer ${token}`)).toBeVisible();
    expect(dialog).toHaveAccessibleDescription(/appears only once.*Authorization as the key.*including Bearer/i);
    fireEvent.click(createDialog.getByRole("button", { name: "Copy Authorization value" }));
    expect(copy).toHaveBeenLastCalledWith(`Bearer ${token}`);
    fireEvent.click(createDialog.getByText("Close").closest("button")!);

    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(await screen.findByText(connection.label)).toBeVisible();
    expect(screen.getByText("Not connected yet")).toBeVisible();
    const validity = shanghaiExpiry ? `Connection valid until ${shanghaiExpiry} (UTC+8)` : "Valid permanently";
    expect(screen.getByText(validity)).toBeVisible();
    expect(screen.queryByText(`Bearer ${token}`)).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Create MCP connection" }));
    const freshDialog = within(await screen.findByRole("dialog"));
    expect(freshDialog.getByRole("textbox", { name: "Connection name" })).toHaveValue("");
    expect(freshDialog.getByRole("radio", { name: "7 days" })).toBeChecked();
    expect(freshDialog.queryByRole("button", { name: "Copy Authorization value" })).not.toBeInTheDocument();
  });

  it("polls for the first connection and stops when a permanent token connects", async () => {
    vi.useFakeTimers({ toFake: ["setInterval", "clearInterval"] });
    let listedConnection = connection;
    authenticatedFetch.mockImplementation(async (url, init) => {
      expect(url).toBe(connectionsPath);
      expect(init?.method ?? "GET").toBe("GET");
      return Response.json({ connections: [listedConnection] });
    });
    const { unmount } = renderSettings();

    try {
      expect(await screen.findByText("Not connected yet")).toBeVisible();
      expect(screen.queryByText(/^First connection IP /)).not.toBeInTheDocument();
      listedConnection = usedConnection;

      await act(async () => vi.advanceTimersByTimeAsync(14_999));
      expect(authenticatedFetch).toHaveBeenCalledTimes(1);
      await act(async () => vi.advanceTimersByTimeAsync(1));

      expect(await screen.findByText("Connected")).toBeVisible();
      expect(screen.getByText(`First connection IP ${usedConnection.remoteIp}`)).toBeVisible();
      expect(screen.getByText("First connection 2026-11-22 00:01:00")).toBeVisible();
      expect(screen.queryByText("Not connected yet")).not.toBeInTheDocument();
      await act(async () => vi.advanceTimersByTimeAsync(15_000));
      expect(authenticatedFetch).toHaveBeenCalledTimes(2);
    } finally {
      unmount();
      vi.useRealTimers();
    }
  });

  it("continues polling a connected 7-day token until it expires", async () => {
    vi.useFakeTimers({ toFake: ["setInterval", "clearInterval"] });
    let listedConnection = { ...usedConnection, expiresAt: connection.createdAt + 7 * 86_400 };
    authenticatedFetch.mockImplementation(async (url) => {
      expect(url).toBe(connectionsPath);
      return Response.json({ connections: [listedConnection] });
    });
    const { unmount } = renderSettings();

    try {
      expect(await screen.findByText("Connected")).toBeVisible();
      listedConnection = { ...listedConnection, status: "expired" };
      await act(async () => vi.advanceTimersByTimeAsync(15_000));

      expect(await screen.findByText("Expired")).toBeVisible();
      expect(screen.getByRole("button", { name: "Delete token" })).toBeEnabled();
      await act(async () => vi.advanceTimersByTimeAsync(15_000));
      expect(authenticatedFetch).toHaveBeenCalledTimes(2);
    } finally {
      unmount();
      vi.useRealTimers();
    }
  });

  it.each([
    { row: { ...connection, status: "awaiting" }, statusLabel: "New token required" },
    { row: { ...connection, status: "pending" }, statusLabel: "New token required" },
    { row: connection, statusLabel: "Not connected yet" },
    { row: usedConnection, statusLabel: "Connected" },
    { row: { ...usedConnection, status: "expired", expiresAt: connection.createdAt + 7 * 86_400 }, statusLabel: "Expired" },
    { row: { ...connection, status: "revoked" }, statusLabel: "Revoked" },
    { row: { ...connection, status: "denied" }, statusLabel: "Denied" },
  ])("shows $statusLabel for $row.status tokens and requires confirmation before deleting", async ({ row, statusLabel }) => {
    const retainedConnection = { ...usedConnection, id: "retained-connection", label: "Laptop" };
    let connections = [row, retainedConnection];
    const deleteToken = vi.fn();
    authenticatedFetch.mockImplementation(async (url, init) => {
      if (url === deletePath) {
        deleteToken(init?.method, init?.body);
        connections = [retainedConnection];
        return new Response(null, { status: 204 });
      }
      expect(url).toBe(connectionsPath);
      return Response.json({ connections });
    });
    renderSettings();

    const tokenRow = within((await screen.findByText(connection.label)).closest("li")!);
    expect(tokenRow.getByText(statusLabel)).toBeVisible();
    expect(tokenRow.queryByRole("button", { name: /approve|deny|revoke/i })).not.toBeInTheDocument();
    fireEvent.click(tokenRow.getByRole("button", { name: "Delete token" }));
    const dialogName = `Delete token for ${connection.label}?`;
    const confirmation = within(await screen.findByRole("alertdialog", { name: dialogName }));
    expect(deleteToken).not.toHaveBeenCalled();
    fireEvent.click(confirmation.getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument());
    expect(deleteToken).not.toHaveBeenCalled();

    fireEvent.click(tokenRow.getByRole("button", { name: "Delete token" }));
    const confirmedDialog = within(await screen.findByRole("alertdialog", { name: dialogName }));
    fireEvent.click(confirmedDialog.getByRole("button", { name: "Delete token" }));

    await waitFor(() => expect(screen.queryByText(connection.label)).not.toBeInTheDocument());
    await waitFor(() => expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument());
    expect(deleteToken).toHaveBeenCalledExactlyOnceWith("DELETE", undefined);
    expect(screen.getByText(retainedConnection.label)).toBeVisible();
  });
});
