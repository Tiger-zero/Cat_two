import { cn } from "@/lib/utils";
import type { CalendarDayCell } from "./types";

/**
 * The square chip inside a cell carries every visual. The grid's columns are 28px (see
 * CALENDAR_GRID_CLASSES), so the chip is a 28px square; `max-w-7` keeps it there should a
 * wider container ever hand the cell more.
 */
const CHIP_BASE =
  "relative flex aspect-square w-full max-w-7 items-center justify-center rounded-mu-md border border-transparent text-center mu-caption tabular-nums transition-[background-color,color,filter,box-shadow] duration-150 ease-out";

/** Hover reads off the cell (`group/day`), not the chip, so the whole column responds to the pointer. */
const INTERACTIVE_CHIP =
  "group-hover/day:brightness-[0.97] group-focus-visible/day:outline-2 group-focus-visible/day:outline-solid group-focus-visible/day:-outline-offset-2 group-focus-visible/day:outline-mu-focus";

const OUTSIDE_MONTH_CHIP = "bg-transparent text-mu-text-secondary/25";
const EMPTY_CHIP = "bg-[var(--memos-activity-empty,var(--mu-well))] text-mu-text-secondary";
const TODAY_CHIP = "border-[color:var(--memos-activity-today,var(--mu-primary))]";
const SELECTED_CHIP = "z-10 border-mu-accent bg-mu-selected text-mu-text";

const getFillClass = (day: CalendarDayCell): string => {
  if (!day.isCurrentMonth) return OUTSIDE_MONTH_CHIP;
  if (day.isSelected) return SELECTED_CHIP;
  // Fixed bands keep the same color for the same activity across months.
  if (day.count === 0) return EMPTY_CHIP;
  if (day.count <= 2) return "bg-[var(--memos-activity-low,var(--mu-selected))] text-mu-text";
  if (day.count <= 5) return "bg-[var(--memos-activity-medium,var(--mu-selected))] text-mu-text";
  return "bg-[var(--memos-activity-high,var(--mu-selected))] text-mu-text dark:text-mu-on-primary";
};

export const getChipClassName = (day: CalendarDayCell, isInteractive: boolean): string =>
  cn(
    CHIP_BASE,
    getFillClass(day),
    day.isCurrentMonth && day.isToday && !day.isSelected && TODAY_CHIP,
    isInteractive && day.isCurrentMonth && INTERACTIVE_CHIP,
  );
