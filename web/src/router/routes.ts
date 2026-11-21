export const ROUTES = {
  HOME: "/",
  ARCHIVED: "/archived",
  TRASH: "/trash",
  VIEWS: "/views",
  SETTING: "/setting",
  AUTH: "/auth",
  AUTH_SIGNUP: "/auth/signup",
  AUTH_ADMIN: "/auth/admin",
} as const;

export type RouteKey = keyof typeof ROUTES;
export type RoutePath = (typeof ROUTES)[RouteKey];

/** Keep filters from old collection links while retiring URL-owned creator scopes. */
export const personalFeedSearch = (search: string): string => {
  const params = new URLSearchParams(search);
  if (!params.has("creator")) return search;
  params.delete("creator");
  return params.size ? `?${params}` : "";
};
