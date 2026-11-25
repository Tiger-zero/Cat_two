import { matchPath } from "react-router-dom";
import { ROUTES } from "@/router/routes";

export type SidebarRouteKind = "home" | "trash" | "memo" | "common";

export const getSidebarRouteKind = (path: string): SidebarRouteKind => {
  const normalizedPath = path.split(/[?#]/, 1)[0];
  if (matchPath(ROUTES.HOME, normalizedPath)) return "home";
  if (matchPath(ROUTES.TRASH, normalizedPath)) return "trash";
  if (matchPath("/memos/:uid", normalizedPath)) return "memo";
  return "common";
};
