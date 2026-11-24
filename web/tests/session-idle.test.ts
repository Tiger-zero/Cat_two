import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { clearAccessToken, getAccessToken, onAccessSessionEnd, setAccessToken } from "@/auth-state";
import { authenticatedFetch } from "@/connect";
import { recordMemoEditingActivity } from "@/lib/memo-editing-activity";
import { monitorIdleSession, observeIdleDeadline } from "@/lib/session-idle";

vi.mock("@/connect", () => ({ authenticatedFetch: vi.fn() }));

const start = Date.parse("2026-11-22T12:00:00Z");
const idleLimit = 45 * 60_000;
const iso = (value: number) => new Date(value).toISOString();
const deadlineKey = (session: string) => `memos_edit_idle:${session}`;

function startSession(session: string) {
  const token = `test.${btoa(JSON.stringify({ sid: session }))}.token`;
  setAccessToken(token, new Date(Date.now() + 15 * 60_000));
  return token;
}

describe("browser idle editing deadline", () => {
  const cleanup: (() => void)[] = [];

  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(start);
    vi.mocked(authenticatedFetch).mockReset();
    vi.mocked(authenticatedFetch).mockRejectedValue(new Error("network unavailable"));
  });

  afterEach(() => {
    for (const dispose of cleanup.splice(0)) dispose();
    clearAccessToken();
    localStorage.clear();
    vi.useRealTimers();
  });

  function trackSessionEnd() {
    const onEnd = vi.fn();
    cleanup.push(onAccessSessionEnd(onEnd), monitorIdleSession());
    return onEnd;
  }

  it("expires on wake and uses the original deadline when a passive response reports the same deadline", () => {
    const token = startSession("passive");
    const onEnd = trackSessionEnd();
    const serverStart = start - 5 * 60_000;
    const serverExpiry = iso(serverStart + idleLimit);
    observeIdleDeadline(token, serverExpiry, iso(serverStart));
    vi.setSystemTime(start + 15_500);
    observeIdleDeadline(token, serverExpiry, iso(serverStart + 15_000));
    expect(localStorage.getItem(deadlineKey("passive"))).toBe(String(start + idleLimit));
    expect(authenticatedFetch).not.toHaveBeenCalled();

    vi.setSystemTime(start + idleLimit);
    window.dispatchEvent(new Event("pageshow"));
    expect(onEnd).toHaveBeenCalledExactlyOnceWith("idle-session");
    expect(getAccessToken()).toBeNull();
    recordMemoEditingActivity();
    expect(authenticatedFetch).not.toHaveBeenCalled();
  });

  it("reads another tab's later edit before an older timer can end the shared session", () => {
    const token = startSession("peer");
    const onEnd = trackSessionEnd();
    observeIdleDeadline(token, iso(start + idleLimit), iso(start));
    vi.advanceTimersByTime(10_000);
    localStorage.setItem(deadlineKey("peer"), String(Date.now() + idleLimit));

    vi.advanceTimersByTime(idleLimit - 10_000);
    expect(onEnd).not.toHaveBeenCalled();
    expect(getAccessToken()).toBe(token);
    vi.advanceTimersByTime(10_000);
    expect(onEnd).toHaveBeenCalledExactlyOnceWith("idle-session");
  });

  it("persists real editor events immediately and tolerates transient activity delivery errors", async () => {
    const token = startSession("editing");
    const onEnd = trackSessionEnd();
    observeIdleDeadline(token, iso(start + idleLimit), iso(start));
    vi.advanceTimersByTime(30_000);
    recordMemoEditingActivity();
    expect(localStorage.getItem(deadlineKey("editing"))).toBe(String(Date.now() + idleLimit));
    expect(authenticatedFetch).toHaveBeenCalledExactlyOnceWith(
      "/internal/session/activity",
      { method: "POST", keepalive: true },
      "editing",
    );

    await vi.advanceTimersByTimeAsync(100);
    recordMemoEditingActivity();
    expect(localStorage.getItem(deadlineKey("editing"))).toBe(String(Date.now() + idleLimit));
    expect(authenticatedFetch).toHaveBeenCalledTimes(1);
    await vi.advanceTimersByTimeAsync(900);
    expect(authenticatedFetch).toHaveBeenCalledTimes(2);
    expect(onEnd).not.toHaveBeenCalled();
    expect(getAccessToken()).toBe(token);
  });

  it("does not deliver an older session's queued edit through a new sign-in", async () => {
    const first = startSession("previous-login");
    observeIdleDeadline(first, iso(start + idleLimit), iso(start));
    recordMemoEditingActivity();
    recordMemoEditingActivity();
    const active = startSession("new-login");
    await vi.advanceTimersByTimeAsync(1_000);

    expect(authenticatedFetch).toHaveBeenCalledExactlyOnceWith(
      "/internal/session/activity",
      { method: "POST", keepalive: true },
      "previous-login",
    );
    expect(getAccessToken()).toBe(active);
  });
});
