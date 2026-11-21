import { matchPath } from "react-router-dom";
import { extractSpaceUidFromName } from "@/lib/space-display";

export const ROUTES = {
  HOME: "/",
  INBOX: "/inbox",
  ARCHIVED: "/archived",
  VIEWS: "/views",
  SETTING: "/setting",
  EXPLORE: "/explore",
  USER_PROFILE: "/u/:username",
  AUTH: "/auth",
  AUTH_SIGNUP: "/auth/signup",
  AUTH_ADMIN: "/auth/admin",
  AUTH_CALLBACK: "/auth/callback",
} as const;

export type RouteKey = keyof typeof ROUTES;
export type RoutePath = (typeof ROUTES)[RouteKey];

export const SPACE_ROUTE_PATTERN = "/spaces/:spaceUid";

/** Collection pages that exist both globally and beneath a Space. */
const COLLECTION_ROUTE_PATTERNS = [ROUTES.HOME, ROUTES.EXPLORE];

const isCollectionPathname = (pathname: string): boolean =>
  COLLECTION_ROUTE_PATTERNS.some((path) => matchPath({ path, caseSensitive: false }, pathname) !== null);

export interface CollectionRoute {
  spaceName?: string;
  /** The global twin of a collection page; otherwise the normalized pathname. */
  pathname: string;
  isCollection: boolean;
}

/** The URL owns collection scope; unknown Space subroutes are not collection pages. */
export const resolveCollectionRoute = (path: string): CollectionRoute => {
  const pathname = (path.split(/[?#]/, 1)[0] || ROUTES.HOME).replace(/\/+$/, "") || ROUTES.HOME;
  const spaceMatch = matchPath({ path: `${SPACE_ROUTE_PATTERN}/*`, caseSensitive: false }, pathname);
  if (!spaceMatch) return { pathname, isCollection: isCollectionPathname(pathname) };
  const uid = spaceMatch.params.spaceUid ?? "";
  const collectionPath = `/${spaceMatch.params["*"] ?? ""}`;
  // An encoded slash decodes into a UID that cannot round-trip through buildCollectionPath.
  if (!uid || uid.includes("/") || !isCollectionPathname(collectionPath)) return { pathname, isCollection: false };
  return { spaceName: `spaces/${uid}`, pathname: collectionPath, isCollection: true };
};

/** Builds a collection URL from a global collection pathname and an optional Space. */
export const buildCollectionPath = (pathname: string, spaceName?: string): string => {
  const route = resolveCollectionRoute(pathname);
  if (!route.isCollection) return pathname;
  if (!spaceName) return route.pathname;
  const base = `/spaces/${encodeURIComponent(extractSpaceUidFromName(spaceName))}`;
  return route.pathname === ROUTES.HOME ? base : `${base}${route.pathname}`;
};

/** An explicit creator is a URL-owned collection filter, independent of the selected Space. */
export const getCollectionCreator = (search: string): string | undefined => {
  const username = new URLSearchParams(search).get("creator");
  return username || undefined;
};

export const withCollectionCreator = (search: string, username?: string): string => {
  const params = new URLSearchParams(search);
  if (username) params.set("creator", username);
  else params.delete("creator");
  const query = params.toString();
  return query ? `?${query}` : "";
};

/** The canonical all-Space collection for one creator. */
export const getCreatorHomePath = (username: string): string => `${ROUTES.HOME}${withCollectionCreator("", username)}`;

/** Keep the active creator scope when opening another collection. */
export const getCollectionHomePath = (location: { pathname: string }): string => {
  const route = resolveCollectionRoute(location.pathname);
  const home = route.pathname.toLowerCase() === ROUTES.EXPLORE ? ROUTES.EXPLORE : ROUTES.HOME;
  return buildCollectionPath(home, route.spaceName);
};

/** Opens the memo list while carrying the current collection filters and creator. */
export const collectionNavigationPath = (location: { pathname: string; search: string }, currentUsername?: string): string => {
  const route = resolveCollectionRoute(location.pathname);
  const creator =
    route.pathname.toLowerCase() === ROUTES.EXPLORE
      ? undefined
      : ((route.isCollection ? getCollectionCreator(location.search) : undefined) ?? currentUsername);
  const filter = route.isCollection ? new URLSearchParams(location.search).get("filter") : null;
  const search = filter ? new URLSearchParams({ filter }).toString() : "";
  const home = buildCollectionPath(creator ? ROUTES.HOME : ROUTES.EXPLORE, route.spaceName);
  return `${home}${withCollectionCreator(search, creator === currentUsername ? undefined : creator)}`;
};

/** A creator change keeps the current Space and filters. */
export const getCreatorSwitchPath = (
  location: { pathname: string; search: string; hash?: string },
  username?: string,
  currentUsername?: string,
): string => {
  const route = resolveCollectionRoute(location.pathname);
  const pathname = buildCollectionPath(username ? ROUTES.HOME : ROUTES.EXPLORE, route.spaceName);
  const creator = username === currentUsername ? undefined : username;
  return `${pathname}${withCollectionCreator(location.search, creator)}${location.hash || ""}`;
};

/** Switching preserves collection views and their query; other pages start at Home. */
export const getSpaceSwitchPath = (location: { pathname: string; search: string; hash?: string }, spaceName?: string): string => {
  const route = resolveCollectionRoute(location.pathname);
  return route.isCollection
    ? `${buildCollectionPath(route.pathname, spaceName)}${location.search}${location.hash || ""}`
    : buildCollectionPath(ROUTES.HOME, spaceName);
};
