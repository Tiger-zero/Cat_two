import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import TagsSection from "@/components/Settings/TagsSection";

const { authenticatedFetch, getUserStats, toast } = vi.hoisted(() => ({
  authenticatedFetch: vi.fn(),
  getUserStats: vi.fn(),
  toast: vi.fn(),
}));
vi.mock("@/connect", () => ({ authenticatedFetch, userServiceClient: { getUserStats } }));
vi.mock("@/lib/toast", () => ({ toast: { add: toast }, showErrorToast: vi.fn() }));
vi.mock("@/utils/i18n", () => ({
  useTranslate: () => (key: string, options?: { count: number }) =>
    options ? `${options.count} ${options.count === 1 ? "memo" : "memos"}` : key,
}));
const settings = { tags: Object.fromEntries(["a/.*", "a+", ".+"].map((rule) => [rule, { blurContent: false }])) };
vi.mock("@/contexts/AuthContext", () => ({
  useAuth: () => ({ currentUser: { name: "users/test" }, userTagsSetting: settings }),
}));

it("shows distinct totals from filtered stats and omits oversized rules without a request", async () => {
  const totals = new Map([
    ['tags.exists(t, t.startsWith("a/"))', 1],
    ['tag in ["a","a+","aaa"]', 3],
  ]);
  getUserStats.mockImplementation(async ({ name, filter }) => {
    expect(name).toBe("users/test");
    if (filter === undefined) return { tagCounts: { a: 1, "a/x": 1, "a/y": 1, "a+": 1, aaa: 2, ["z".repeat(8192)]: 1 } };
    expect(totals.has(filter)).toBe(true);
    return { totalMemoCount: totals.get(filter) };
  });

  render(
    <QueryClientProvider client={new QueryClient({ defaultOptions: { queries: { retry: false } } })}>
      <TagsSection />
    </QueryClientProvider>,
  );

  const row = (name: string) => within(screen.getByText(name, { selector: "span" }).closest("li")!);
  expect(await row("a/.*").findByText("1 memo")).toBeVisible();
  expect(await row("a+").findByText("3 memos")).toBeVisible();
  expect(row(".+").queryByText(/memos?$/)).toBeNull();
  expect(getUserStats).toHaveBeenCalledTimes(3);
});

it("asks for confirmation before normalizing and refreshes note and tag queries", async () => {
  getUserStats.mockResolvedValue({ tagCounts: {} });
  authenticatedFetch.mockImplementation(async () => Response.json({ updatedCount: 2 }));
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const invalidate = vi.spyOn(queryClient, "invalidateQueries");
  render(
    <QueryClientProvider client={queryClient}>
      <TagsSection />
    </QueryClientProvider>,
  );

  fireEvent.click(screen.getByRole("button", { name: "setting.tags.normalize" }));
  const dialog = await screen.findByRole("alertdialog", { name: "setting.tags.normalize-title" });
  expect(authenticatedFetch).not.toHaveBeenCalled();
  fireEvent.click(within(dialog).getByRole("button", { name: "setting.tags.normalize" }));

  await waitFor(() => expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument());
  expect(authenticatedFetch).toHaveBeenCalledWith("/internal/tags/normalize", { method: "POST" });
  expect(invalidate).toHaveBeenCalledWith({ queryKey: ["memos"] });
  expect(invalidate).toHaveBeenCalledWith({ queryKey: ["users", "stats"] });
  expect(toast).toHaveBeenCalledWith({ title: "2 memos", type: "success" });
});
