import { Button, Tooltip } from "@usememos/ui";
import { MoonIcon, SunIcon } from "lucide-react";
import { useAuth } from "@/contexts/AuthContext";
import { useResolvedTheme, useThemePreference } from "@/hooks/useResolvedTheme";
import { useUpdateUserGeneralSetting } from "@/hooks/useUserQueries";
import { handleError } from "@/lib/error";
import { showErrorToast } from "@/lib/toast";
import { useTranslate } from "@/utils/i18n";
import { getInitialTheme, isDarkTheme, loadTheme } from "@/utils/theme";

const ThemeToggle = () => {
  const t = useTranslate();
  const { currentUser, isUserSettingsInitialized, refetchSettings } = useAuth();
  const { mutate: updateTheme, isPending } = useUpdateUserGeneralSetting(currentUser?.name);
  const previousTheme = useThemePreference();
  const dark = isDarkTheme(useResolvedTheme());
  const nextTheme = dark ? "default" : "default-dark";
  const label = t(dark ? "setting.preference.theme-light" : "setting.preference.theme-dark");

  const toggle = () => {
    loadTheme(nextTheme);
    if (!currentUser) return;
    updateTheme(
      { generalSetting: { theme: nextTheme }, updateMask: ["theme"] },
      {
        onSuccess: () => refetchSettings(),
        onError: (error) => {
          if (getInitialTheme() === nextTheme) loadTheme(previousTheme);
          handleError(error, showErrorToast);
        },
      },
    );
  };

  return (
    <Tooltip.Root>
      <Tooltip.Trigger
        render={
          <Button
            variant="ghost"
            icon={dark ? <SunIcon /> : <MoonIcon />}
            aria-label={label}
            disabled={isPending || (Boolean(currentUser) && !isUserSettingsInitialized)}
            onClick={toggle}
          />
        }
      />
      <Tooltip.Popup side="bottom">{label}</Tooltip.Popup>
    </Tooltip.Root>
  );
};

export default ThemeToggle;
