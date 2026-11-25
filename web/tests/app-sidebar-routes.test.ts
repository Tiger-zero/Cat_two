import { describe, expect, it } from "vitest";
import { getSidebarRouteKind } from "@/components/AppSidebar/routes";

describe("sidebar route content", () => {
  it.each([
    ["/", "home"],
    ["/trash", "trash"],
    ["/Trash/", "trash"],
    ["/explore", "common"],
    ["/Explore/", "common"],
    ["/spaces/work", "common"],
    ["/spaces/work/explore", "common"],
    ["/u/steven", "common"],
    ["/U/Steven/", "common"],
    ["/views", "common"],
    ["/Views/", "common"],
    ["/calendar", "common"],
    ["/calendar/2026/08", "common"],
    ["/Calendar/2026/08/02/", "common"],
    ["/attachments", "common"],
    ["/Attachments/", "common"],
    ["/inbox", "common"],
    ["/Inbox/", "common"],
    ["/setting", "common"],
    ["/Setting/", "common"],
    ["/memos/abc", "memo"],
    ["/Memos/ABC/", "memo"],
    ["/memos/shares/token", "common"],
    ["/Memos/Shares/token/", "common"],
    ["/about", "common"],
    ["/403", "common"],
    ["/404", "common"],
    ["/unknown", "common"],
  ])("maps %s to %s content", (path, kind) => {
    expect(getSidebarRouteKind(path)).toBe(kind);
  });
});
