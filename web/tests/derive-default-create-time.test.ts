import { describe, expect, it } from "vitest";
import {
  deriveDefaultCreateTimeFromDate,
  deriveDefaultCreateTimeFromFilters,
  restampDefaultCreateTime,
  withTimeOfDay,
} from "@/components/MemoEditor/edit/utils/deriveDefaultCreateTime";
import type { MemoFilter } from "@/contexts/MemoFilterContext";
import { inAppTimeZone } from "@/lib/time-zone";

describe("deriveDefaultCreateTimeFromFilters", () => {
  const now = new Date("2026-05-02T14:32:10+08:00"); // 2026-05-02 14:32:10 in Shanghai

  it("returns undefined when no filters are set", () => {
    expect(deriveDefaultCreateTimeFromFilters([], now)).toBeUndefined();
  });

  it("returns undefined when no displayTime filter is present", () => {
    const filters: MemoFilter[] = [
      { factor: "tagSearch", value: "work" },
      { factor: "pinned", value: "true" },
    ];
    expect(deriveDefaultCreateTimeFromFilters(filters, now)).toBeUndefined();
  });

  it("merges the displayTime date with the current Shanghai time", () => {
    const filters: MemoFilter[] = [{ factor: "displayTime", value: "2025-05-01" }];
    const result = deriveDefaultCreateTimeFromFilters(filters, now);
    expect(result).toBeDefined();
    expect(result?.toISOString()).toBe("2025-05-01T06:32:10.000Z");
  });

  it("ignores extra non-displayTime filters", () => {
    const filters: MemoFilter[] = [
      { factor: "tagSearch", value: "work" },
      { factor: "displayTime", value: "2025-05-01" },
      { factor: "pinned", value: "true" },
    ];
    const result = deriveDefaultCreateTimeFromFilters(filters, now);
    expect(result?.toISOString()).toBe("2025-05-01T06:32:10.000Z");
  });

  it("returns undefined for a malformed YYYY-MM-DD value", () => {
    const cases: MemoFilter[][] = [
      [{ factor: "displayTime", value: "not-a-date" }],
      [{ factor: "displayTime", value: "2025-13-40" }],
      [{ factor: "displayTime", value: "" }],
      [{ factor: "displayTime", value: "2025-5-1" }], // single-digit month/day
    ];
    for (const filters of cases) {
      expect(deriveDefaultCreateTimeFromFilters(filters, now)).toBeUndefined();
    }
  });

  it("uses real `new Date()` when `now` is omitted", () => {
    const filters: MemoFilter[] = [{ factor: "displayTime", value: "2025-05-01" }];
    const before = new Date();
    const result = deriveDefaultCreateTimeFromFilters(filters);
    const after = new Date();
    expect(result).toBeDefined();
    // Date components must come from the filter, not from `now` — guards
    // against an impl that silently returns `new Date()` and ignores filters.
    const resultTime = inAppTimeZone(result!);
    expect(resultTime.format("YYYY-MM-DD")).toBe("2025-05-01");
    // Time of day should fall between before and after to the second.
    const resultTimeOnly = Number(resultTime.format("HHmmss"));
    const beforeTimeOnly = Number(inAppTimeZone(before).format("HHmmss"));
    const afterTimeOnly = Number(inAppTimeZone(after).format("HHmmss"));
    // Handle midnight rollover by allowing any value if before > after.
    if (beforeTimeOnly <= afterTimeOnly) {
      expect(resultTimeOnly).toBeGreaterThanOrEqual(beforeTimeOnly);
      expect(resultTimeOnly).toBeLessThanOrEqual(afterTimeOnly);
    }
  });
});

describe("withTimeOfDay", () => {
  const now = new Date("2026-10-06T13:17:23.456+08:00");

  it("keeps the day and replaces the time of day without mutating the input", () => {
    const day = new Date("2026-10-01T00:01:00+08:00");
    expect(withTimeOfDay(day, now)).toEqual(new Date("2026-10-01T05:17:23Z"));
    expect(day).toEqual(new Date("2026-10-01T00:01:00+08:00"));
  });
});

describe("restampDefaultCreateTime", () => {
  const now = new Date("2026-10-06T13:17:23+08:00");
  const defaultCreateTime = deriveDefaultCreateTimeFromDate("2026-10-01", now) as Date;

  it("re-stamps an untouched default with the time of saving", () => {
    const later = new Date("2026-10-06T13:18:10+08:00");
    expect(restampDefaultCreateTime(defaultCreateTime, defaultCreateTime, later)).toEqual(new Date("2026-10-01T05:18:10Z"));
  });

  it("gives consecutive saves distinct, ordered times within the selected day", () => {
    const first = restampDefaultCreateTime(defaultCreateTime, defaultCreateTime, new Date("2026-10-06T13:17:40+08:00")) as Date;
    const second = restampDefaultCreateTime(defaultCreateTime, defaultCreateTime, new Date("2026-10-06T13:17:55+08:00")) as Date;
    expect(first.getTime()).toBeLessThan(second.getTime());
    expect(first.toISOString()).toBe("2026-10-01T05:17:40.000Z");
    expect(second.toISOString()).toBe("2026-10-01T05:17:55.000Z");
  });

  it("leaves a manually edited timestamp alone", () => {
    const edited = new Date("2026-09-20T09:30:00+08:00");
    expect(restampDefaultCreateTime(edited, defaultCreateTime, now)).toBe(edited);
  });

  it("passes through when there is no default or no timestamp", () => {
    expect(restampDefaultCreateTime(undefined, defaultCreateTime, now)).toBeUndefined();
    const time = new Date("2026-09-20T09:30:00+08:00");
    expect(restampDefaultCreateTime(time, undefined, now)).toBe(time);
  });
});
