import { Menu, menuClasses } from "@usememos/ui";
import { ChevronRightIcon, GlobeIcon, LogOutIcon, type LucideIcon, PaletteIcon, SettingsIcon } from "lucide-react";
import { useState } from "react";
import { hasStoredToken } from "@/auth-state";
import { ScopePanelRowIcon } from "@/components/AppSidebar/SidebarRow";
import { SCOPE_PANEL_ICON_CLASSES, SCOPE_PANEL_ROW_CLASSES, SIDEBAR_LEADING_SLOT_CLASSES } from "@/components/AppSidebar/sidebar-layout";
import UserAvatar from "@/components/UserAvatar";
import { useAuth } from "@/contexts/AuthContext";
import useCurrentUser from "@/hooks/useCurrentUser";
import { useSSEConnectionStatus } from "@/hooks/useLiveMemoRefresh";
import { useThemePreference } from "@/hooks/useResolvedTheme";
import { useUpdateUserGeneralSetting } from "@/hooks/useUserQueries";
import { locales } from "@/i18n";
import { handleError } from "@/lib/error";
import { showErrorToast } from "@/lib/toast";
import { cn } from "@/lib/utils";
import { buildAuthRoute } from "@/utils/auth-redirect";
import { getLocaleDisplayName, getLocaleWithFallback, loadLocale, useTranslate } from "@/utils/i18n";
import { loadTheme, THEME_OPTIONS } from "@/utils/theme";

interface PreferenceSubmenuProps {
  icon: LucideIcon;
  label: string;
  value: string;
  options: Array<{ value: string; label: string }>;
  onChange: (value: string) => void;
  /** Long lists scroll; typing jumps to a match. */
  className?: string;
}

/** A row that opens its choices beside the panel, like a submenu, and applies one on select. */
const PreferenceSubmenu = ({ icon: Icon, label, value, options, onChange, className }: PreferenceSubmenuProps) => (
  <Menu.Root modal={false}>
    <Menu.Trigger openOnHover className={SCOPE_PANEL_ROW_CLASSES}>
      <ScopePanelRowIcon icon={Icon} />
      <span className="shrink-0">{label}</span>
      <span className="min-w-0 flex-1 truncate text-end text-mu-text-secondary">
        {options.find((option) => option.value === value)?.label ?? value}
      </span>
      <ChevronRightIcon aria-hidden="true" className={cn(SCOPE_PANEL_ICON_CLASSES, "rtl:rotate-180")} />
    </Menu.Trigger>
    {/* Like a nested submenu: its first row lines up with this one, and its rows match the panel's fixed menu density.
        When neither side has room (narrow screens), it drops below the row instead of overflowing. */}
    <Menu.Popup
      side="inline-end"
      align="start"
      collisionAvoidance={{ fallbackAxisSide: "end" }}
      positionMethod="fixed"
      className={className}
    >
      <Menu.RadioGroup value={value} onValueChange={onChange}>
        {options.map((option) => (
          <Menu.RadioItem key={option.value} value={option.value} closeOnClick>
            {option.label}
          </Menu.RadioItem>
        ))}
      </Menu.RadioGroup>
    </Menu.Popup>
  </Menu.Root>
);

const LOCALE_OPTIONS = locales.map((locale) => ({ value: locale, label: getLocaleDisplayName(locale) }));

/** Theme and language apply at once: saved to the account when signed in, otherwise kept in this browser. */
const UserPreferenceMenu = () => {
  const t = useTranslate();
  const currentUser = useCurrentUser();
  const { userGeneralSetting, refetchSettings } = useAuth();
  const { mutate: updateUserGeneralSetting } = useUpdateUserGeneralSetting(currentUser?.name);
  // The chosen value shows immediately, before a signed-in save round-trips.
  const [locale, setLocale] = useState<string>();
  const currentTheme = useThemePreference();
  const currentLocale = locale ?? getLocaleWithFallback(userGeneralSetting?.locale);

  const handleThemeChange = (value: string) => {
    loadTheme(value);
    if (currentUser) {
      updateUserGeneralSetting({ generalSetting: { theme: value }, updateMask: ["theme"] }, { onSuccess: () => refetchSettings() });
    }
  };

  const handleLocaleChange = (value: string) => {
    setLocale(loadLocale(value));
    if (currentUser) {
      updateUserGeneralSetting({ generalSetting: { locale: value }, updateMask: ["locale"] }, { onSuccess: () => refetchSettings() });
    }
  };

  return (
    <section aria-label={t("setting.preference.label")}>
      <PreferenceSubmenu
        icon={PaletteIcon}
        label={t("setting.preference.theme")}
        value={currentTheme}
        options={THEME_OPTIONS.map((option) => ({ value: option.value, label: t(option.labelKey) }))}
        onChange={handleThemeChange}
      />
      <PreferenceSubmenu
        icon={GlobeIcon}
        label={t("common.language")}
        value={currentLocale}
        options={LOCALE_OPTIONS}
        onChange={handleLocaleChange}
        className="max-h-72"
      />
    </section>
  );
};

/** Account settings and personal destinations at the top of the username menu. */
const UserMenu = ({ onOpenSettings }: { onOpenSettings: () => void }) => {
  const t = useTranslate();
  const currentUser = useCurrentUser();
  const sseStatus = useSSEConnectionStatus();
  const { logout } = useAuth();
  const accountLabel = currentUser?.displayName || currentUser?.username || t("common.profile");

  const handleSignOut = async () => {
    try {
      await logout();
    } catch (error) {
      handleError(error, showErrorToast, { context: "Sign out" });
      return;
    }
    // A newer same-origin login might have supplied a different session.
    if (hasStoredToken()) return;
    try {
      const keysToPreserve = ["memos-theme", "memos-locale", "memos-view-setting", "tag-view-as-tree", "memos_signed_out_session"];
      const keysToRemove: string[] = [];
      for (let i = 0; i < localStorage.length; i++) {
        const key = localStorage.key(i);
        if (key && !keysToPreserve.includes(key) && !key.startsWith("memos-pinned-tags:")) keysToRemove.push(key);
      }
      keysToRemove.forEach((key) => localStorage.removeItem(key));
    } catch {
      // Ignore errors from localStorage operations.
    }
    window.location.replace(buildAuthRoute());
  };

  return (
    <>
      {currentUser && (
        <button type="button" className={SCOPE_PANEL_ROW_CLASSES} onClick={onOpenSettings} aria-haspopup="dialog">
          <ScopePanelRowIcon icon={SettingsIcon} />
          <span className="min-w-0 flex-1 truncate">{t("common.settings")}</span>
        </button>
      )}
      <UserPreferenceMenu />
      {currentUser && (
        <>
          <div role="separator" className={menuClasses().separator} />
          <section aria-label={t("setting.account.title")}>
            <div className="flex h-(--mu-row-height) items-center gap-2 px-(--mu-row-padding)">
              <span className={cn(SIDEBAR_LEADING_SLOT_CLASSES, "relative")}>
                <UserAvatar avatarUrl={currentUser.avatarUrl} name={accountLabel} className="size-5" />
                {sseStatus !== "connected" && (
                  <span
                    role="img"
                    aria-label={t(`live-update.${sseStatus}` as Parameters<typeof t>[0])}
                    className={cn(
                      "absolute -bottom-0.5 -end-0.5 size-2.5 rounded-full border-2 border-mu-surface",
                      sseStatus === "connecting" ? "animate-pulse bg-mu-text-secondary" : "bg-mu-danger-key",
                    )}
                  />
                )}
              </span>
              <span className="min-w-0 flex-1 truncate mu-label">{accountLabel}</span>
            </div>
            <button type="button" className={cn(SCOPE_PANEL_ROW_CLASSES, "text-mu-text-secondary")} onClick={() => void handleSignOut()}>
              <ScopePanelRowIcon icon={LogOutIcon} />
              <span className="min-w-0 flex-1 truncate">{t("common.sign-out")}</span>
            </button>
          </section>
        </>
      )}
    </>
  );
};

export default UserMenu;
