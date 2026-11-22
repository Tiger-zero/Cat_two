import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import SecuritySection from "@/components/Settings/SecuritySection";

const { authenticatedFetch, showErrorToast, addToast } = vi.hoisted(() => ({
  authenticatedFetch: vi.fn<(url: string, init?: RequestInit) => Promise<Response>>(),
  showErrorToast: vi.fn(),
  addToast: vi.fn(),
}));

vi.mock("@/connect", () => ({ authenticatedFetch }));
vi.mock("@/hooks/useCurrentUser", () => ({ default: () => ({ name: "users/admin" }) }));
vi.mock("@/lib/toast", () => ({ showErrorToast, toast: { add: addToast } }));

const blocklistPath = "/api/auth/loginBlocklist";
const blockedAt = "2026-11-21T16:00:00Z";
const firstEntry = { ip: "192.0.2.14", blockedAt };
const secondEntry = { ip: "2001:db8::1", blockedAt };

function renderSettings() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return render(
    <QueryClientProvider client={queryClient}>
      <SecuritySection />
    </QueryClientProvider>,
  );
}

describe("Login IP blocklist settings", () => {
  beforeEach(() => {
    authenticatedFetch.mockReset();
    showErrorToast.mockClear();
    addToast.mockClear();
  });

  it("shows loading, allows retry, and reports an empty blocklist", async () => {
    authenticatedFetch.mockResolvedValueOnce(new Response(null, { status: 503 })).mockResolvedValueOnce(Response.json({ entries: [] }));
    renderSettings();

    expect(screen.getByRole("status")).toHaveTextContent("Loading blocked IP addresses");
    expect(await screen.findByRole("alert")).toHaveTextContent("Could not load blocked IP addresses.");
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));

    expect(await screen.findByText("No blocked IP addresses.")).toBeVisible();
    expect(authenticatedFetch).toHaveBeenCalledTimes(2);
    expect(authenticatedFetch).toHaveBeenLastCalledWith(blocklistPath, { signal: expect.any(AbortSignal), cache: "no-store" });
  });

  it("loads the next page and unblocks an IPv6 address only after confirmation", async () => {
    const pageToken = "next/page+=1";
    const nextPage = `${blocklistPath}?${new URLSearchParams({ pageToken })}`;
    let removed = false;
    authenticatedFetch.mockImplementation(async (url, init) => {
      if (init?.method === "DELETE") {
        expect(url).toBe(`${blocklistPath}/2001%3Adb8%3A%3A1`);
        removed = true;
        return new Response(null, { status: 204 });
      }
      if (url === nextPage) return Response.json({ entries: removed ? [] : [secondEntry] });
      expect(url).toBe(blocklistPath);
      return Response.json({ entries: [firstEntry], nextPageToken: removed ? undefined : pageToken });
    });
    renderSettings();

    expect(await screen.findByText(firstEntry.ip)).toBeVisible();
    expect(screen.getByText("Blocked at 2026-11-22 00:00:00 (UTC+8)")).toHaveAttribute("datetime", blockedAt);
    fireEvent.click(screen.getByRole("button", { name: "Load more" }));
    await screen.findByText(secondEntry.ip);

    fireEvent.click(screen.getByRole("button", { name: `Unblock ${secondEntry.ip}` }));
    const dialog = await screen.findByRole("alertdialog", { name: `Unblock ${secondEntry.ip}?` });
    fireEvent.click(within(dialog).getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(dialog).not.toBeInTheDocument());
    expect(removed).toBe(false);

    fireEvent.click(screen.getByRole("button", { name: `Unblock ${secondEntry.ip}` }));
    const confirmation = within(await screen.findByRole("alertdialog", { name: `Unblock ${secondEntry.ip}?` }));
    fireEvent.click(confirmation.getByRole("button", { name: "Unblock" }));
    await waitFor(() => expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument());

    expect(removed).toBe(true);
    expect(screen.queryByText(secondEntry.ip)).not.toBeInTheDocument();
    expect(screen.getByText(firstEntry.ip)).toBeVisible();
    expect(addToast).toHaveBeenCalledWith({ title: `Unblocked ${secondEntry.ip}`, type: "success" });
  });

  it("keeps the blocked address and its confirmation open if unblocking fails", async () => {
    const diagnostic = vi.spyOn(console, "error").mockImplementation(() => {});
    try {
      authenticatedFetch
        .mockResolvedValueOnce(Response.json({ entries: [firstEntry] }))
        .mockResolvedValueOnce(new Response(null, { status: 503 }));
      renderSettings();

      fireEvent.click(await screen.findByRole("button", { name: `Unblock ${firstEntry.ip}` }));
      const dialog = await screen.findByRole("alertdialog", { name: `Unblock ${firstEntry.ip}?` });
      const confirm = within(dialog).getByRole("button", { name: "Unblock" });
      fireEvent.click(confirm);

      await waitFor(() => expect(showErrorToast).toHaveBeenCalledWith("Could not unblock this IP address. Try again."));
      expect(dialog).toBeVisible();
      await waitFor(() => expect(confirm).not.toHaveAttribute("aria-busy", "true"));
      expect(screen.getByText(firstEntry.ip)).toBeInTheDocument();
      expect(addToast).not.toHaveBeenCalled();
    } finally {
      diagnostic.mockRestore();
    }
  });
});
