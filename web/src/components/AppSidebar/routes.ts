import { matchPath } from "react-router-dom";
import { isMemoCollectionRoute, type MemoScope, resolveMemoScope } from "@/lib/memo-views";
import { ROUTES, resolveCollectionRoute } from "@/router/routes";

export type SidebarRouteKind = MemoScope | "views" | "inbox" | "settings" | "memo" | "common";

export type RouteSearchScope = "route-collection" | "user-collection" | "all";

export interface RouteActionPolicy {
  searchScope: RouteSearchScope;
  /** When absent, Quick Find stays on the current route. */
  searchDestination?: string;
}

export const getSidebarRouteKind = (path: string): SidebarRouteKind => {
  const normalizedPath = resolveCollectionRoute(path).pathname;
  if (isMemoCollectionRoute(normalizedPath)) return resolveMemoScope(normalizedPath);
  if (matchPath(ROUTES.VIEWS, normalizedPath)) return "views";
  if (matchPath(ROUTES.INBOX, normalizedPath)) return "inbox";
  if (matchPath(ROUTES.SETTING, normalizedPath)) return "settings";
  if (matchPath("/memos/:uid", normalizedPath)) return "memo";
  return "common";
};

/**
 * Keeps read/search scope explicit at route boundaries. Compose placement needs no
 * policy: the URL only carries a Space on collection pages.
 */
export const getRouteActionPolicy = (path: string): RouteActionPolicy => {
  const kind = getSidebarRouteKind(path);

  if (kind === "home" || kind === "explore") return { searchScope: "route-collection" };
  if (kind === "archived") return { searchScope: "user-collection" };

  return { searchScope: "all", searchDestination: ROUTES.EXPLORE };
};
