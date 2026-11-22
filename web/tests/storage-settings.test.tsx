import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, fireEvent, render, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import StorageSection from "@/components/Settings/StorageSection";

const getInstanceStats = vi.hoisted(() => vi.fn());
vi.mock("@/connect", () => ({ instanceServiceClient: { getInstanceStats } }));

const stats = {
  dataDirectory: "/srv/cat-two/data",
  notesPath: "/srv/cat-two/data/notes",
  attachmentsDirectory: "/srv/cat-two/data/assets",
  database: { driver: "sqlite", sizeBytes: 1024n },
  localStorageBytes: 4096n,
  generatedTime: timestampFromDate(new Date("2026-11-21T16:00:00Z")),
};

function renderStorage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={client}>
      <StorageSection />
    </QueryClientProvider>,
  );
}

describe("Storage settings", () => {
  beforeEach(() => getInstanceStats.mockReset());

  it("shows storage paths and existing usage in the same read-only section", async () => {
    getInstanceStats.mockResolvedValue(stats);
    renderStorage();

    expect(screen.getByRole("heading", { name: "Storage" })).toBeVisible();
    const paths = within(await screen.findByRole("list", { name: "Storage locations" }));
    expect(paths.getByText("Data directory")).toBeVisible();
    expect(paths.getByText(stats.dataDirectory)).toBeVisible();
    expect(paths.getByText("Markdown notes directory")).toBeVisible();
    expect(paths.getByText(stats.notesPath)).toBeVisible();
    expect(paths.getByText("Managed attachments directory")).toBeVisible();
    expect(paths.getByText(stats.attachmentsDirectory)).toBeVisible();

    const usage = within(screen.getByRole("list", { name: "Disk usage" }));
    expect(usage.getByText("sqlite")).toBeVisible();
    expect(usage.getByText("1.0 KB")).toBeVisible();
    expect(usage.getByText("4.0 KB")).toBeVisible();
    expect(screen.getByText("Updated 2026-11-22 00:00:00 (UTC+8)")).toHaveAttribute("datetime", "2026-11-21T16:00:00.000Z");
    expect(screen.queryByRole("radio")).not.toBeInTheDocument();
    expect(screen.queryByRole("textbox")).not.toBeInTheDocument();
    expect(getInstanceStats).toHaveBeenCalledOnce();
  });

  it("shows loading and allows retry after an unavailable response", async () => {
    let reportOffline!: () => void;
    getInstanceStats
      .mockReturnValueOnce(new Promise((_, reject) => (reportOffline = () => reject(new Error("offline")))))
      .mockResolvedValueOnce(stats);
    renderStorage();

    expect(screen.getByRole("status")).toHaveTextContent("Loading storage statistics");
    expect(screen.getByRole("button", { name: "Refresh" })).toBeDisabled();
    await act(async () => reportOffline());
    expect(await screen.findByRole("alert")).toHaveTextContent("Could not load storage statistics.");
    fireEvent.click(screen.getByRole("button", { name: "Refresh" }));

    expect(await screen.findByText(stats.notesPath)).toBeVisible();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(getInstanceStats).toHaveBeenCalledTimes(2);
  });

  it("does not assume a Docker path for older servers and distinguishes unknown bytes from zero", async () => {
    getInstanceStats.mockResolvedValue({ database: { driver: "sqlite", sizeBytes: -1n }, localStorageBytes: 0n });
    renderStorage();

    const paths = within(await screen.findByRole("list", { name: "Storage locations" }));
    expect(paths.getAllByText("Unknown")).toHaveLength(3);
    const usage = within(screen.getByRole("list", { name: "Disk usage" }));
    expect(usage.getByText("Unknown")).toBeVisible();
    expect(usage.getByText("0 B")).toBeVisible();
  });
});
