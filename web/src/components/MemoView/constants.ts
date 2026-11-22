import { cn } from "@/lib/utils";

export const MEMO_CARD_GUTTER_CLASSES = "px-4 md:px-5";
export const MEMO_CARD_FRAME_CLASSES = "memos-panel memo-list-gap w-full";

export const MEMO_CARD_CONTENT_CLASSES = `relative group flex w-full flex-col items-start justify-start gap-2.5 ${MEMO_CARD_GUTTER_CLASSES} py-3 text-mu-text md:py-4`;

export const MEMO_CARD_BASE_CLASSES = `${MEMO_CARD_CONTENT_CLASSES} ${MEMO_CARD_FRAME_CLASSES}`;

/**
 * A memo's timestamp as a control, on the card header and in the editor: one line of muted
 * 13px text with no box, darkening on hover like every quiet label.
 */
export const MEMO_TIME_CONTROL_CLASSES = cn(
  "shrink-0 whitespace-nowrap rounded-mu-sm text-start mu-caption tabular-nums text-mu-text-secondary transition-colors select-none hover:text-mu-text",
  "border border-transparent mu-focus focus-visible:border-mu-focus",
);

export const RELATIVE_TIME_THRESHOLD_MS = 1000 * 60 * 60 * 24;
