import { ROUTES } from "@/router/routes";

export type MemoScope = "home" | "archived";

export const getMemoViewId = (name: string): string => {
  const parts = name.split("/");
  return parts.length === 4 ? parts[3] : name;
};

const cleanPathname = (value: string): string => {
  const pathname = value.split(/[?#]/, 1)[0] || ROUTES.HOME;
  return (pathname.replace(/\/+$/, "") || ROUTES.HOME).toLowerCase();
};

/** Routes that display a memo list with the sidebar's view, date and tag filters. */
export const isMemoCollectionRoute = (pathname: string): boolean => {
  const cleanPath = cleanPathname(pathname);
  return cleanPath === ROUTES.HOME || cleanPath === ROUTES.ARCHIVED;
};

interface ResolveMemoScopeOptions {
  detailFrom?: string;
  memoArchived?: boolean;
  fallback?: MemoScope;
}

export const resolveMemoScope = (pathname: string, options: ResolveMemoScopeOptions = {}): MemoScope => {
  const cleanPath = cleanPathname(pathname);
  if (cleanPath === ROUTES.ARCHIVED) return "archived";
  if (cleanPath === ROUTES.HOME) return "home";

  if (cleanPath.startsWith("/memos/") && options.detailFrom) {
    return resolveMemoScope(options.detailFrom, {
      fallback: options.fallback,
    });
  }

  if (cleanPath.startsWith("/memos/") && options.memoArchived) {
    return "archived";
  }

  return options.fallback ?? "home";
};
