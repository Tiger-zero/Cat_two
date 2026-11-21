import { describe, expect, it } from "vitest";
import { getRouteActionPolicy, getSidebarRouteKind } from "@/components/AppSidebar/routes";

describe("sidebar route content", () => {
  it.each([
    ["/", "home"],
    ["/archived", "archived"],
    ["/explore", "common"],
    ["/Explore/", "common"],
    ["/spaces/work", "common"],
    ["/spaces/work/explore", "common"],
    ["/ARCHIVED/", "archived"],
    ["/u/steven", "common"],
    ["/U/Steven/", "common"],
    ["/views", "views"],
    ["/Views/", "views"],
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

  it("keeps search in the personal feed", () => {
    expect(getRouteActionPolicy("/")).toEqual({
      searchScope: "route-collection",
    });
  });

  it.each(["/archived", "/ARCHIVED/"])("keeps %s in the user archive", (path) => {
    expect(getRouteActionPolicy(path)).toEqual({
      searchScope: "user-collection",
    });
  });

  it.each([
    "/inbox",
    "/explore",
    "/spaces/work",
    "/setting",
    "/views",
    "/about",
    "/memos/abc",
    "/memos/shares/token",
    "/403",
    "/404",
    "/unknown",
  ])("sends search to the personal feed from %s", (path) => {
    expect(getRouteActionPolicy(path)).toEqual({
      searchScope: "all",
      searchDestination: "/",
    });
  });
});
