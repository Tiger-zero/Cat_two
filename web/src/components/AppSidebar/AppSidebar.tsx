import { Button, Drawer, Tooltip } from "@usememos/ui";
import {
  ArchiveIcon,
  ArrowRightIcon,
  BellIcon,
  CalendarDaysIcon,
  ListIcon,
  type LucideIcon,
  MenuIcon,
  SearchIcon,
  SquarePenIcon,
  UserRoundIcon,
} from "lucide-react";
import { useRef } from "react";
import { Link, useLocation } from "react-router-dom";
import { MemoDetailSidebar } from "@/components/MemoDetailSidebar";
import MemoDisplaySettingMenu from "@/components/MemoDisplaySettingMenu";
import { DEFAULT_SETTING_SECTION, SETTINGS_SECTIONS } from "@/components/Settings/settingSections";
import StatisticsView from "@/components/StatisticsView";
import { type InboxFilter, useAppSidebar } from "@/contexts/AppSidebarContext";
import { useAuth } from "@/contexts/AuthContext";
import { useGlobalMemoEditor } from "@/contexts/GlobalMemoEditorContext";
import { useInstance } from "@/contexts/InstanceContext";
import { useSpaceContext } from "@/contexts/SpaceContext";
import useCurrentUser from "@/hooks/useCurrentUser";
import { useFilteredMemoStats } from "@/hooks/useFilteredMemoStats";
import useMediaQuery from "@/hooks/useMediaQuery";
import { useNotifications } from "@/hooks/useUserQueries";
import { cn } from "@/lib/utils";
import { collectionNavigationPath, ROUTES } from "@/router/routes";
import { User_Role, UserNotification_Status } from "@/types/proto/api/user_service_pb";
import { useTranslate } from "@/utils/i18n";
import { getSidebarRouteKind } from "./routes";
import SidebarRow, { SIDEBAR_ROW_CLASSES, SidebarRowIconSlot, sidebarRowStateClasses } from "./SidebarRow";
import SidebarSection, { SIDEBAR_SECTION_STACK_CLASSES } from "./SidebarSection";
import SpaceSwitcher from "./SpaceSwitcher";
import {
  SIDEBAR_LEADING_SLOT_CLASSES,
  SIDEBAR_NAV_LEADING_SLOT_CLASSES,
  SIDEBAR_RAIL_CLASSES,
  sidebarSurfaceVariants,
} from "./sidebar-layout";
import TagsSection from "./TagsSection";
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

const CollectionSidebarContent = ({ context }: { context: "home" | "explore" | "archived" }) => {
  const t = useTranslate();
  const currentUser = useCurrentUser();
  const { memoFilter, selectedSpaceName, creatorUsername } = useSpaceContext();
  const md = useMediaQuery("md");
  const { mobileOpen, setMobileOpen } = useAppSidebar();
  const { isInitialized: authInitialized } = useAuth();
  const { isInitialized: instanceInitialized } = useInstance();
  // User-level collections stay aligned with their unscoped feeds even when a Space is remembered.
  const isUserLevelCollection = context === "archived";
  const { statistics, tags } = useFilteredMemoStats({
    context: context === "archived" ? "archived" : "collection",
    filter: isUserLevelCollection ? undefined : memoFilter,
    enabled: authInitialized && instanceInitialized && (md || mobileOpen),
  });

  const tagStateScope = isUserLevelCollection ? context : `${creatorUsername ?? "all"}${selectedSpaceName ? `:${selectedSpaceName}` : ""}`;

  return (
    <div className={SIDEBAR_SECTION_STACK_CLASSES}>
      <SidebarSection ariaLabel={t("common.statistics")}>
        <StatisticsView statisticsData={statistics} onDateSelect={() => setMobileOpen(false)} />
      </SidebarSection>
      {/* Every collection route narrows the same way: views (yours, so signed-in only), days, tags. */}
      {currentUser && <ViewsSection />}
      <TagsSection tagCount={tags} scope={tagStateScope} onSelect={() => setMobileOpen(false)} />
    </div>
  );
};

const InboxSidebarContent = () => {
  const t = useTranslate();
  const { inboxFilter, setInboxFilter, setMobileOpen } = useAppSidebar();
  const { data: notifications = [] } = useNotifications();
  const rows: Array<{ value: InboxFilter; icon: LucideIcon; label: string; count: number }> = [
    { value: "all", icon: ListIcon, label: t("common.all"), count: notifications.length },
    {
      value: "unread",
      icon: BellIcon,
      label: t("inbox.unread"),
      count: notifications.filter((item) => item.status === UserNotification_Status.UNREAD).length,
    },
    {
      value: "archived",
      icon: ArchiveIcon,
      label: t("common.archived"),
      count: notifications.filter((item) => item.status === UserNotification_Status.ARCHIVED).length,
    },
  ];
  return (
    <SidebarSection label={t("common.inbox")}>
      {rows.map((row) => (
        <SidebarRow
          key={row.value}
          state={inboxFilter === row.value ? "current" : "idle"}
          icon={row.icon}
          label={row.label}
          count={row.count}
          onClick={() => {
            setInboxFilter(row.value);
            setMobileOpen(false);
          }}
        />
      ))}
    </SidebarSection>
  );
};

const SettingsSidebarContent = () => {
  const t = useTranslate();
  const location = useLocation();
  const user = useCurrentUser();
  const { setMobileOpen } = useAppSidebar();
  const isHost = user?.role === User_Role.ADMIN;
  const currentSection = location.hash.slice(1) || DEFAULT_SETTING_SECTION;
  const basic = SETTINGS_SECTIONS.filter((section) => section.scope === "basic");
  const admin = SETTINGS_SECTIONS.filter((section) => section.scope === "admin");
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

const RouteSidebarContent = () => {
  const location = useLocation();
  const kind = getSidebarRouteKind(location.pathname);
  if (kind === "home" || kind === "archived" || kind === "explore") {
    return <CollectionSidebarContent context={kind} />;
  }
  if (kind === "views") return <ViewsSection manageActive />;
  if (kind === "inbox") return <InboxSidebarContent />;
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
  const currentUser = useCurrentUser();
  const { setQuickFindOpen } = useAppSidebar();
  const routeKind = getSidebarRouteKind(location.pathname);
  const calendarActive = routeKind === "home" || routeKind === "explore";

  return (
    <Tooltip.Provider>
      <nav
        className={cn("@container mu-s-md flex h-(--mu-row-height) items-center gap-0.5", SIDEBAR_RAIL_CLASSES)}
        aria-label={t("common.primary-navigation")}
      >
        <div
          ref={calendarRef}
          className={cn("flex min-w-0 items-center rounded-mu-md", calendarActive && sidebarRowStateClasses("current"))}
        >
          <NavPill
            to={collectionNavigationPath(location, currentUser?.username)}
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

const AppSidebar = ({ className }: { className?: string }) => {
  const t = useTranslate();
  const currentUser = useCurrentUser();
  const { setMobileOpen } = useAppSidebar();
  const { canOpen: canCompose, openEditor } = useGlobalMemoEditor();
  return (
    <aside className={cn("mu-compact flex h-full w-full select-none flex-col bg-mu-canvas text-mu-text-secondary", className)}>
      <div data-sidebar-header className={cn("flex h-13 shrink-0 items-center justify-between gap-2", SIDEBAR_RAIL_CLASSES)}>
        <SpaceSwitcher className="min-w-0" size="header" />
        {canCompose && <NewMemoAction onClick={openEditor} />}
      </div>
      <GlobalNavigation />
      <div className="mx-3 mt-2 border-t border-mu-line/70" />
      <div className={cn("min-h-0 flex-1 overflow-y-auto overflow-x-hidden pt-2 pb-3 [scrollbar-width:thin]", SIDEBAR_RAIL_CLASSES)}>
        <RouteSidebarContent />
      </div>
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
      <SpaceSwitcher className="max-w-[12rem]" size="md" />
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
        <AppSidebar />
      </Drawer.Popup>
    </Drawer.Root>
  );
};

export default AppSidebar;
