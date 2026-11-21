import { createBrowserRouter, Navigate, type RouteObject, useLocation } from "react-router-dom";
import App from "@/App";
import { ChunkLoadErrorFallback } from "@/components/ErrorBoundary";
import { preloadMemoEditing } from "@/components/MemoEditor";
import MainLayout from "@/layouts/MainLayout";
import RootLayout from "@/layouts/RootLayout";
import { lazyWithReload } from "@/utils/lazy";
import { RequireAuthRoute, RequireFullInitializationRoute, RequireGuestRoute, RequireInstanceInitializationRoute } from "./guards";
import { LegacyCalendarRedirect } from "./LegacyCalendarRedirect";
import { personalFeedSearch, ROUTES, SETTINGS_SECTION_PARAM } from "./routes";

const AdminSignIn = lazyWithReload(() => import("@/pages/AdminSignIn"));
const Archived = lazyWithReload(() => import("@/pages/Archived"));
// A signed-in user's Home composes at once, so the page's code arrives together with the
// editing session's and the composer renders with the page, never after it.
const Home = lazyWithReload(() => Promise.all([import("@/pages/Home"), preloadMemoEditing()]).then(([page]) => page));
const MemoDetail = lazyWithReload(() => import("@/pages/MemoDetail"));
const NotFound = lazyWithReload(() => import("@/pages/NotFound"));
const PermissionDenied = lazyWithReload(() => import("@/pages/PermissionDenied"));
const SignIn = lazyWithReload(() => import("@/pages/SignIn"));
const SignUp = lazyWithReload(() => import("@/pages/SignUp"));
const Trash = lazyWithReload(() => import("@/pages/Trash"));

const LEGACY_CALENDAR_PATH = "calendar/:year?/:month?/:day?";

const HomeRoute = () => {
  const { pathname, search, hash } = useLocation();
  const normalizedSearch = personalFeedSearch(search);
  if (pathname !== ROUTES.HOME || normalizedSearch !== search) {
    return <Navigate to={{ pathname: ROUTES.HOME, search: normalizedSearch, hash }} replace />;
  }
  return <Home />;
};

const LegacySettingsRedirect = () => {
  const { hash, search } = useLocation();
  const params = new URLSearchParams(personalFeedSearch(search));
  params.set(SETTINGS_SECTION_PARAM, hash.slice(1));
  return <Navigate to={{ pathname: ROUTES.HOME, search: params.toString() }} replace />;
};

// Backward compatibility alias.
export const Routes = ROUTES;
export { ROUTES };

/**
 * Static route configuration. Exported so tests can assert on the tree shape
 * (e.g. that `/auth/callback` stays outside the guest-only guard subtree) and
 * so integration tests can drive a `createMemoryRouter` over the same tree.
 */
export const routeConfig: RouteObject[] = [
  {
    path: "/",
    element: <App />,
    errorElement: <ChunkLoadErrorFallback />,
    children: [
      {
        path: Routes.AUTH,
        children: [
          {
            element: <RequireInstanceInitializationRoute />,
            children: [
              {
                element: <RequireGuestRoute />,
                children: [
                  { path: "", element: <SignIn /> },
                  { path: "admin", element: <AdminSignIn /> },
                  { path: "signup", element: <SignUp /> },
                ],
              },
            ],
          },
        ],
      },
      {
        element: <RequireAuthRoute />,
        children: [
          { path: "home", element: <HomeRoute /> },
          { path: "explore", element: <HomeRoute /> },
          { path: "inbox", element: <HomeRoute /> },
          { path: "views", element: <HomeRoute /> },
          { path: "u/:username", element: <HomeRoute /> },
          {
            path: "spaces/:spaceUid",
            children: [
              { index: true, element: <HomeRoute /> },
              { path: "explore", element: <HomeRoute /> },
              { path: LEGACY_CALENDAR_PATH, element: <LegacyCalendarRedirect /> },
            ],
          },
          { path: LEGACY_CALENDAR_PATH, element: <LegacyCalendarRedirect /> },
          {
            element: <RootLayout />,
            children: [
              {
                element: <MainLayout />,
                children: [
                  { index: true, element: <HomeRoute /> },
                  { path: Routes.ARCHIVED, element: <Archived /> },
                  { path: Routes.TRASH, element: <Trash /> },
                ],
              },
              { path: "memos/:uid", element: <MemoDetail /> },
              {
                element: <RequireFullInitializationRoute />,
                children: [{ path: Routes.SETTING, element: <LegacySettingsRedirect /> }],
              },
            ],
          },
        ],
      },
      {
        element: <RootLayout />,
        children: [
          { path: "403", element: <PermissionDenied /> },
          { path: "404", element: <NotFound /> },
          { path: "*", element: <NotFound /> },
        ],
      },
    ],
  },
];

const router = createBrowserRouter(routeConfig);

export default router;
