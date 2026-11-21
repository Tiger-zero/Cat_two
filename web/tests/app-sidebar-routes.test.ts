import { describe, expect, it } from "vitest";
import { getRouteActionPolicy, getSidebarRouteKind } from "@/components/AppSidebar/routes";

describe("sidebar route content", () => {
  it.each([
    ["/", "home"],
    ["/archived", "archived"],
    ["/explore", "explore"],
    ["/Explore/", "explore"],
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
    ["/inbox", "inbox"],
    ["/Inbox/", "inbox"],
    ["/setting", "settings"],
    ["/Setting/", "settings"],
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

  it.each(["/", "/explore"])("keeps search in the route collection on %s", (path) => {
    expect(getRouteActionPolicy(path)).toEqual({
      searchScope: "route-collection",
    });
  });

  it.each(["/archived", "/ARCHIVED/"])("keeps %s in the user archive", (path) => {
    expect(getRouteActionPolicy(path)).toEqual({
      searchScope: "user-collection",
    });
  });

  it("keeps a normalized Explore route in the route scope", () => {
    const path = "/Explore/";
    expect(getRouteActionPolicy(path)).toEqual({
      searchScope: "route-collection",
    });
  });

  it.each([
    "/inbox",
    "/setting",
    "/views",
    "/about",
    "/memos/abc",
    "/memos/shares/token",
    "/403",
    "/404",
    "/unknown",
  ])("sends search to All on %s", (path) => {
    expect(getRouteActionPolicy(path)).toEqual({
      searchScope: "all",
      searchDestination: "/explore",
    });
  });
});
