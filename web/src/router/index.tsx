import { createBrowserRouter, Navigate, type RouteObject, useLocation } from "react-router-dom";
import App from "@/App";
import { ChunkLoadErrorFallback } from "@/components/ErrorBoundary";
import { preloadMemoEditing } from "@/components/MemoEditor";
import MainLayout from "@/layouts/MainLayout";
import RootLayout from "@/layouts/RootLayout";
import { lazyWithReload } from "@/utils/lazy";
import { RequireAuthRoute, RequireFullInitializationRoute, RequireGuestRoute, RequireInstanceInitializationRoute } from "./guards";
import { LegacyProfileRedirect } from "./LegacyProfileRedirect";
import { CALENDAR_ROUTE_PATTERN, ROUTES, SPACE_ROUTE_PATTERN, withCollectionCreator } from "./routes";
import { SpaceRoute } from "./SpaceRoute";

const AdminSignIn = lazyWithReload(() => import("@/pages/AdminSignIn"));
const Archived = lazyWithReload(() => import("@/pages/Archived"));
const AuthCallback = lazyWithReload(() => import("@/pages/AuthCallback"));
const Calendar = lazyWithReload(() => import("@/pages/Calendar"));
// A signed-in user's Home composes at once, so the page's code arrives together with the
// editing session's and the composer renders with the page, never after it.
const Home = lazyWithReload(() => Promise.all([import("@/pages/Home"), preloadMemoEditing()]).then(([page]) => page));
const Inboxes = lazyWithReload(() => import("@/pages/Inboxes"));
const MemoDetail = lazyWithReload(() => import("@/pages/MemoDetail"));
const NotFound = lazyWithReload(() => import("@/pages/NotFound"));
const PermissionDenied = lazyWithReload(() => import("@/pages/PermissionDenied"));
const Attachments = lazyWithReload(() => import("@/pages/Attachments"));
const Setting = lazyWithReload(() => import("@/pages/Setting"));
const MemoViews = lazyWithReload(() => import("@/pages/MemoViews"));
const SignIn = lazyWithReload(() => import("@/pages/SignIn"));
const SignUp = lazyWithReload(() => import("@/pages/SignUp"));

const ExploreRoute = () => {
  const location = useLocation();
  const search = withCollectionCreator(location.search);
  if (search !== location.search) return <Navigate to={{ pathname: location.pathname, search, hash: location.hash }} replace />;
  return <Home />;
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
          // The OAuth callback must run regardless of the current session — an
          // authenticated tab elsewhere must not block it from consuming its
          // one-time OAuth state. Keep it outside the guest-only subtree.
          { path: "callback", element: <AuthCallback /> },
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
      // Backward compatibility: the old `/home` URL now lives at `/`.
      { path: "home", element: <Navigate to={Routes.HOME} replace /> },
      {
        element: <RequireAuthRoute />,
        children: [
          {
            element: <RootLayout />,
            children: [
              {
                element: <MainLayout />,
                children: [
                  { index: true, element: <Home /> },
                  { path: Routes.EXPLORE, element: <ExploreRoute /> },
                  { path: CALENDAR_ROUTE_PATTERN, element: <Calendar /> },
                  { path: Routes.ARCHIVED, element: <Archived /> },
                  {
                    element: <RequireFullInitializationRoute />,
                    children: [{ path: Routes.VIEWS, element: <MemoViews /> }],
                  },
                ],
              },
              { path: "memos/:uid", element: <MemoDetail /> },
              {
                element: <RequireFullInitializationRoute />,
                children: [
                  {
                    path: SPACE_ROUTE_PATTERN,
                    children: [
                      {
                        element: <SpaceRoute />,
                        children: [
                          {
                            element: <MainLayout />,
                            children: [
                              { index: true, element: <Home /> },
                              { path: "explore", element: <ExploreRoute /> },
                              { path: "calendar/:year?/:month?/:day?", element: <Calendar /> },
                            ],
                          },
                          { path: "attachments", element: <Attachments /> },
                        ],
                      },
                      { path: "*", element: <NotFound /> },
                    ],
                  },
                  { path: Routes.ATTACHMENTS, element: <Attachments /> },
                  { path: Routes.INBOX, element: <Inboxes /> },
                  { path: Routes.SETTING, element: <Setting /> },
                ],
              },
            ],
          },
        ],
      },
      {
        element: <RootLayout />,
        children: [
          { path: Routes.USER_PROFILE, element: <LegacyProfileRedirect /> },
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
