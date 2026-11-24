import { accessSessionID, endAccessSession, getAccessToken } from "@/auth-state";
import { authenticatedFetch } from "@/connect";
import { AUTH_REASON_IDLE_SESSION } from "@/utils/redirect-safety";
import { extendIdleDeadlineForEdit, hasIdleDeadlineElapsed, observeIdleDeadline, SESSION_IDLE_EXPIRED_MESSAGE } from "./session-idle";

const SYNC_INTERVAL_MS = 1_000;

let queuedSession = "";
let lastSentAt = 0;
let timer: number | undefined;

function sendEditingActivity(session: string): void {
  const token = getAccessToken();
  if (accessSessionID(token) !== session || hasIdleDeadlineElapsed(token)) return;
  lastSentAt = Date.now();
  void authenticatedFetch("/internal/session/activity", { method: "POST", keepalive: true }, session)
    .then(async (response) => {
      if (session !== accessSessionID(getAccessToken())) return;
      if (response.ok) {
        const { idleExpiresAt } = (await response.json()) as { idleExpiresAt?: string };
        observeIdleDeadline(token, idleExpiresAt, response.headers.get("Date"));
      } else if (response.status === 401) {
        const { message } = (await response.json()) as { message?: string };
        if (message === SESSION_IDLE_EXPIRED_MESSAGE && session === accessSessionID(getAccessToken())) {
          endAccessSession(token, AUTH_REASON_IDLE_SESSION);
        }
      }
    })
    .catch(() => {
      // A temporary network failure should leave the draft and local editing session available.
    });
}

/** Called by editor actions after a user changes a note. */
export function recordMemoEditingActivity(): void {
  const token = getAccessToken();
  const session = accessSessionID(token);
  if (!session || !extendIdleDeadlineForEdit(token)) return;

  if (queuedSession !== session) {
    window.clearTimeout(timer);
    queuedSession = session;
    lastSentAt = 0;
    timer = undefined;
  }
  if (timer !== undefined) return;
  const delay = lastSentAt + SYNC_INTERVAL_MS - Date.now();
  if (delay <= 0) {
    sendEditingActivity(session);
  } else {
    timer = window.setTimeout(() => {
      timer = undefined;
      sendEditingActivity(session);
    }, delay);
  }
}
