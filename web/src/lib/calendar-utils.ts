import type { MemoTimeBasis } from "@/contexts/ViewContext";
import { combineCELFilters } from "@/lib/cel-filter";
import { APP_TIME_ZONE, calendarDate, inAppTimeZone, parseAppDateTime } from "@/lib/time-zone";

export const MONTH_DATE_FORMAT = "YYYY-MM" as const;
export const ISO_DATE_FORMAT = "YYYY-MM-DD" as const;

export const getToday = (): string => inAppTimeZone().format(ISO_DATE_FORMAT);
export const getCurrentMonth = (): string => inAppTimeZone().format(MONTH_DATE_FORMAT);

export const addMonths = (date: string, count: number): string => calendarDate(date).add(count, "month").format(MONTH_DATE_FORMAT);

/** "August 2026" for `YYYY-MM`, in the viewer's language. */
export const formatMonthLabel = (month: string, locale: string): string =>
  calendarDate(month).toDate().toLocaleString(locale, { timeZone: APP_TIME_ZONE, year: "numeric", month: "long" });

/**
 * The Shanghai midnight named by `YYYY-MM-DD`, or undefined for anything else — including
 * dates JS would silently roll forward (2026-02-30 → March 2).
 */
export const parseLocalDate = (value: string): Date | undefined => parseAppDateTime(`${value} 00:00:00`);

/** A half-open `[start, end)` window in epoch seconds, based on the Shanghai calendar. */
export interface LocalTimestampRange {
  startTimestamp: number;
  endTimestamp: number;
}

const toRange = (start: Date, end: Date): LocalTimestampRange => ({
  startTimestamp: Math.floor(start.getTime() / 1000),
  endTimestamp: Math.floor(end.getTime() / 1000),
});

/** The local calendar day named by `YYYY-MM-DD`, or undefined for an invalid date. */
const getTimestampRange = (value: string, unit: "day" | "month"): LocalTimestampRange | undefined => {
  const start = parseLocalDate(value);
  if (!start) return undefined;
  const end = parseLocalDate(calendarDate(value).add(1, unit).format(ISO_DATE_FORMAT));
  return end && toRange(start, end);
};

export const getLocalDayTimestampRange = (value: string): LocalTimestampRange | undefined => getTimestampRange(value, "day");

/** The local calendar month named by `YYYY-MM`, or undefined for an invalid month. */
export const getLocalMonthTimestampRange = (value: string): LocalTimestampRange | undefined => getTimestampRange(`${value}-01`, "month");

/** The CEL timestamp field a time basis is stored under. */
export const getTimeBasisField = (timeBasis: MemoTimeBasis): "created_ts" | "updated_ts" =>
  timeBasis === "update_time" ? "updated_ts" : "created_ts";

/** A CEL clause selecting memos whose `field` falls inside `range`. */
export const buildTimestampRangeFilter = (field: "created_ts" | "updated_ts", range: LocalTimestampRange): string =>
  `${field} >= timestamp(${range.startTimestamp}) && ${field} < timestamp(${range.endTimestamp})`;

/** `filter` narrowed to memos whose `timeBasis` timestamp falls inside `range`; unchanged when there is no range. */
export const withTimestampRange = (
  filter: string | undefined,
  range: LocalTimestampRange | undefined,
  timeBasis: MemoTimeBasis,
): string | undefined => (range ? combineCELFilters(filter, buildTimestampRangeFilter(getTimeBasisField(timeBasis), range)) : filter);
