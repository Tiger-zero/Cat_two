import { Button } from "@usememos/ui";
import type { CSSProperties } from "react";
import { useEffect, useRef } from "react";
import { matchPath, Navigate, Outlet, useLocation, useSearchParams } from "react-router-dom";
import AppSidebar, {
  MobileAppHeader,
  MobileAppSidebar,
  QuickFindDialog,
  SIDEBAR_WIDTH_VAR,
  SidebarResizeHandle,
  useSidebarWidth,
} from "@/components/AppSidebar";
import SettingsDialog from "@/components/Settings/SettingsDialog";
import { AppSidebarProvider } from "@/contexts/AppSidebarContext";
import { useAuth } from "@/contexts/AuthContext";
import { useInstance } from "@/contexts/InstanceContext";
import { MemoFilterProvider, useMemoFilterContext } from "@/contexts/MemoFilterContext";
import { useView } from "@/contexts/ViewContext";
import useCurrentUser from "@/hooks/useCurrentUser";
import useMediaQuery from "@/hooks/useMediaQuery";
import { useResolvedTheme } from "@/hooks/useResolvedTheme";
import { cn } from "@/lib/utils";
import { ROUTES } from "@/router/routes";
import { InstanceAccessMode } from "@/types/proto/api/instance_service_pb";
import { buildAuthRoute, shouldGatePrivateInstance } from "@/utils/auth-redirect";
import { useTranslate } from "@/utils/i18n";

const CAT_TWO_DEPLOY_URL = "https://github.com/Tiger-zero/Cat_two#部署服务端";

const DemoBanner = () => {
  const t = useTranslate();

  return (
    <div className="static w-full shrink-0 border-b border-mu-line bg-mu-well/70 px-4 py-2 mu-body text-mu-text-secondary sm:px-6">
      <div className="mx-auto flex max-w-5xl flex-col items-start gap-1 sm:flex-row sm:items-center sm:justify-center sm:gap-2">
        <span className="font-medium text-mu-text">{t("demo.banner-title")}</span>
        <span>{t("demo.banner-description")}</span>
        <a
          className="font-medium text-mu-accent underline-offset-4 hover:underline"
          href={CAT_TWO_DEPLOY_URL}
          target="_blank"
          rel="noreferrer"
        >
          {t("demo.deploy-link")}
        </a>
      </div>
    </div>
  );
};

const UserSettingsRecovery = () => {
  const t = useTranslate();
  const { userSettingsLoadError, isLoading, refetchSettings } = useAuth();
  if (!userSettingsLoadError) return null;

  return (
    <div className="w-full px-4 pt-3 sm:px-6">
      <div className="memos-panel flex flex-wrap items-center justify-between gap-3 p-4">
        <p role="alert" className="mu-body text-mu-text-secondary">
          {t("auth.settings-load-error")}
        </p>
        <Button variant="neutral" loading={isLoading} onClick={() => void refetchSettings().catch(() => undefined)}>
          {t("common.retry")}
        </Button>
      </div>
    </div>
  );
};

const RootLayoutContent = () => {
  const location = useLocation();
  const [searchParams] = useSearchParams();
  const currentUser = useCurrentUser();
  const md = useMediaQuery("md");
  const { profile } = useInstance();
  const { removeFilter } = useMemoFilterContext();
  const { fullWidth, maxColumns } = useView();
  const { pathname } = location;
  const home = pathname === ROUTES.HOME;
  const homeCollection = home || (matchPath(ROUTES.TRASH, pathname) !== null && location.state?.trashFrom !== ROUTES.ARCHIVED);
  const lightHome = useResolvedTheme() === "default" && homeCollection;
  const limitShell = !fullWidth && maxColumns !== 0;
  const prevPathnameRef = useRef<string | undefined>(undefined);
  const shellRef = useRef<HTMLDivElement>(null);
  const { width: sidebarWidth, minWidth, maxWidth, defaultWidth, setWidth: setSidebarWidth } = useSidebarWidth(lightHome);

  useEffect(() => {
    const prevPathname = prevPathnameRef.current;

    // When the route changes and there is no filter in the search params, remove all filters.
    if (prevPathname !== undefined && prevPathname !== pathname && !searchParams.has("filter")) {
      removeFilter(() => true);
    }

    prevPathnameRef.current = pathname;
  }, [pathname, searchParams, removeFilter]);

  // Treat an unspecified mode as private so a partial or older response cannot expose content.
  if (
    shouldGatePrivateInstance({
      isPrivateInstance: profile.accessMode !== InstanceAccessMode.PUBLIC,
      isAuthenticated: !!currentUser,
    })
  ) {
    const redirect = `${pathname}${location.search}${location.hash}`;
    return <Navigate to={buildAuthRoute({ redirect })} replace />;
  }

  return (
    <div
      ref={shellRef}
      className={cn(
        "mx-auto flex min-h-full w-full bg-mu-canvas md:gap-6 md:px-4 lg:gap-8 lg:px-6",
        limitShell && (maxColumns === 1 ? "max-w-[66.5rem]" : "max-w-7xl"),
        limitShell && maxColumns === 1 && homeCollection && "light:lg:max-w-[69rem]",
      )}
      style={{ [SIDEBAR_WIDTH_VAR]: `${sidebarWidth}px` } as CSSProperties}
    >
      {md && (
        <div className="sticky top-4 z-30 mt-4 h-[calc(100dvh-2rem)] w-(--app-sidebar-width) shrink-0 self-start">
          <AppSidebar className="rounded-mu-xl border border-mu-line/75" />
          <SidebarResizeHandle
            appearance="grip"
            width={sidebarWidth}
            minWidth={minWidth}
            maxWidth={maxWidth}
            defaultWidth={defaultWidth}
            onWidthChange={setSidebarWidth}
            targetRef={shellRef}
          />
        </div>
      )}
      <MobileAppSidebar />
      <main className="flex min-h-full min-w-0 flex-1 flex-col items-center">
        <MobileAppHeader />
        {profile.demo && <DemoBanner />}
        <UserSettingsRecovery />
        <Outlet />
      </main>
      <QuickFindDialog />
      <SettingsDialog />
    </div>
  );
};

const RootLayout = () => (
  <MemoFilterProvider>
    <AppSidebarProvider>
      <RootLayoutContent />
    </AppSidebarProvider>
  </MemoFilterProvider>
);

export default RootLayout;
