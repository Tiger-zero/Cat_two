import { Button, Tooltip } from "@usememos/ui";
import { Minimize2Icon } from "lucide-react";
import { useTranslate } from "@/utils/i18n";
import { FOCUS_MODE_STYLES } from "../constants";
import type { FocusModeExitButtonProps, FocusModeOverlayProps } from "../types";

export function FocusModeOverlay({ isActive, onToggle }: FocusModeOverlayProps) {
  const t = useTranslate();
  if (!isActive) return null;

  return <div className={FOCUS_MODE_STYLES.backdrop} onClick={onToggle} aria-label={t("editor.exit-focus-mode")} />;
}

/** Collapses focus mode back into the page. */
export function FocusModeExitButton({ onExit }: FocusModeExitButtonProps) {
  const t = useTranslate();
  const label = t("editor.exit-focus-mode");

  return (
    <Tooltip.Root>
      <Tooltip.Trigger render={<Button variant="ghost" size="sm" aria-label={label} onClick={onExit} icon={<Minimize2Icon />} />} />
      <Tooltip.Popup>{label}</Tooltip.Popup>
    </Tooltip.Root>
  );
}
