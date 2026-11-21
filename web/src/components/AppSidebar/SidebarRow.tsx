import type { LucideIcon } from "lucide-react";
import type { ReactNode } from "react";
import { cn } from "@/lib/utils";
import { SCOPE_PANEL_ICON_CLASSES, SIDEBAR_LEADING_SLOT_CLASSES, sidebarSurfaceVariants } from "./sidebar-layout";

/**
 * The rail's row rhythm — height, type scale, radius, padding, gap. Every list in the
 * sidebar is built from this, so a row reads the same whether it is a setting or
 * a tag. Split from the focus treatment because a row is not always its own focus target:
 * rows that carry a trailing control put the box on a wrapper and focus on the button
 * inside it.
 */
export const SIDEBAR_ROW_BOX_CLASSES = `${sidebarSurfaceVariants({ role: "row" })} group transition-colors`;

export const SIDEBAR_ROW_CLASSES = `${SIDEBAR_ROW_BOX_CLASSES} border border-transparent mu-focus focus-visible:border-mu-focus`;

export const SIDEBAR_ROW_ICON_CLASSES =
  // The md row's 16px icon at the md stroke, beside its 14px label, in the row's own muted ink.
  "mu-icon me-auto shrink-0 group-data-checked:text-white";
const SIDEBAR_ROW_COUNT_CLASSES = "mu-caption tabular-nums text-mu-text-secondary group-data-checked:text-white";

/**
 * The focusable body of a split row — rows whose box is a wrapper carrying other controls
 * put their label layout and focus ring here. The gap must stay equal to the box's own
 * `gap-1` or slot alignment breaks between one-control and split rows.
 */
export const SIDEBAR_ROW_LABEL_CLASSES = `flex h-full min-w-0 flex-1 items-center gap-1 text-start border border-transparent mu-focus focus-visible:border-mu-focus`;

/**
 * Fixed leading slot: a 20px column begins on the artwork rail. Small glyphs pin to its
 * start while 20px marks fill it; the following 4px gap puts every first-level label on
 * the same rail without compensating margins.
 */
export const SIDEBAR_ROW_SLOT_CLASSES = SIDEBAR_LEADING_SLOT_CLASSES;

/** A slot that is itself a control (disclosure, row menu): same box plus the hover chip. */
export const SIDEBAR_ROW_SLOT_BUTTON_CLASSES = `${SIDEBAR_ROW_SLOT_CLASSES} relative rounded hover:bg-mu-hover group-data-checked:hover:bg-white/15 after:absolute after:-inset-0.5 after:content-[''] border border-transparent mu-focus focus-visible:border-mu-focus`;

/** Trailing rail for counts, wide enough that digits align down the list. */
export const SIDEBAR_ROW_COUNT_RAIL_CLASSES = `${SIDEBAR_ROW_COUNT_CLASSES} min-w-[3ch] shrink-0 text-end`;

/** The standard icon-in-slot pairing, so every list renders its glyphs identically. */
export const SidebarRowIconSlot = ({ icon: Icon }: { icon: LucideIcon }) => (
  <span className={SIDEBAR_ROW_SLOT_CLASSES} aria-hidden="true">
    <Icon className={SIDEBAR_ROW_ICON_CLASSES} />
  </span>
);

/**
 * The scope panel's leading icon (see sidebar-layout): links, actions and submenu triggers in
 * the panel share the 20px leading slot with the sidebar rows above them.
 */
export const ScopePanelRowIcon = ({ icon: Icon, className }: { icon: LucideIcon; className?: string }) => (
  <span className={SIDEBAR_LEADING_SLOT_CLASSES} aria-hidden="true">
    <Icon className={cn(SCOPE_PANEL_ICON_CLASSES, className)} />
  </span>
);

/**
 * Navigation and settings use the theme's current surface. Selected tags and the trash row
 * use the same green fill across themes.
 */
export type SidebarRowState = "idle" | "current" | "checked";

const SIDEBAR_ROW_HOVER_CLASSES = "hover:bg-mu-hover/65 hover:text-mu-text";
export const SIDEBAR_ROW_SELECTION_CLASSES = "bg-[#397354] text-white";

export const sidebarRowStateClasses = (state: SidebarRowState = "idle") => {
  // Selection never changes weight (the type rules), so labels do not shift: fill and color carry it.
  if (state === "current")
    return "bg-mu-hover text-mu-text light:bg-mu-selected light:text-mu-accent dark:bg-mu-selected dark:text-mu-accent";
  if (state === "checked") return SIDEBAR_ROW_SELECTION_CLASSES;
  return `text-mu-text-secondary ${SIDEBAR_ROW_HOVER_CLASSES}`;
};

/** Lets controls and counts keep a readable foreground on the selected fill. */
export const sidebarRowStateAttributes = (state: SidebarRowState) => ({
  "data-checked": state === "checked" ? "" : undefined,
});

interface Props {
  state?: SidebarRowState;
  icon?: LucideIcon;
  label: ReactNode;
  count?: number;
  onClick?: () => void;
  trailing?: ReactNode;
}

const SidebarRow = ({ state = "idle", icon: Icon, label, count, onClick, trailing }: Props) => (
  <button
    type="button"
    onClick={onClick}
    aria-pressed={state !== "idle" || undefined}
    {...sidebarRowStateAttributes(state)}
    className={cn(SIDEBAR_ROW_CLASSES, sidebarRowStateClasses(state))}
  >
    {Icon && <SidebarRowIconSlot icon={Icon} />}
    <span data-sidebar-label className="min-w-0 flex-1 truncate text-start">
      {label}
    </span>
    {count != null && count > 0 && (
      <span className={cn(SIDEBAR_ROW_COUNT_RAIL_CLASSES, state === "current" && "light:text-mu-accent dark:text-mu-accent")}>{count}</span>
    )}
    {trailing}
  </button>
);

export default SidebarRow;
