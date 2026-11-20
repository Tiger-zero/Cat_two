import type { MemoFilter } from "@/contexts/MemoFilterContext";
import { ISO_DATE_FORMAT } from "@/lib/calendar-utils";
import { inAppTimeZone, parseAppDateTime } from "@/lib/time-zone";

/**
 * Derive a default `createTime` for a new memo from the active memo filters.
 * If a `displayTime:YYYY-MM-DD` filter is present, combines that day
 * with `now`'s Shanghai clock time. Returns undefined otherwise or
 * when the filter value is malformed.
 */
export function deriveDefaultCreateTimeFromFilters(filters: MemoFilter[], now: Date = new Date()): Date | undefined {
  const dateFilter = filters.find((f) => f.factor === "displayTime");
  if (!dateFilter) return undefined;
  return deriveDefaultCreateTimeFromDate(dateFilter.value, now);
}

/**
 * The date `YYYY-MM-DD` combined with `now`'s Shanghai clock time, so a memo composed
 * for a past day still orders naturally within it. Undefined for a malformed date.
 */
export function deriveDefaultCreateTimeFromDate(value: string, now: Date = new Date()): Date | undefined {
  return parseAppDateTime(`${value} ${inAppTimeZone(now).format("HH:mm:ss")}`);
}

/** Combine the Shanghai calendar day of `date` with the Shanghai clock time of `now`. */
export function withTimeOfDay(date: Date, now: Date = new Date()): Date | undefined {
  return deriveDefaultCreateTimeFromDate(inAppTimeZone(date).format(ISO_DATE_FORMAT), now);
}

/**
 * The timestamp to send when a new memo is saved. A derived default is computed once
 * when the host selects a day and then held for every memo composed there, so if the
 * editor still carries that untouched default (same reference; the timestamp popover
 * dispatches a fresh `Date`), its time of day is re-stamped from `now` so consecutive
 * saves keep their order within the selected day. Anything else is returned as is.
 */
export function restampDefaultCreateTime(
  time: Date | undefined,
  defaultCreateTime: Date | undefined,
  now: Date = new Date(),
): Date | undefined {
  if (!time || !defaultCreateTime || time !== defaultCreateTime) return time;
  return withTimeOfDay(time, now) ?? time;
}
