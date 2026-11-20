import { afterEach, describe, expect, it, vi } from "vitest";
import { getCurrentMonth, getLocalDayTimestampRange, getLocalMonthTimestampRange, getToday } from "@/lib/calendar-utils";
import { APP_DATE_TIME_FORMAT, inAppTimeZone, parseAppDateTime } from "@/lib/time-zone";

describe("Shanghai calendar time", () => {
  afterEach(() => vi.useRealTimers());

  it("moves into the new calendar month at 16:00 UTC even on a different device time zone", () => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-11-30T15:59:59Z"));
    expect(getToday()).toBe("2026-11-30");
    expect(getCurrentMonth()).toBe("2026-11");
    vi.setSystemTime(new Date("2026-11-30T16:00:00Z"));
    expect(getToday()).toBe("2026-12-01");
    expect(getCurrentMonth()).toBe("2026-12");
  });

  it("queries day and month boundaries as half-open ranges of epoch seconds", () => {
    expect(getLocalDayTimestampRange("2026-12-01")).toEqual({
      startTimestamp: Date.parse("2026-11-30T16:00:00Z") / 1000,
      endTimestamp: Date.parse("2026-12-01T16:00:00Z") / 1000,
    });
    expect(getLocalMonthTimestampRange("2026-12")).toEqual({
      startTimestamp: Date.parse("2026-11-30T16:00:00Z") / 1000,
      endTimestamp: Date.parse("2026-12-31T16:00:00Z") / 1000,
    });
  });

  it("round-trips a timestamp at Shanghai midnight and rejects an invalid calendar date", () => {
    const instant = new Date("2026-11-30T16:05:00Z");
    expect(inAppTimeZone(instant).format(APP_DATE_TIME_FORMAT)).toBe("2026-12-01 00:05:00");
    expect(parseAppDateTime("2026-12-01 00:05:00")).toEqual(instant);
    expect(parseAppDateTime("2026-02-29 00:05:00")).toBeUndefined();
  });
});
