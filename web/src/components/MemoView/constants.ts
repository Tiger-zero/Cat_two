import { cn } from "@/lib/utils";

export const MEMO_CARD_BASE_CLASSES =
  "memos-panel memo-list-gap relative group flex w-full flex-col items-start justify-start gap-2.5 px-4 py-3 text-mu-text md:px-5 md:py-4";

/**
 * A memo's timestamp as a control, in the card footer and the editor: one line of muted
 * 13px text with no box, darkening on hover like every quiet label.
 */
export const MEMO_TIME_CONTROL_CLASSES = cn(
  "shrink-0 whitespace-nowrap rounded-mu-sm text-start mu-caption tabular-nums text-mu-text-secondary transition-colors select-none hover:text-mu-text",
  "border border-transparent mu-focus focus-visible:border-mu-focus",
);
