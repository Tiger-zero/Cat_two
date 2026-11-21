import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { draftKeyFor, legacyDraftKeyFor, migrateLegacyDraft, resolveHomeDraftKey } from "@/components/MemoEditor/edit/draftKey";
import { cacheService } from "@/components/MemoEditor/edit/services/cacheService";
import { findFirstStagedDraftKey } from "@/components/MemoEditor/edit/services/stagedDraftFiles";

vi.mock("@/components/MemoEditor/edit/services/stagedDraftFiles", () => ({ findFirstStagedDraftKey: vi.fn() }));

describe("draft keys", () => {
  it("follow what is edited", () => {
    expect(draftKeyFor({ kind: "edit", name: "memos/a" })).toBe("edit:memos/a");
    expect(draftKeyFor({ kind: "comment", parent: "memos/a" })).toBe("comment:memos/a");
    expect(draftKeyFor({ kind: "new", scope: "home" })).toBe("new:home");
    expect(draftKeyFor({ kind: "new", scope: "home", space: "spaces/work" })).toBe("new:home:spaces/work");
    expect(draftKeyFor({ kind: "new", scope: "calendar:2026-10-18" })).toBe("new:calendar:2026-10-18");
  });

  it("name the key each host chose before, where one existed", () => {
    expect(legacyDraftKeyFor({ kind: "comment", parent: "memos/a" })).toBe("memos/a-comment");
    expect(legacyDraftKeyFor({ kind: "new", scope: "home" })).toBe("home-memo-editor");
    expect(legacyDraftKeyFor({ kind: "new", scope: "global", space: "spaces/x" })).toBe("global-memo-editor:spaces/x");
    expect(legacyDraftKeyFor({ kind: "new", scope: "calendar:2026-10-18" })).toBe("calendar-day-editor:2026-10-18");
    expect(legacyDraftKeyFor({ kind: "new", scope: "elsewhere" })).toBeUndefined();
    expect(legacyDraftKeyFor({ kind: "edit", name: "memos/a" })).toBeUndefined();
  });
});

describe("resolveHomeDraftKey", () => {
  const username = "users/1";
  const key = (suffix: string) => cacheService.key(username, suffix);
  const homeKey = key("new:home");
  const checkStagedFiles = vi.mocked(findFirstStagedDraftKey);

  beforeEach(() => {
    localStorage.clear();
    vi.stubGlobal("indexedDB", {});
    checkStagedFiles.mockReset().mockResolvedValue(undefined);
  });
  afterEach(() => {
    localStorage.clear();
    vi.unstubAllGlobals();
  });

  it("uses an existing Home draft without querying IndexedDB", async () => {
    localStorage.setItem(homeKey, "Home draft");
    localStorage.setItem(key("new:global"), "Former sidebar draft");

    await expect(resolveHomeDraftKey(username)).resolves.toBe(homeKey);
    expect(checkStagedFiles).not.toHaveBeenCalled();
  });

  it.each([
    { staged: "new:home", text: "home-memo-editor", selected: "new:home" },
    { staged: "home-memo-editor", text: "new:global", selected: "home-memo-editor" },
    { staged: "new:global", text: "global-memo-editor", selected: "new:global" },
    { staged: "new:global", text: "home-memo-editor", selected: "home-memo-editor" },
  ])("uses $selected when files and text exist under different keys", async ({ staged, text, selected }) => {
    checkStagedFiles.mockResolvedValue(key(staged));
    localStorage.setItem(key(text), "Unsent draft");

    await expect(resolveHomeDraftKey(username)).resolves.toBe(key(selected));
    expect(checkStagedFiles).toHaveBeenCalledWith([homeKey, key("home-memo-editor"), key("new:global"), key("global-memo-editor")]);
    expect(localStorage.getItem(key(text))).toBe("Unsent draft");
  });

  it("recovers text for this account when IndexedDB is unavailable", async () => {
    vi.stubGlobal("indexedDB", undefined);
    localStorage.setItem(cacheService.key("users/2", "new:home"), "Another account's draft");
    localStorage.setItem(key("global-memo-editor"), "Former sidebar draft");

    await expect(resolveHomeDraftKey(username)).resolves.toBe(key("global-memo-editor"));
    expect(checkStagedFiles).not.toHaveBeenCalled();
    expect(localStorage.getItem(key("global-memo-editor"))).toBe("Former sidebar draft");
    expect(localStorage.getItem(homeKey)).toBeNull();
  });

  it("opens Home if IndexedDB lookup fails, leaving other drafts intact", async () => {
    checkStagedFiles.mockRejectedValue(new Error("IndexedDB is blocked"));
    localStorage.setItem(key("new:global"), "Former sidebar draft");

    await expect(resolveHomeDraftKey(username)).resolves.toBe(homeKey);
    expect(localStorage.getItem(key("new:global"))).toBe("Former sidebar draft");
  });

  it("uses the normal Home key when no draft exists", async () => {
    await expect(resolveHomeDraftKey(username)).resolves.toBe(homeKey);
  });
});

describe("migrateLegacyDraft", () => {
  beforeEach(() => {
    const storage = new Map<string, string>();
    vi.stubGlobal("localStorage", {
      getItem: vi.fn((key: string) => storage.get(key) ?? null),
      setItem: vi.fn((key: string, value: string) => storage.set(key, value)),
      removeItem: vi.fn((key: string) => storage.delete(key)),
    });
  });
  afterEach(() => vi.unstubAllGlobals());

  it("moves a draft from the old key to the new one, once", () => {
    const identity = { kind: "new", scope: "home", space: "spaces/work" } as const;
    const oldKey = cacheService.key("users/1", "home-memo-editor:spaces/work");
    const newKey = cacheService.key("users/1", draftKeyFor(identity));
    cacheService.saveNow(oldKey, { content: "Unfinished", attachments: [] });

    migrateLegacyDraft("users/1", identity);

    expect(cacheService.loadDraft(newKey).content).toBe("Unfinished");
    expect(localStorage.getItem(oldKey)).toBeNull();
  });

  it("keeps a draft already under the new key", () => {
    const identity = { kind: "comment", parent: "memos/a" } as const;
    cacheService.saveNow(cacheService.key("users/1", "memos/a-comment"), { content: "old", attachments: [] });
    cacheService.saveNow(cacheService.key("users/1", draftKeyFor(identity)), { content: "new", attachments: [] });

    migrateLegacyDraft("users/1", identity);

    expect(cacheService.loadDraft(cacheService.key("users/1", draftKeyFor(identity))).content).toBe("new");
  });
});
