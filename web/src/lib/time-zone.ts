import dayjs from "dayjs";
import utc from "dayjs/plugin/utc";

dayjs.extend(utc);

export const APP_TIME_ZONE = "Asia/Shanghai";
export const APP_DATE_TIME_FORMAT = "YYYY-MM-DD HH:mm:ss";

const wallClockFormatter = new Intl.DateTimeFormat("en-CA", {
  timeZone: APP_TIME_ZONE,
  year: "numeric",
  month: "2-digit",
  day: "2-digit",
  hour: "2-digit",
  minute: "2-digit",
  second: "2-digit",
  hourCycle: "h23",
});

/** A date-only calendar value for arithmetic, represented independently of the device clock. */
export const calendarDate = (date: string) => dayjs.utc(date);

/** Shanghai calendar and clock fields for display and calendar arithmetic, represented on a UTC clock. */
export function inAppTimeZone(date: Date = new Date()) {
  const { year, month, day, hour, minute, second } = Object.fromEntries(
    wallClockFormatter.formatToParts(date).map(({ type, value }) => [type, value]),
  );
  const milliseconds = String(date.getUTCMilliseconds()).padStart(3, "0");
  return dayjs.utc(`${year.padStart(4, "0")}-${month}-${day}T${hour}:${minute}:${second}.${milliseconds}Z`);
}

/** Resolve a Shanghai wall-clock timestamp to an instant. Reject overflow and nonexistent local times. */
export function parseAppDateTime(value: string): Date | undefined {
  if (!/^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}$/.test(value) || value.startsWith("0000")) return undefined;
  const wallTime = Date.parse(`${value.replace(" ", "T")}Z`);
  if (!Number.isFinite(wallTime)) return undefined;

  const estimatedOffset = inAppTimeZone(new Date(wallTime)).valueOf() - wallTime;
  const estimate = new Date(wallTime - estimatedOffset);
  // The offset at the wall-clock estimate can differ from the offset at the actual instant.
  const offset = inAppTimeZone(estimate).valueOf() - estimate.getTime();
  const instant = new Date(wallTime - offset);
  return inAppTimeZone(instant).format(APP_DATE_TIME_FORMAT) === value ? instant : undefined;
}
