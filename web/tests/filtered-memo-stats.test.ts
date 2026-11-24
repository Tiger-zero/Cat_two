import { renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// Mock dependencies BEFORE importing the hook under test.
vi.mock("@/hooks/useUserQueries", () => ({
  useAllUserStats: vi.fn(),
  useUserStats: vi.fn(),
}));
vi.mock("@/hooks/useMemoQueries", () => ({
  useMemos: () => ({ data: undefined, isLoading: false }),
}));
vi.mock("@/hooks/useCurrentUser", () => ({
  default: () => ({ name: "users/test", id: 1 }),
}));

const mockUseView = vi.fn();
vi.mock("@/contexts/ViewContext", async () => {
  const actual = await vi.importActual<typeof import("@/contexts/ViewContext")>("@/contexts/ViewContext");
  return {
    ...actual,
    useView: () => mockUseView(),
  };
});

import { useFilteredMemoStats } from "@/hooks/useFilteredMemoStats";
import { useAllUserStats, useUserStats } from "@/hooks/useUserQueries";
import { State } from "@/types/proto/api/common_pb";

const wrapper = ({ children }: { children: ReactNode }) => children as never;

// Shanghai midnight is 16:00 UTC on the previous day.
const ts = (year: number, month: number, day: number) => ({
  seconds: BigInt(Date.UTC(year, month - 1, day - 1, 16) / 1000),
  nanos: 0,
});

describe("useFilteredMemoStats", () => {
  beforeEach(() => {
    vi.mocked(useAllUserStats).mockReturnValue({
      data: [],
      isLoading: false,
    } as unknown as ReturnType<typeof useAllUserStats>);
    vi.mocked(useUserStats).mockReturnValue({
      data: {
        memoCreatedTimestamps: [ts(2026, 5, 1), ts(2026, 5, 1), ts(2026, 5, 2)],
        memoUpdatedTimestamps: [ts(2026, 5, 3), ts(2026, 5, 3), ts(2026, 5, 3)],
        tagCounts: {},
      },
      isLoading: false,
    } as unknown as ReturnType<typeof useUserStats>);
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it("aggregates by created timestamps when timeBasis is create_time", () => {
    mockUseView.mockReturnValue({
      timeBasis: "create_time",
      orderByTimeAsc: false,
      setOrderByTimeAsc: vi.fn(),
      setTimeBasis: vi.fn(),
    });

    const { result } = renderHook(() => useFilteredMemoStats({ userName: "users/test" }), { wrapper });

    expect(result.current.statistics.activityStats).toEqual({ "2026-05-01": 2, "2026-05-02": 1 });
    expect(result.current.statistics.timeBasis).toBe("create_time");
  });

  it("aggregates by updated timestamps when timeBasis is update_time", () => {
    mockUseView.mockReturnValue({
      timeBasis: "update_time",
      orderByTimeAsc: false,
      setOrderByTimeAsc: vi.fn(),
      setTimeBasis: vi.fn(),
    });

    const { result } = renderHook(() => useFilteredMemoStats({ userName: "users/test" }), { wrapper });

    expect(result.current.statistics.activityStats).toEqual({ "2026-05-03": 3 });
    expect(result.current.statistics.timeBasis).toBe("update_time");
  });

  it("counts unique Shanghai creation days and tags in the collection regardless of calendar time basis", () => {
    mockUseView.mockReturnValue({ timeBasis: "update_time" });
    const midnight = ts(2026, 5, 2);
    vi.mocked(useAllUserStats).mockReturnValue({
      data: [
        {
          totalMemoCount: 3,
          tagCounts: { work: 2, "work/project": 1 },
          memoCreatedTimestamps: [{ ...midnight, seconds: midnight.seconds - 1n }, midnight, midnight],
          memoUpdatedTimestamps: [ts(2026, 5, 3), ts(2026, 5, 3), ts(2026, 5, 3)],
        },
      ],
      isLoading: false,
    } as unknown as ReturnType<typeof useAllUserStats>);

    const { result, rerender } = renderHook(() => useFilteredMemoStats({ context: "collection" }), { wrapper });

    expect(result.current.statistics.activityStats).toEqual({ "2026-05-03": 3 });
    expect(result.current.summary).toEqual({ memoCount: 3, tagCount: 2, recordedDays: 2 });

    mockUseView.mockReturnValue({ timeBasis: "create_time" });
    rerender();
    expect(result.current.statistics.activityStats).toEqual({ "2026-05-01": 1, "2026-05-02": 2 });
    expect(result.current.summary).toEqual({ memoCount: 3, tagCount: 2, recordedDays: 2 });
  });

  it("aggregates large timestamp groups across users without overflowing the argument stack", () => {
    const count = 150_000;
    const firstDay = ts(2026, 5, 1);
    const secondDay = ts(2026, 5, 2);
    mockUseView.mockReturnValue({ timeBasis: "update_time" });
    vi.mocked(useAllUserStats).mockReturnValue({
      data: [
        { memoCreatedTimestamps: [], memoUpdatedTimestamps: Array(count).fill(firstDay), tagCounts: { work: 2 } },
        { memoCreatedTimestamps: [], memoUpdatedTimestamps: [firstDay, secondDay], tagCounts: { work: 1, personal: 1 } },
      ],
      isLoading: false,
    } as unknown as ReturnType<typeof useAllUserStats>);

    const { result } = renderHook(() => useFilteredMemoStats({ context: "collection" }), { wrapper });

    expect(result.current.statistics.activityStats).toEqual({ "2026-05-01": count + 1, "2026-05-02": 1 });
    expect(result.current.tags).toEqual({ work: 3, personal: 1 });
  });

  it("falls back to created timestamps when updated array is empty (old server)", () => {
    // Old servers that don't know about the new field deserialize it as [].
    // Length divergence (created non-empty, updated empty) is the signal.
    vi.mocked(useUserStats).mockReturnValue({
      data: {
        memoCreatedTimestamps: [ts(2026, 5, 1)],
        memoUpdatedTimestamps: [],
        tagCounts: {},
        totalMemoCount: 1,
      },
      isLoading: false,
    } as unknown as ReturnType<typeof useUserStats>);
    mockUseView.mockReturnValue({
      timeBasis: "update_time",
      orderByTimeAsc: false,
      setOrderByTimeAsc: vi.fn(),
      setTimeBasis: vi.fn(),
    });

    const warn = vi.spyOn(console, "warn").mockImplementation(() => {});
    const { result } = renderHook(() => useFilteredMemoStats({ context: "home", userName: "users/test" }), { wrapper });

    expect(result.current.statistics.activityStats).toEqual({ "2026-05-01": 1 });
    expect(result.current.summary).toEqual({ memoCount: 1, tagCount: 0, recordedDays: 1 });
    expect(warn).toHaveBeenCalled();
    warn.mockRestore();
  });

  it("disables both statistics queries when deferred", () => {
    mockUseView.mockReturnValue({
      timeBasis: "create_time",
      orderByTimeAsc: false,
      setOrderByTimeAsc: vi.fn(),
      setTimeBasis: vi.fn(),
    });

    renderHook(() => useFilteredMemoStats({ userName: "users/test", context: "collection", enabled: false }), { wrapper });

    expect(useUserStats).toHaveBeenCalledWith("users/test", { enabled: false, filter: undefined });
    expect(useAllUserStats).toHaveBeenCalledWith(expect.anything(), { enabled: false });
  });

  it("passes the selected Space filter to Home statistics", () => {
    mockUseView.mockReturnValue({
      timeBasis: "create_time",
      orderByTimeAsc: false,
      setOrderByTimeAsc: vi.fn(),
      setTimeBasis: vi.fn(),
    });
    const filter = 'space == "spaces/product"';

    renderHook(() => useFilteredMemoStats({ userName: "users/test", context: "home", filter }), { wrapper });

    expect(useUserStats).toHaveBeenCalledWith("users/test", { enabled: true, filter });
  });

  it("scopes collection statistics to a Space without excluding readable private memos", () => {
    mockUseView.mockReturnValue({
      timeBasis: "create_time",
      orderByTimeAsc: false,
      setOrderByTimeAsc: vi.fn(),
      setTimeBasis: vi.fn(),
    });
    const filter = 'space == "spaces/product"';

    renderHook(() => useFilteredMemoStats({ context: "collection", filter }), { wrapper });

    expect(useAllUserStats).toHaveBeenCalledWith(
      {
        state: State.NORMAL,
        filter,
      },
      { enabled: true },
    );
  });

  it("leaves the creator and visibility filters to access control in the All collection", () => {
    mockUseView.mockReturnValue({
      timeBasis: "create_time",
      orderByTimeAsc: false,
      setOrderByTimeAsc: vi.fn(),
      setTimeBasis: vi.fn(),
    });

    renderHook(() => useFilteredMemoStats({ context: "collection" }), { wrapper });

    expect(useAllUserStats).toHaveBeenCalledWith(
      {
        state: State.NORMAL,
        filter: undefined,
      },
      { enabled: true },
    );
  });

  it("supports an explicit unassigned filter", () => {
    mockUseView.mockReturnValue({
      timeBasis: "create_time",
      orderByTimeAsc: false,
      setOrderByTimeAsc: vi.fn(),
      setTimeBasis: vi.fn(),
    });
    const filter = "space == null";

    renderHook(() => useFilteredMemoStats({ context: "archived", filter }), { wrapper });

    expect(useAllUserStats).toHaveBeenCalledWith({ state: State.ARCHIVED, filter }, { enabled: true });
  });
});
