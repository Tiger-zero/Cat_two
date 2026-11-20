import { describe, expect, it } from "vitest";
import { buildMemoFilter } from "@/hooks/useMemoFilters";
import { combineCELFilters } from "@/lib/cel-filter";
import { getMemoViewId, isMemoCollectionRoute, isMemoScopeRoute, resolveMemoScope } from "@/lib/memo-views";

describe("memo scopes", () => {
  it("resolves collection and detail routes", () => {
    expect(resolveMemoScope("/archived")).toBe("archived");
    expect(resolveMemoScope("/explore")).toBe("explore");
    expect(resolveMemoScope("/memos/123", { detailFrom: "/archived?filter=tagSearch%3Awork" })).toBe("archived");
    expect(resolveMemoScope("/memos/123", { memoArchived: true })).toBe("archived");
    expect(resolveMemoScope("/settings", { fallback: "explore" })).toBe("explore");
  });

  it("recognizes Home, Archived, and the legacy Explore alias", () => {
    expect(isMemoScopeRoute("/")).toBe(true);
    expect(isMemoScopeRoute("/explore")).toBe(true);
    expect(isMemoScopeRoute("/archived")).toBe(true);
    expect(isMemoScopeRoute("/attachments")).toBe(false);
  });

  it("treats Home and calendar as collection routes where views and filters apply in place", () => {
    expect(isMemoCollectionRoute("/")).toBe(true);
    expect(isMemoCollectionRoute("/archived")).toBe(true);
    expect(isMemoCollectionRoute("/?creator=steven")).toBe(true);
    expect(isMemoCollectionRoute("/u/steven")).toBe(false);
    expect(isMemoCollectionRoute("/calendar")).toBe(true);
    expect(isMemoCollectionRoute("/Calendar/2026/08/02/")).toBe(true);
    expect(isMemoCollectionRoute("/calendars")).toBe(false);
    expect(isMemoScopeRoute("/u/steven")).toBe(false);
    expect(isMemoCollectionRoute("/memos/123")).toBe(false);
    expect(isMemoCollectionRoute("/attachments")).toBe(false);
  });
});

describe("memo views", () => {
  it("combines the creation-channel facet with the search and ignores unsupported channel inputs", () => {
    expect(
      buildMemoFilter({
        filters: [
          { factor: "source", value: "web" },
          { factor: "contentSearch", value: "plan" },
        ],
        includePinned: false,
      }),
    ).toBe('(creation_channel == "web") && (content.contains("plan"))');
    expect(buildMemoFilter({ filters: [{ factor: "source", value: "not-supported" }], includePinned: false })).toBeUndefined();
  });

  it("keeps OR expressions inside the author, Space, view, and facet constraints", () => {
    const query = 'content.contains("plan") || pinned';
    const filter = buildMemoFilter({
      creatorName: "users/steven",
      selectedMemoViewFilter: "has_link || has_code",
      filters: [
        { factor: "celSearch", value: query },
        { factor: "tagSearch", value: "work" },
      ],
      includePinned: true,
    });
    expect(combineCELFilters('space == "spaces/product"', filter)).toBe(
      '(space == "spaces/product") && ((creator == "users/steven") && (has_link || has_code) && (content.contains("plan") || pinned) && (tag in ["work"]))',
    );
  });
  it("extracts the saved view ID", () => {
    expect(getMemoViewId("users/steven/views/work")).toBe("work");
  });

  it("composes a saved task view with search, tags, and creator", () => {
    expect(
      buildMemoFilter({
        creatorName: "users/steven",
        selectedMemoViewFilter: "has_task_list && has_incomplete_tasks",
        filters: [
          { factor: "contentSearch", value: "plan" },
          { factor: "tagSearch", value: "work" },
        ],
        includePinned: false,
      }),
    ).toBe('(creator == "users/steven") && (has_task_list && has_incomplete_tasks) && (content.contains("plan")) && (tag in ["work"])');
  });

  it("maps property filter factors to their CEL flags", () => {
    expect(
      buildMemoFilter({
        filters: [
          { factor: "property.hasLink", value: "" },
          { factor: "property.hasTaskList", value: "" },
          { factor: "property.hasCode", value: "" },
          { factor: "property.hasAttachments", value: "" },
          { factor: "property.hasImage", value: "" },
        ],
        includePinned: false,
      }),
    ).toBe("(has_link) && (has_task_list) && (has_code) && (has_attachments) && (has_image)");
  });

  it("uses a saved memo view filter", () => {
    expect(
      buildMemoFilter({
        filters: [],
        includePinned: false,
        selectedMemoViewFilter: 'tag in ["work"]',
      }),
    ).toBe('(tag in ["work"])');
  });

  it("builds display-time filters from Shanghai calendar-day boundaries", () => {
    const start = new Date("2026-08-01T16:00:00Z");
    const end = new Date("2026-08-02T16:00:00Z");

    expect(
      buildMemoFilter({
        filters: [{ factor: "displayTime", value: "2026-08-02" }],
        includePinned: false,
      }),
    ).toBe(
      `(created_ts >= timestamp(${Math.floor(start.getTime() / 1000)}) && created_ts < timestamp(${Math.floor(end.getTime() / 1000)}))`,
    );
  });

  it("selects on update time when that is the basis", () => {
    const start = new Date("2026-08-01T16:00:00Z");
    const end = new Date("2026-08-02T16:00:00Z");

    expect(
      buildMemoFilter({
        filters: [{ factor: "displayTime", value: "2026-08-02" }],
        includePinned: false,
        timeBasis: "update_time",
      }),
    ).toBe(
      `(updated_ts >= timestamp(${Math.floor(start.getTime() / 1000)}) && updated_ts < timestamp(${Math.floor(end.getTime() / 1000)}))`,
    );
  });

  it("ignores invalid display-time filter values", () => {
    expect(
      buildMemoFilter({
        filters: [{ factor: "displayTime", value: "2026-02-30" }],
        includePinned: false,
      }),
    ).toBeUndefined();
  });
});
