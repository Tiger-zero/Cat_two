import type { MemoTimeBasis } from "@/contexts/ViewContext";
import type { useTranslate } from "@/utils/i18n";

export type TranslateFunction = ReturnType<typeof useTranslate>;

export const getTooltipText = (count: number, date: string, t: TranslateFunction, timeBasis: MemoTimeBasis = "create_time"): string => {
  if (count === 0) {
    return date;
  }

  const key = timeBasis === "update_time" ? "memo.count-memos-updated-in-date" : "memo.count-memos-in-date";
  return t(key, {
    count,
    memos: count === 1 ? t("common.memo") : t("common.memos"),
    date,
  }).toLowerCase();
};
