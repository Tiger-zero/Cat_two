import { useSyncExternalStore } from "react";
import { getAppliedTheme, getInitialTheme, type ResolvedTheme, subscribeToTheme, type Theme } from "@/utils/theme";

/** The theme actually in effect: the user's setting, or the OS scheme while that setting is "system". */
export const useResolvedTheme = (): ResolvedTheme => useSyncExternalStore(subscribeToTheme, getAppliedTheme, () => "default");

/** The original preference, including System when the OS supplies the scheme. */
export const useThemePreference = (): Theme => useSyncExternalStore(subscribeToTheme, getInitialTheme, () => "system");
