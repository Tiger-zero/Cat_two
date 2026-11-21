import { render, waitFor } from "@testing-library/react";
import { isValidElement } from "react";
import { createMemoryRouter, matchRoutes, type RouteObject, RouterProvider } from "react-router-dom";
import { describe, expect, it } from "vitest";
import { ROUTES, routeConfig } from "@/router";
import { RequireAuthRoute, RequireFullInitializationRoute, RequireGuestRoute, RequireInstanceInitializationRoute } from "@/router/guards";
import { LegacyCalendarRedirect } from "@/router/LegacyCalendarRedirect";

// Walk the nested route config and find the first route with the given path,
// starting from the provided roots. Returns undefined if nothing matches.
function findByPath(routes: RouteObject[], path: string): RouteObject | undefined {
  for (const route of routes) {
    if (route.path === path) return route;
    const hit = route.children ? findByPath(route.children, path) : undefined;
    if (hit) return hit;
  }
  return undefined;
}

function elementType(route: RouteObject | undefined): unknown {
  if (!route?.element || !isValidElement(route.element)) return undefined;
  return route.element.type;
}

function hasAncestorOfType(routes: RouteObject[], path: string, guardType: unknown): boolean {
  const walk = (subtree: RouteObject[], ancestorGuards: unknown[]): boolean => {
    for (const route of subtree) {
      const nextAncestors = [...ancestorGuards];
      const type = elementType(route);
      if (type) nextAncestors.push(type);
      if (route.path === path) {
        return nextAncestors.includes(guardType);
      }
      if (route.children && walk(route.children, nextAncestors)) {
        return true;
      }
    }
    return false;
  };
  return walk(routes, []);
}

describe("router configuration", () => {
  it.each(["/", "/explore", "/calendar/2026/09/30"])("checks authentication before mounting the workspace for %s", (path) => {
    const matches = matchRoutes(routeConfig, path);
    // The first match is App. Check auth before mounting any workspace layout.
    expect(elementType(matches?.[1]?.route)).toBe(RequireAuthRoute);
  });

  it.each([
    "/home",
    "/explore",
    "/inbox",
    "/spaces/product",
    "/spaces/product/explore",
    "/u/alice",
  ])("redirects the retired %s address to the authenticated personal feed", async (path) => {
    const source = `${path}?creator=alice&filter=tagSearch%3Awork#anchor`;
    const matches = matchRoutes(routeConfig, source);
    expect(matches?.map(({ route }) => elementType(route))).toContain(RequireAuthRoute);
    const router = createMemoryRouter(
      [
        { path, element: matches?.at(-1)?.route.element },
        { path: "/", element: <p>Memo list</p> },
      ],
      { initialEntries: [source] },
    );
    render(<RouterProvider router={router} />);
    await waitFor(() =>
      expect(router.state.location).toMatchObject({ pathname: "/", search: "?filter=tagSearch%3Awork", hash: "#anchor" }),
    );
    expect(router.state.historyAction).toBe("REPLACE");
  });

  it.each([
    ["/calendar", "/"],
    ["/calendar/2026/9/6", "/?filter=displayTime%3A2026-09-06"],
    ["/spaces/product/calendar/2026/09?filter=tagSearch%3Awork", "/?filter=tagSearch%3Awork"],
    [
      "/spaces/product/calendar/2026/9/6?creator=alice&filter=tagSearch%3Awork%2CdisplayTime%3A2026-09-05#anchor",
      "/?filter=tagSearch%3Awork%2CdisplayTime%3A2026-09-06#anchor",
    ],
    ["/calendar/2026/02/30?creator=alice", "/"],
    ["/spaces/a%2Fb/calendar/2026/9/6", "/404"],
  ])("redirects %s to %s", async (source, destination) => {
    const matches = matchRoutes(routeConfig, source);
    const redirect = matches?.at(-1)?.route;
    expect(elementType(redirect)).toBe(LegacyCalendarRedirect);
    expect(matches?.map(({ route }) => elementType(route))).toContain(RequireAuthRoute);

    const spacePrefix = source.startsWith("/spaces/") ? "/spaces/:spaceUid/" : "/";
    const router = createMemoryRouter(
      [
        { path: `${spacePrefix}${redirect?.path}`, element: redirect?.element },
        { path: "*", element: <p>Memo list</p> },
      ],
      { initialEntries: [source] },
    );
    render(<RouterProvider router={router} />);
    await waitFor(() => {
      const { pathname, search, hash } = router.state.location;
      expect(`${pathname}${search}${hash}`).toBe(destination);
      expect(router.state.historyAction).toBe("REPLACE");
    });
  });

  it("keeps unknown Space subroutes outside collection pages", () => {
    const matches = matchRoutes(routeConfig, "/spaces/product/unknown");
    expect(matches?.at(-1)?.route.path).toBe("*");
  });

  it.each(["/attachments", "/spaces/product/attachments"])("uses the not-found route for %s", (path) => {
    const matches = matchRoutes(routeConfig, path);
    expect(matches?.at(-1)?.route.path).toBe("*");
  });

  it("uses the not-found route for the retired SSO callback", () => {
    expect(matchRoutes(routeConfig, "/auth/callback")?.at(-1)?.route.path).toBe("*");
  });

  it("wraps the remaining /auth children in RequireGuestRoute", () => {
    for (const path of ["", "admin", "signup"]) {
      expect(hasAncestorOfType(routeConfig, path, RequireGuestRoute)).toBe(true);
      expect(hasAncestorOfType(routeConfig, path, RequireInstanceInitializationRoute)).toBe(true);
    }
  });

  it("wraps authenticated-only pages in RequireAuthRoute", () => {
    for (const path of [ROUTES.ARCHIVED, ROUTES.TRASH, ROUTES.VIEWS, ROUTES.SETTING, "memos/:uid"]) {
      expect(hasAncestorOfType(routeConfig, path, RequireAuthRoute)).toBe(true);
    }
  });

  it("keeps non-feed pages behind their full initialization requirements", () => {
    for (const path of [ROUTES.VIEWS, ROUTES.SETTING]) {
      expect(hasAncestorOfType(routeConfig, path, RequireFullInitializationRoute)).toBe(true);
    }
  });

  it("preserves the Settings section in links from before the modal", async () => {
    const router = createMemoryRouter(
      [
        { path: ROUTES.SETTING, element: findByPath(routeConfig, ROUTES.SETTING)?.element },
        { path: ROUTES.HOME, element: <p>Memo list</p> },
      ],
      { initialEntries: ["/setting#access-token"] },
    );
    render(<RouterProvider router={router} />);
    await waitFor(() => expect(router.state.location).toMatchObject({ pathname: "/", search: "?settings=access-token", hash: "" }));
    expect(router.state.historyAction).toBe("REPLACE");
  });

  it("does not register a dedicated new memo route", () => {
    expect(findByPath(routeConfig, "/new")).toBeUndefined();
  });

  it("does not register an About page", () => {
    expect(findByPath(routeConfig, "/about")).toBeUndefined();
  });

  it("does not register a public share token route", () => {
    expect(findByPath(routeConfig, "memos/shares/:token")).toBeUndefined();
  });

  it("leaves memo feeds available for early queries", () => {
    for (const path of [ROUTES.ARCHIVED, "memos/:uid"]) {
      expect(hasAncestorOfType(routeConfig, path, RequireFullInitializationRoute)).toBe(false);
    }
  });

  it("leaves public pages outside RequireAuthRoute", () => {
    for (const path of ["403", "404"]) {
      expect(hasAncestorOfType(routeConfig, path, RequireAuthRoute)).toBe(false);
    }
  });
});
