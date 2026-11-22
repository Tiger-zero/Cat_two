import { create, toBinary } from "@bufbuild/protobuf";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { Code } from "@connectrpc/connect";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { RefreshTokenResponseSchema } from "@/types/proto/api/auth_service_pb";
import { redirectOnAuthFailure } from "@/utils/auth-redirect";

vi.mock("@/utils/auth-redirect", () => ({ redirectOnAuthFailure: vi.fn() }));

const refreshPath = "/memos.api.AuthService/RefreshToken";
const testTokenExpiry = () => new Date(Date.now() + 600_000);

async function openTab() {
  vi.resetModules();
  const auth = await import("@/auth-state");
  const connect = await import("@/connect");
  return { ...auth, ...connect };
}

function refreshResponse(token: string) {
  const message = create(RefreshTokenResponseSchema, {
    accessToken: token,
    accessTokenExpireTime: timestampFromDate(testTokenExpiry()),
  });
  return new Response(toBinary(RefreshTokenResponseSchema, message) as Uint8Array<ArrayBuffer>, {
    headers: { "content-type": "application/proto" },
  });
}

function unauthorizedRefresh() {
  return Response.json({ code: "unauthenticated", message: "refresh revoked" }, { status: 401 });
}

function browserToken(sessionID: string, revision = 1) {
  return `test.${btoa(JSON.stringify({ sid: sessionID, revision })).replace(/=+$/u, "")}.token`;
}

describe("rotation of the refresh cookie shared by tabs", () => {
  beforeEach(() => {
    localStorage.clear();
    vi.stubGlobal("BroadcastChannel", undefined);
    vi.stubGlobal("navigator", { locks: undefined });
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    localStorage.clear();
  });

  it("serializes refresh in two tabs and reuses the first tab's new token without a broadcast", async () => {
    let pending = Promise.resolve();
    const request = vi.fn((_name: string, task: () => Promise<void>) => {
      const next = pending.then(task);
      pending = next.catch(() => {});
      return next;
    });
    vi.stubGlobal("navigator", { locks: { request } });
    const fetch = vi.fn(async () => refreshResponse("new-access-token"));
    vi.stubGlobal("fetch", fetch);
    const first = await openTab();
    first.setAccessToken("old-access-token", testTokenExpiry());
    const second = await openTab();
    expect(second.getAccessToken()).toBe("old-access-token");

    await Promise.all([first.refreshAccessToken(), second.refreshAccessToken()]);

    expect(request).toHaveBeenCalledTimes(2);
    expect(fetch).toHaveBeenCalledTimes(1);
    expect(first.getAccessToken()).toBe("new-access-token");
    expect(second.getAccessToken()).toBe("new-access-token");
  });

  it("retries an authenticated fetch with the shared token when it was published before the response arrived", async () => {
    const first = await openTab();
    first.setAccessToken("old-access-token", testTokenExpiry());
    const second = await openTab();
    const fetch = vi.fn(async (input: URL, init: RequestInit) => {
      expect(input.pathname).toBe("/api/trash");
      const authorization = new Headers(init.headers).get("Authorization");
      if (authorization === "Bearer old-access-token") {
        first.setAccessToken("new-access-token", testTokenExpiry());
        return new Response(null, { status: 401 });
      }
      expect(authorization).toBe("Bearer new-access-token");
      return Response.json({ memos: [] });
    });
    vi.stubGlobal("fetch", fetch);

    expect(await second.authenticatedFetch("/api/trash")).toMatchObject({ status: 200 });
    expect(fetch).toHaveBeenCalledTimes(2);
    expect(redirectOnAuthFailure).not.toHaveBeenCalled();
  });

  it("recovers the winning token from shared storage after a conflicting refresh on LAN HTTP", async () => {
    const first = await openTab();
    first.setAccessToken("old-access-token", testTokenExpiry());
    const second = await openTab();
    const fetch = vi.fn(async (input: string) => {
      expect(new URL(input).pathname).toBe(refreshPath);
      first.setAccessToken("new-access-token", testTokenExpiry());
      return unauthorizedRefresh();
    });
    vi.stubGlobal("fetch", fetch);

    await second.refreshAccessToken();

    expect(fetch).toHaveBeenCalledTimes(1);
    expect(second.getAccessToken()).toBe("new-access-token");
  });

  it("propagates a rejected refresh when the session cannot be recovered", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => unauthorizedRefresh()),
    );
    const tab = await openTab();
    tab.setAccessToken("old-access-token", testTokenExpiry());

    await expect(tab.refreshAccessToken()).rejects.toMatchObject({ code: Code.Unauthenticated });
  });

  it("uses the existing cookie once when upgrading an unbound access token", async () => {
    const tab = await openTab();
    tab.setAccessToken("legacy-access-token", testTokenExpiry());
    const upgraded = browserToken("migrated-session");
    const fetch = vi.fn(async (input: URL | string, init: RequestInit) => {
      const headers = new Headers(init.headers);
      if (new URL(input).pathname === refreshPath) {
        expect(headers.get("x-memos-session-id")).toBeNull();
        return refreshResponse(upgraded);
      }
      if (headers.get("Authorization") === "Bearer legacy-access-token") return unauthorizedRefresh();
      expect(headers.get("Authorization")).toBe(`Bearer ${upgraded}`);
      return Response.json({ memos: [] });
    });
    vi.stubGlobal("fetch", fetch);

    await expect(tab.authenticatedFetch("/api/trash")).resolves.toMatchObject({ status: 200 });
    expect(tab.getAccessToken()).toBe(upgraded);
    expect(fetch).toHaveBeenCalledTimes(3);
  });

  it("ends the same session in another tab and preserves a different session", async () => {
    const first = await openTab();
    const originalToken = browserToken("sign-out-target");
    const otherToken = browserToken("other-session");
    first.setAccessToken(originalToken, testTokenExpiry());
    const peer = await openTab();
    expect(peer.getAccessToken()).toBe(originalToken);
    const peerEnded = vi.fn();
    const unsubscribePeer = peer.onAccessSessionEnd(peerEnded);
    const other = await openTab();
    other.setAccessToken(otherToken, testTokenExpiry());
    const otherEnded = vi.fn();
    const unsubscribeOther = other.onAccessSessionEnd(otherEnded);

    first.endAccessSession(originalToken);
    window.dispatchEvent(new StorageEvent("storage", { key: "memos_signed_out_session", newValue: "sign-out-target" }));

    expect(peerEnded).toHaveBeenCalledOnce();
    expect(peer.getAccessToken()).toBeNull();
    expect(otherEnded).not.toHaveBeenCalled();
    expect(other.getAccessToken()).toBe(otherToken);
    expect(localStorage.getItem("memos_access_token")).toBe(otherToken);
    unsubscribePeer();
    unsubscribeOther();
  });

  it("keeps a tab signed out if its in-flight refresh finishes after logout", async () => {
    const tab = await openTab();
    const originalToken = browserToken("interrupted-session");
    tab.setAccessToken(originalToken, testTokenExpiry());
    let completeRefresh!: (response: Response) => void;
    const fetch = vi.fn(async (_input: string, init: RequestInit) => {
      expect(new Headers(init.headers).get("x-memos-session-id")).toBe("interrupted-session");
      return new Promise<Response>((resolve) => {
        completeRefresh = resolve;
      });
    });
    vi.stubGlobal("fetch", fetch);

    const pending = tab.refreshAccessToken();
    await vi.waitFor(() => expect(fetch).toHaveBeenCalledOnce());
    tab.endAccessSession(originalToken);
    completeRefresh(refreshResponse(browserToken("interrupted-session", 2)));

    await expect(pending).rejects.toMatchObject({ code: Code.Unauthenticated });
    expect(tab.getAccessToken()).toBeNull();
    expect(localStorage.getItem("memos_access_token")).toBeNull();
  });
});
