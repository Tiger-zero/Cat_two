import { matchPath } from "react-router-dom";
import { isMemoCollectionRoute, type MemoScope, resolveMemoScope } from "@/lib/memo-views";
import { ROUTES } from "@/router/routes";

export type SidebarRouteKind = MemoScope | "views" | "settings" | "memo" | "common";

export type RouteSearchScope = "route-collection" | "user-collection" | "all";

export interface RouteActionPolicy {
  searchScope: RouteSearchScope;
  /** When absent, Quick Find stays on the current route. */
  searchDestination?: string;
}

export const getSidebarRouteKind = (path: string): SidebarRouteKind => {
  const normalizedPath = path.split(/[?#]/, 1)[0];
  if (isMemoCollectionRoute(normalizedPath)) return resolveMemoScope(normalizedPath);
  if (matchPath(ROUTES.VIEWS, normalizedPath)) return "views";
  if (matchPath(ROUTES.SETTING, normalizedPath)) return "settings";
  if (matchPath("/memos/:uid", normalizedPath)) return "memo";
  return "common";
};

/** Searches in the active feed, or opens the personal feed from other pages. */
export const getRouteActionPolicy = (path: string): RouteActionPolicy => {
  const kind = getSidebarRouteKind(path);

  if (kind === "home") return { searchScope: "route-collection" };
  if (kind === "archived") return { searchScope: "user-collection" };

  return { searchScope: "all", searchDestination: ROUTES.HOME };
};
