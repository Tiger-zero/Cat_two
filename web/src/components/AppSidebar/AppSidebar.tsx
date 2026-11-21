import { Button, Drawer, Tooltip } from "@usememos/ui";
import {
  ArrowRightIcon,
  CalendarDaysIcon,
  type LucideIcon,
  MenuIcon,
  SearchIcon,
  SquarePenIcon,
  Trash2Icon,
  UserRoundIcon,
} from "lucide-react";
import { useRef } from "react";
import { Link, useLocation } from "react-router-dom";
import { MemoDetailSidebar } from "@/components/MemoDetailSidebar";
import MemoDisplaySettingMenu from "@/components/MemoDisplaySettingMenu";
import { DEFAULT_SETTING_SECTION, SETTINGS_SECTIONS } from "@/components/Settings/settingSections";
import StatisticsView from "@/components/StatisticsView";
import { useAppSidebar } from "@/contexts/AppSidebarContext";
import { useAuth } from "@/contexts/AuthContext";
import { useGlobalMemoEditor } from "@/contexts/GlobalMemoEditorContext";
import { useInstance } from "@/contexts/InstanceContext";
import useCurrentUser from "@/hooks/useCurrentUser";
import { useFilteredMemoStats } from "@/hooks/useFilteredMemoStats";
import useMediaQuery from "@/hooks/useMediaQuery";
import { buildMemoCreatorFilter } from "@/lib/resource-names";
import { cn } from "@/lib/utils";
import { ROUTES } from "@/router/routes";
import { User_Role } from "@/types/proto/api/user_service_pb";
import { useTranslate } from "@/utils/i18n";
import AccountMenu from "./AccountMenu";
import CollectionStats from "./CollectionStats";
import { getSidebarRouteKind, type SidebarRouteKind } from "./routes";
import { SIDEBAR_ROW_CLASSES, SidebarRowIconSlot, sidebarRowStateClasses } from "./SidebarRow";
import SidebarSection, { SIDEBAR_SECTION_STACK_CLASSES } from "./SidebarSection";
import {
  SIDEBAR_LEADING_SLOT_CLASSES,
  SIDEBAR_NAV_LEADING_SLOT_CLASSES,
  SIDEBAR_RAIL_CLASSES,
  sidebarSurfaceVariants,
} from "./sidebar-layout";
import TagsSection from "./TagsSection";
import ThemeToggle from "./ThemeToggle";
import ViewsSection from "./ViewsSection";

const NewMemoAction = ({ onClick }: { onClick: () => void }) => {
  const t = useTranslate();
  const label = t("editor.new-memo");

  return (
    <Tooltip.Root>
      <Tooltip.Trigger
        render={<Button variant="neutral" onClick={onClick} aria-label={label} data-new-memo-trigger icon={<SquarePenIcon />} />}
      />
      <Tooltip.Popup side="bottom">{label}</Tooltip.Popup>
    </Tooltip.Root>
  );
};

const CollectionSidebarContent = ({ context, desktop }: { context: "home" | "archived"; desktop: boolean }) => {
  const t = useTranslate();
  const currentUser = useCurrentUser();
  const md = useMediaQuery("md");
  const { mobileOpen, setMobileOpen } = useAppSidebar();
  const { isInitialized: authInitialized } = useAuth();
  const { isInitialized: instanceInitialized } = useInstance();
  const { statistics, tags, summary, loading } = useFilteredMemoStats({
    context: context === "archived" ? "archived" : "collection",
    filter: context === "home" && currentUser ? buildMemoCreatorFilter(currentUser.name) : undefined,
    enabled: authInitialized && instanceInitialized && (md || mobileOpen),
  });

  return (
    <div className={cn(SIDEBAR_SECTION_STACK_CLASSES, desktop && "gap-6")}>
      {context === "home" && currentUser && <CollectionStats summary={summary} loading={loading} desktop={desktop} />}
      <SidebarSection ariaLabel={t("common.calendar")}>
        <StatisticsView statisticsData={statistics} onDateSelect={() => setMobileOpen(false)} />
      </SidebarSection>
      <div className="contents" data-sidebar-collection-list={desktop || undefined}>
        {currentUser && <ViewsSection />}
        <TagsSection tagCount={tags} scope={`${currentUser?.name ?? "anonymous"}:${context}`} onSelect={() => setMobileOpen(false)} />
      </div>
    </div>
  );
};

const SettingsSidebarContent = () => {
  const t = useTranslate();
  const location = useLocation();
  const user = useCurrentUser();
  const { setMobileOpen } = useAppSidebar();
  const isHost = user?.role === User_Role.ADMIN;
  const basic = SETTINGS_SECTIONS.filter((section) => section.scope === "basic");
  const admin = SETTINGS_SECTIONS.filter((section) => section.scope === "admin");
  const requestedSection = location.hash.slice(1);
  const currentSection = [...basic, ...(isHost ? admin : [])].some((section) => section.key === requestedSection)
    ? requestedSection
    : DEFAULT_SETTING_SECTION;
  const renderSections = (sections: typeof SETTINGS_SECTIONS) =>
    sections.map((section) => (
      <Link
        key={section.key}
        to={`${ROUTES.SETTING}#${section.key}`}
        onClick={() => setMobileOpen(false)}
        className={cn(SIDEBAR_ROW_CLASSES, sidebarRowStateClasses(currentSection === section.key ? "current" : "idle"))}
      >
        <SidebarRowIconSlot icon={section.icon} />
        <span className="truncate">{t(section.labelKey)}</span>
      </Link>
    ));
  return (
    <div className={SIDEBAR_SECTION_STACK_CLASSES}>
      <SidebarSection label={t("common.basic")}>{renderSections(basic)}</SidebarSection>
      {isHost && <SidebarSection label={t("common.admin")}>{renderSections(admin)}</SidebarSection>}
    </div>
  );
};

const MemoDetailSidebarContent = () => {
  const { memoDetail, closeMobileThen } = useAppSidebar();
  if (!memoDetail) return null;
  const runAndClose = (action: (() => void) | undefined) => (action ? () => closeMobileThen(action) : undefined);
  return (
    <MemoDetailSidebar
      memo={memoDetail.memo}
      parentMemo={memoDetail.parentMemo}
      parentStatus={memoDetail.parentStatus}
      onParentRetry={memoDetail.onParentRetry}
      parentPage={memoDetail.from}
      commentCount={memoDetail.commentCount}
      contentHidden={memoDetail.contentHidden}
      onEdit={runAndClose(memoDetail.onEdit)}
      onCommentsOpen={runAndClose(memoDetail.onCommentsOpen)}
      onCommentCreate={runAndClose(memoDetail.onCommentCreate)}
      onShareImageOpen={runAndClose(memoDetail.onShareImageOpen)}
      className="pb-2"
    />
  );
};

const RouteSidebarContent = ({ kind, desktopCollection }: { kind: SidebarRouteKind; desktopCollection: boolean }) => {
  if (kind === "home" || kind === "archived") {
    return <CollectionSidebarContent context={kind} desktop={desktopCollection} />;
  }
  if (kind === "views") return <ViewsSection manageActive />;
  if (kind === "settings") return <SettingsSidebarContent />;
  if (kind === "memo") return <MemoDetailSidebarContent />;
  return null;
};

interface NavPillProps {
  label: string;
  icon: LucideIcon;
  /** A link when set; otherwise a button that runs `onClick`. */
  to?: string;
  onClick?: () => void;
  active?: boolean;
  expanded?: boolean;
  className?: string;
}

/**
 * The compact navigator is intentionally horizontal. Its 16px glyph plus 6px padding
 * on each side makes the collapsed control an exact 28px square, the same box as the
 * header's compose control. Expanding the label only opens the text track, so the
 * artwork and surface never jump.
 *
 * Actions are separated by 2px; icons and labels by 6px. Below a 230px row the current
 * page stays a filled square with its tooltip, leaving room for the other actions.
 * The calendar link's trailing arrow shares its surface, with a separate click target.
 */
const NavPill = ({ label, icon: Icon, to, onClick, active = false, expanded = false, className }: NavPillProps) => {
  const { setMobileOpen } = useAppSidebar();
  const props = {
    onClick: () => {
      setMobileOpen(false);
      onClick?.();
    },
    "aria-label": label,
    "aria-current": active ? ("page" as const) : undefined,
    className: cn(
      sidebarSurfaceVariants({ role: "navPill" }),
      "border border-transparent mu-focus focus-visible:border-mu-focus",
      sidebarRowStateClasses(active ? "current" : "idle"),
      // The one pill with an open label gives way first, so the row always ends on the rail.
      expanded && "min-w-0 shrink",
      className,
    ),
  };
  return (
    <Tooltip.Root disabled={expanded}>
      <Tooltip.Trigger render={to ? <Link to={to} {...props} /> : <button type="button" {...props} />}>
        <span className={SIDEBAR_NAV_LEADING_SLOT_CLASSES} aria-hidden="true">
          <Icon className="mu-icon" />
        </span>
        {/* The control carries its own aria-label; this text is decoration whether or not it is open. */}
        <span
          aria-hidden="true"
          className={cn(
            "grid min-w-0 grid-cols-[0fr] ps-0 transition-[grid-template-columns,padding] duration-200 ease-out motion-reduce:transition-none",
            expanded && "@min-[230px]:grid-cols-[1fr] @min-[230px]:ps-1.5",
          )}
        >
          <span className="flex min-w-0 items-center overflow-hidden">
            <span data-sidebar-label className="min-w-0 truncate mu-body">
              {label}
            </span>
          </span>
        </span>
      </Tooltip.Trigger>
      <Tooltip.Popup side="bottom">{label}</Tooltip.Popup>
    </Tooltip.Root>
  );
};

const GlobalNavigation = () => {
  const t = useTranslate();
  const calendarRef = useRef<HTMLDivElement>(null);
  const location = useLocation();
  const { setQuickFindOpen } = useAppSidebar();
  const routeKind = getSidebarRouteKind(location.pathname);
  const calendarActive = routeKind === "home";

  return (
    <Tooltip.Provider>
      <nav
        className={cn("@container mu-s-md flex h-(--mu-row-height) shrink-0 items-center gap-0.5", SIDEBAR_RAIL_CLASSES)}
        aria-label={t("common.primary-navigation")}
      >
        <div
          ref={calendarRef}
          className={cn("flex min-w-0 items-center rounded-mu-md", calendarActive && sidebarRowStateClasses("current"))}
        >
          <NavPill
            to={calendarActive ? `${ROUTES.HOME}${location.search}` : ROUTES.HOME}
            label={t("common.calendar")}
            icon={CalendarDaysIcon}
            active={calendarActive}
            expanded
            className={cn(calendarActive && "rounded-e-none pe-0")}
          />
          {calendarActive && <MemoDisplaySettingMenu anchor={calendarRef} />}
        </div>
        <NavPill label={t("common.search")} icon={SearchIcon} onClick={() => setQuickFindOpen(true)} className="ms-auto bg-mu-well" />
      </nav>
    </Tooltip.Provider>
  );
};

const AppSidebar = ({ className, mobile = false }: { className?: string; mobile?: boolean }) => {
  const t = useTranslate();
  const currentUser = useCurrentUser();
  const { pathname } = useLocation();
  const routeKind = getSidebarRouteKind(pathname);
  const desktopCollection = !mobile && (routeKind === "home" || routeKind === "archived");
  const { setMobileOpen } = useAppSidebar();
  const { canOpen: canCompose, openEditor } = useGlobalMemoEditor();
  return (
    <aside
      className={cn(
        "memos-sidebar mu-compact flex h-full w-full select-none flex-col bg-mu-canvas text-mu-text-secondary",
        !mobile && "overflow-x-hidden overflow-y-auto overscroll-y-contain border-e-4 border-transparent [scrollbar-gutter:stable]",
        className,
      )}
    >
      <div data-sidebar-header className={cn("flex h-13 shrink-0 items-center justify-between gap-2", SIDEBAR_RAIL_CLASSES)}>
        <AccountMenu className="min-w-0" size="header" />
        <div className="flex shrink-0 items-center gap-1">
          {!mobile && <ThemeToggle />}
          {canCompose && <NewMemoAction onClick={openEditor} />}
        </div>
      </div>
      <GlobalNavigation />
      <div className={cn("mx-3 border-t border-mu-line/70", desktopCollection ? "mt-3" : "mt-2")} />
      <div
        className={cn(
          mobile ? "min-h-0 flex-1 overflow-y-auto overflow-x-hidden pb-3" : "shrink-0",
          desktopCollection ? "pt-3" : "pt-2",
          SIDEBAR_RAIL_CLASSES,
        )}
      >
        <RouteSidebarContent kind={routeKind} desktopCollection={desktopCollection} />
      </div>
      {currentUser && (
        <div className={cn("shrink-0", mobile ? "border-t border-mu-line/70 py-2" : "pt-1 pb-4", SIDEBAR_RAIL_CLASSES)}>
          <Link
            to={ROUTES.TRASH}
            onClick={() => setMobileOpen(false)}
            aria-current={pathname === ROUTES.TRASH ? "page" : undefined}
            className={cn(SIDEBAR_ROW_CLASSES, !mobile && "h-8", sidebarRowStateClasses(pathname === ROUTES.TRASH ? "current" : "idle"))}
          >
            <SidebarRowIconSlot icon={Trash2Icon} />
            <span className="truncate">{t("trash.title")}</span>
          </Link>
        </div>
      )}
      {!currentUser && (
        <footer className="shrink-0 border-t border-mu-line/70">
          <Link
            to={ROUTES.AUTH}
            onClick={() => setMobileOpen(false)}
            className={cn(
              sidebarSurfaceVariants({ role: "account" }),
              "group mu-label text-mu-text transition-colors hover:bg-mu-hover border border-transparent mu-focus focus-visible:border-mu-focus",
            )}
          >
            <span className={SIDEBAR_LEADING_SLOT_CLASSES}>
              <UserRoundIcon className="mu-icon me-auto text-mu-text-secondary" />
            </span>
            <span data-sidebar-label className="min-w-0 flex-1 truncate">
              {t("common.sign-in-to-memos")}
            </span>
            <ArrowRightIcon
              data-sidebar-trailing
              className="mu-icon-sm shrink-0 text-mu-text-secondary/60 transition-transform group-hover:translate-x-0.5 rtl:rotate-180 rtl:group-hover:-translate-x-0.5"
            />
          </Link>
        </footer>
      )}
    </aside>
  );
};

export const MobileAppHeader = () => {
  const t = useTranslate();
  const { setMobileOpen } = useAppSidebar();
  return (
    <header className="sticky top-0 z-20 flex h-12 w-full shrink-0 items-center justify-start gap-1 border-b border-mu-line/70 bg-mu-canvas/90 px-2 backdrop-blur-md md:hidden">
      <Tooltip.Root>
        <Tooltip.Trigger
          render={
            <Button
              variant="ghost"
              onClick={() => setMobileOpen(true)}
              aria-label={t("common.open-navigation")}
              data-mobile-navigation-trigger
              icon={<MenuIcon />}
            />
          }
        />
        <Tooltip.Popup side="bottom">{t("common.open-navigation")}</Tooltip.Popup>
      </Tooltip.Root>
      <AccountMenu className="max-w-[12rem]" />
      <div className="ms-auto">
        <ThemeToggle />
      </div>
    </header>
  );
};

export const MobileAppSidebar = () => {
  const t = useTranslate();
  const { mobileOpen, setMobileOpen, completeMobileClose } = useAppSidebar();
  return (
    <Drawer.Root side="start" open={mobileOpen} onOpenChange={setMobileOpen} onOpenChangeComplete={completeMobileClose}>
      <Drawer.Popup variant="bare" closeLabel={t("common.close")} className="w-72 max-w-[calc(100%-2rem)]">
        <Drawer.Title className="mu-visually-hidden">{t("common.navigation")}</Drawer.Title>
        <AppSidebar mobile />
      </Drawer.Popup>
    </Drawer.Root>
  );
};

export default AppSidebar;
