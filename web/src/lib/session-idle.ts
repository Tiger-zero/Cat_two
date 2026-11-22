import { accessSessionID, endAccessSession, getAccessSessionToken, onAccessTokenChange } from "@/auth-state";
import { AUTH_REASON_IDLE_SESSION } from "@/utils/redirect-safety";

const IDLE_LIMIT_MS = 45 * 60_000;
const DEADLINE_KEY_PREFIX = "memos_edit_idle:";
const CLOCK_KEY_PREFIX = "memos_edit_idle_clock:";
const IDLE_DEADLINE_HEADER = "X-Memos-Session-Idle-Expires-At";
export const SESSION_IDLE_EXPIRED_MESSAGE = "session idle expired";

const deadlines = new Map<string, number>();
const clockOffsets = new Map<string, number>();
const deadlineListeners = new Set<() => void>();

function readDeadline(session: string): number {
  let expiresAt = deadlines.get(session) ?? 0;
  try {
    const sharedDeadline = Number(localStorage.getItem(DEADLINE_KEY_PREFIX + session));
    if (Number.isFinite(sharedDeadline)) expiresAt = Math.max(expiresAt, sharedDeadline);
  } catch {
    // The tab can still enforce its deadline in memory when storage is disabled.
  }
  return expiresAt;
}

function saveDeadline(session: string, expiresAt: number): void {
  if (!session || expiresAt <= readDeadline(session)) return;
  deadlines.set(session, expiresAt);
  try {
    localStorage.setItem(DEADLINE_KEY_PREFIX + session, String(expiresAt));
  } catch {
    // The monitor below still hears updates from this tab.
  }
  for (const listener of deadlineListeners) listener();
}

export function observeIdleDeadline(token: string | null, expiresAt: string | null | undefined, serverDate: string | null): void {
  const session = accessSessionID(token);
  const serverExpiry = Date.parse(expiresAt ?? "");
  if (!session || !Number.isFinite(serverExpiry)) return;
  // Choose the clock offset once per session. Passive responses carrying the
  // same server deadline must not extend it through rounding or network delay.
  let offset = clockOffsets.get(session);
  if (offset === undefined) {
    const key = CLOCK_KEY_PREFIX + session;
    try {
      const stored = localStorage.getItem(key);
      if (stored !== null && Number.isFinite(Number(stored))) offset = Number(stored);
    } catch {
      // The initial clock sample can stay in this tab's memory.
    }
    if (offset === undefined) {
      const skew = Date.now() - Date.parse(serverDate ?? "");
      offset = Number.isFinite(skew) && Math.abs(skew) > 2_000 ? skew : 0;
      try {
        localStorage.setItem(key, String(offset));
      } catch {
        // The initial clock sample can stay in this tab's memory.
      }
    }
    clockOffsets.set(session, offset);
  }
  saveDeadline(session, serverExpiry + offset);
}

export function observeIdleDeadlineHeader(token: string | null, headers: Headers): void {
  observeIdleDeadline(token, headers.get(IDLE_DEADLINE_HEADER), headers.get("Date"));
}

export function hasIdleDeadlineElapsed(token: string | null): boolean {
  const deadline = readDeadline(accessSessionID(token));
  return deadline > 0 && deadline <= Date.now();
}

export function extendIdleDeadlineForEdit(token: string | null): boolean {
  const session = accessSessionID(token);
  const deadline = readDeadline(session);
  if (!deadline) return false;
  const now = Date.now();
  if (deadline <= now) {
    endAccessSession(token, AUTH_REASON_IDLE_SESSION);
    return false;
  }
  saveDeadline(session, now + IDLE_LIMIT_MS);
  return true;
}

/** Keep the expiry visible even without outgoing requests. */
export function monitorIdleSession(): () => void {
  let timer: number | undefined;
  const check = () => {
    window.clearTimeout(timer);
    const token = getAccessSessionToken();
    const deadline = readDeadline(accessSessionID(token));
    if (!deadline) return;
    const delay = deadline - Date.now();
    if (delay > 0) {
      timer = window.setTimeout(check, Math.min(delay, IDLE_LIMIT_MS));
    } else {
      endAccessSession(token, AUTH_REASON_IDLE_SESSION);
    }
  };
  const onStorage = (event: StorageEvent) => {
    if (event.key === null || event.key.startsWith(DEADLINE_KEY_PREFIX)) check();
  };
  deadlineListeners.add(check);
  const unsubscribe = onAccessTokenChange(check);
  window.addEventListener("storage", onStorage);
  window.addEventListener("focus", check);
  window.addEventListener("pageshow", check);
  document.addEventListener("visibilitychange", check);
  check();
  return () => {
    window.clearTimeout(timer);
    deadlineListeners.delete(check);
    unsubscribe();
    window.removeEventListener("storage", onStorage);
    window.removeEventListener("focus", check);
    window.removeEventListener("pageshow", check);
    document.removeEventListener("visibilitychange", check);
  };
}
