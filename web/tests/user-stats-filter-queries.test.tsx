import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useWelcomeGuide } from "@/components/WelcomeGuide/useWelcomeGuide";
import { useFilteredMemoStats } from "@/hooks/useFilteredMemoStats";
import { useAllUserStats, useTagCounts, useUserStats } from "@/hooks/useUserQueries";
import { State } from "@/types/proto/api/common_pb";

const clients = vi.hoisted(() => ({
  getUserStats: vi.fn(),
  listUserStats: vi.fn(),
}));
const authState = {
  currentUser: { name: "users/test", createTime: timestampFromDate(new Date()) },
  isUserSettingsInitialized: true,
};
const viewState = { timeBasis: "create_time" };

vi.mock("@/connect", () => ({
  userServiceClient: clients,
}));
vi.mock("@/contexts/AuthContext", () => ({ useAuth: () => authState }));
vi.mock("@/contexts/ViewContext", () => ({ useView: () => viewState }));

const createWrapper = () => {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return function QueryWrapper({ children }: { children: ReactNode }) {
    return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
  };
};

describe("User statistics filter queries", () => {
  beforeEach(() => {
    authState.currentUser.createTime = timestampFromDate(new Date());
    authState.isUserSettingsInitialized = true;
    viewState.timeBasis = "create_time";
    clients.getUserStats.mockReset().mockResolvedValue({ name: "users/test/stats" });
    clients.listUserStats.mockReset().mockResolvedValue({ userStats: [] });
  });

  afterEach(() => {
    localStorage.removeItem("memos-welcome-guide-dismissed:users/test");
  });

  it("shares one active owner request between Home statistics, editor tags, and the eligible guide", async () => {
    clients.getUserStats.mockResolvedValue({
      name: "users/test/stats",
      totalMemoCount: 3,
      tagCounts: { work: 2, "work/project": 1 },
      dailyActivity: { createdCounts: { "2026-05-01": 1, "2026-05-02": 2 }, updatedCounts: { "2026-05-03": 3 } },
    });
    const { result, rerender } = renderHook(
      () => ({
        sidebar: useFilteredMemoStats({ context: "home", userName: "users/test" }),
        editor: useTagCounts(true),
        guide: useWelcomeGuide(true),
      }),
      { wrapper: createWrapper() },
    );

    await waitFor(() => expect(result.current.editor.data).toEqual({ work: 2, "work/project": 1 }));
    expect(result.current.sidebar.statistics.activityStats).toEqual({ "2026-05-01": 1, "2026-05-02": 2 });
    expect(result.current.sidebar.summary).toEqual({ memoCount: 3, tagCount: 2, recordedDays: 2 });
    expect(result.current.guide.open).toBe(false);

    viewState.timeBasis = "update_time";
    rerender();
    expect(result.current.sidebar.statistics.activityStats).toEqual({ "2026-05-03": 3 });
    expect(result.current.sidebar.summary?.recordedDays).toBe(2);
    expect(clients.getUserStats).toHaveBeenCalledExactlyOnceWith({ name: "users/test", activityByDay: true });
    expect(clients.listUserStats).not.toHaveBeenCalled();
  });

  it("opens the eligible guide for an empty account", async () => {
    clients.getUserStats.mockResolvedValue({ totalMemoCount: 0 });
    const { result } = renderHook(() => useWelcomeGuide(true), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.open).toBe(true));
    act(() => result.current.onOpenChange(false));
    expect(result.current.open).toBe(false);
    expect(localStorage.getItem("memos-welcome-guide-dismissed:users/test")).toBe("1");
  });

  it.each(["older", "dismissed", "loading settings", "outside Home"])("does not fetch for the %s guide", (reason) => {
    if (reason === "older") authState.currentUser.createTime = timestampFromDate(new Date(Date.now() - 8 * 86_400_000));
    if (reason === "dismissed") localStorage.setItem("memos-welcome-guide-dismissed:users/test", "1");
    if (reason === "loading settings") authState.isUserSettingsInitialized = false;
    const { result } = renderHook(() => useWelcomeGuide(reason !== "outside Home"), { wrapper: createWrapper() });

    expect(result.current.open).toBe(false);
    act(() => result.current.show());
    expect(result.current.open).toBe(true);
    expect(clients.getUserStats).not.toHaveBeenCalled();
  });

  it("sends a Space filter with per-user statistics", async () => {
    const filter = 'space == "spaces/product"';
    const { result } = renderHook(() => useUserStats("users/test", { filter }), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(clients.getUserStats).toHaveBeenCalledWith({ name: "users/test", filter });
  });

  it("sends an explicit unassigned filter with grouped statistics", async () => {
    const filter = "space == null";
    const { result } = renderHook(() => useAllUserStats({ state: State.ARCHIVED, filter }), { wrapper: createWrapper() });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(clients.listUserStats).toHaveBeenCalledWith(expect.objectContaining({ parent: "users/-", state: State.ARCHIVED, filter }));
  });
});
