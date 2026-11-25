import { describe, expect, it } from "vitest";
import { buildQuickFindFilters } from "@/components/AppSidebar/QuickFindDialog";
import { getFilterSearch, type MemoFilter, parseFilterQuery, replaceFiltersByFactor } from "@/contexts/MemoFilterContext";

describe("Quick Find", () => {
  const scopedFilters: MemoFilter[] = [
    { factor: "tagSearch", value: "work" },
    { factor: "displayTime", value: "2026-08-03" },
    { factor: "contentSearch", value: "old" },
  ];

  it.each([
    "content.contains('urgent')",
    'tags.exists(t, t.startsWith("work/"))',
    'content.matches("(?i)urgent")',
    'content.contains("50%, café & C++")\n || pinned',
  ])("retains CEL verbatim through a navigation URL: %s", (expression) => {
    const filters = buildQuickFindFilters(expression, [], "cel");
    const decoded = parseFilterQuery(new URLSearchParams(getFilterSearch(filters)).get("filter"));
    expect(decoded).toEqual([{ factor: "celSearch", value: expression }]);
  });

  it("replaces both kinds of search without dropping facets", () => {
    const filters: MemoFilter[] = [...scopedFilters, { factor: "celSearch", value: "pinned" }];
    expect(buildQuickFindFilters("has_link", filters, "cel")).toEqual([
      ...scopedFilters.slice(0, 2),
      { factor: "celSearch", value: "has_link" },
    ]);
    expect(buildQuickFindFilters("new new", filters, "text")).toEqual([
      ...scopedFilters.slice(0, 2),
      { factor: "contentSearch", value: "new" },
    ]);
    expect(buildQuickFindFilters(" \n ", filters, "cel")).toEqual(scopedFilters.slice(0, 2));
    expect(buildQuickFindFilters(" ", filters, "text")).toEqual(scopedFilters.slice(0, 2));
  });

  it("does not recognize expressions in Text mode", () => {
    expect(buildQuickFindFilters('content.contains("urgent")', [], "text")).toEqual([
      { factor: "contentSearch", value: 'content.contains("urgent")' },
    ]);
  });

  it("replaces search terms while preserving date and tag facets in a collection", () => {
    expect(buildQuickFindFilters("project plan project", scopedFilters, "text")).toEqual([
      { factor: "tagSearch", value: "work" },
      { factor: "displayTime", value: "2026-08-03" },
      { factor: "contentSearch", value: "project" },
      { factor: "contentSearch", value: "plan" },
    ]);
  });

  it("replaces only the selected date facet", () => {
    expect(replaceFiltersByFactor(scopedFilters, "displayTime", [{ factor: "displayTime", value: "2026-08-02" }])).toEqual([
      { factor: "tagSearch", value: "work" },
      { factor: "contentSearch", value: "old" },
      { factor: "displayTime", value: "2026-08-02" },
    ]);
  });

  it("replaces tag filters with untagged while keeping the date and new search terms", () => {
    expect(buildQuickFindFilters("fresh plan", scopedFilters, "text", true)).toEqual([
      { factor: "displayTime", value: "2026-08-03" },
      { factor: "contentSearch", value: "fresh" },
      { factor: "contentSearch", value: "plan" },
      { factor: "untagged", value: "" },
    ]);
  });
});
