import { cn } from "@/lib/utils";

export const MEMO_CARD_CONTENT_CLASSES =
  "relative group flex w-full flex-col items-start justify-start gap-2 px-4 py-3 text-mu-text transition-colors";

export const MEMO_CARD_BASE_CLASSES = `${MEMO_CARD_CONTENT_CLASSES} memos-panel mb-2`;

/**
 * A memo's timestamp as a control, on the card header and in the editor: one line of muted
 * 13px text with no box, darkening on hover like every quiet label.
 */
export const MEMO_TIME_CONTROL_CLASSES = cn(
  "shrink-0 whitespace-nowrap rounded-mu-sm text-start mu-caption text-mu-text-secondary transition-colors select-none hover:text-mu-text",
  "border border-transparent mu-focus focus-visible:border-mu-focus",
);

export const RELATIVE_TIME_THRESHOLD_MS = 1000 * 60 * 60 * 24;
