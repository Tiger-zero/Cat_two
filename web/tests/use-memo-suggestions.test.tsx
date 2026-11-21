import { renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { MemoFilter } from "@/contexts/MemoFilterContext";
import { useMemoSuggestions } from "@/hooks/useMemoSuggestions";

const mocks = vi.hoisted(() => ({
  filters: [] as MemoFilter[],
}));
vi.mock("@/contexts/MemoFilterContext", () => ({ useMemoFilterContext: () => mocks }));

describe("useMemoSuggestions", () => {
  beforeEach(() => {
    mocks.filters = [];
  });

  it("suggests a checklist from a task search", () => {
    mocks.filters = [{ factor: "celSearch", value: "has_task_list && has_incomplete_tasks" }];
    const { result } = renderHook(() => useMemoSuggestions());
    expect(result.current).toEqual([{ id: "checklist", kind: "checklist", source: "search" }]);
  });

  it("reacts to changes in selected tags and search filters", () => {
    const { result, rerender } = renderHook(() => useMemoSuggestions());
    expect(result.current).toEqual([]);
    mocks.filters = [
      { factor: "tagSearch", value: "release" },
      { factor: "celSearch", value: '"planning" in tags' },
    ];
    rerender();
    expect(result.current.map((suggestion) => suggestion.id)).toEqual(["tag:release", "tag:planning"]);
    mocks.filters = [{ factor: "tagSearch", value: "release" }];
    rerender();
    expect(result.current.map((suggestion) => suggestion.id)).toEqual(["tag:release"]);
  });
});
