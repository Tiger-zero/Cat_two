import { create } from "@bufbuild/protobuf";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cacheService } from "@/components/MemoEditor/edit/services/cacheService";
import { AttachmentSchema, MotionMediaFamily, MotionMediaRole, MotionMediaSchema } from "@/types/proto/api/attachment_service_pb";

describe("memo editor cache", () => {
  beforeEach(() => {
    const storage = new Map<string, string>();
    vi.stubGlobal("localStorage", {
      getItem: vi.fn((key: string) => storage.get(key) ?? null),
      setItem: vi.fn((key: string, value: string) => storage.set(key, value)),
      removeItem: vi.fn((key: string) => storage.delete(key)),
    });
    cacheService.clearAll();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("stores draft content", () => {
    const key = cacheService.key("users/steven", "home-memo-editor");

    cacheService.saveNow(key, { content: "- [x] Draft task", attachments: [] });

    expect(cacheService.load(key)).toBe("- [x] Draft task");
  });

  it("removes empty draft content instead of caching it", () => {
    const key = cacheService.key("users/steven", "home-memo-editor");

    cacheService.saveNow(key, { content: "", attachments: [] });

    expect(cacheService.load(key)).toBe("");
  });

  it("loads content from previously structured draft entries", () => {
    const key = cacheService.key("users/steven", "home-memo-editor");
    localStorage.setItem(key, JSON.stringify({ kind: "memos.editor-cache", version: 1, content: "- [ ] migrated task" }));

    expect(cacheService.load(key)).toBe("- [ ] migrated task");
  });

  it("round-trips uploaded attachment metadata with a structured draft", () => {
    const key = cacheService.key("users/steven", "home-memo-editor");
    const attachment = create(AttachmentSchema, {
      name: "attachments/image-one",
      filename: "garden.png",
      externalLink: "https://cdn.example.com/garden.png",
      type: "image/png",
      size: 42n,
      motionMedia: create(MotionMediaSchema, {
        family: MotionMediaFamily.APPLE_LIVE_PHOTO,
        role: MotionMediaRole.STILL,
        groupId: "live-one",
      }),
    });

    cacheService.saveNow(key, { content: "![garden](/file/attachments/image-one)", attachments: [attachment] });

    const restored = cacheService.loadDraft(key);
    expect(restored.content).toBe("![garden](/file/attachments/image-one)");
    expect(restored.attachments).toHaveLength(1);
    expect(restored.attachments[0]).toMatchObject({
      name: "attachments/image-one",
      filename: "garden.png",
      externalLink: "https://cdn.example.com/garden.png",
      type: "image/png",
      size: 42n,
      motionMedia: {
        family: MotionMediaFamily.APPLE_LIVE_PHOTO,
        role: MotionMediaRole.STILL,
        groupId: "live-one",
      },
    });
  });

  it("keeps raw JSON markdown drafts intact", () => {
    const key = cacheService.key("users/steven", "home-memo-editor");
    const jsonDraft = '{"content":"not a cache envelope"}';
    localStorage.setItem(key, jsonDraft);

    expect(cacheService.load(key)).toBe(jsonDraft);
  });

  it("keeps structured-looking drafts without a supported version intact", () => {
    const key = cacheService.key("users/steven", "home-memo-editor");
    const jsonDraft = JSON.stringify({ kind: "memos.editor-cache", content: "not a supported envelope" });
    localStorage.setItem(key, jsonDraft);

    expect(cacheService.load(key)).toBe(jsonDraft);
  });

  it("ignores retired settings while preserving a legacy draft's content and attachments", () => {
    localStorage.setItem(
      "legacy-location",
      JSON.stringify({
        kind: "memos.editor-cache",
        version: 3,
        content: "old",
        attachments: [{ name: "attachments/a", filename: "a.txt" }],
        location: null,
        space: "spaces/work",
        visibility: 999,
      }),
    );
    expect(cacheService.loadDraft("legacy-location")).toMatchObject({
      content: "old",
      attachments: [{ name: "attachments/a", filename: "a.txt" }],
    });
    expect(cacheService.loadDraft("legacy-location")).not.toHaveProperty("location");
    expect(cacheService.loadDraft("legacy-location")).not.toHaveProperty("space");
  });

  it("keeps the cursor for the next editor mount", () => {
    const key = cacheService.key("users/steven", "new:home");

    const selection = { anchor: { block: 1, offset: 9 }, focus: { block: 1, offset: 9 } };
    cacheService.saveCursor(key, selection, "rich");

    expect(cacheService.loadCursor(key, "rich")).toEqual(selection);
    cacheService.clear(key);
    expect(cacheService.loadCursor(key, "rich")).toBeUndefined();
  });

  it("drops a cursor taken in the other editor mode", () => {
    const key = cacheService.key("users/steven", "new:home");

    // Source mode is one block of Markdown, so its offsets mean nothing to rich mode.
    cacheService.saveCursor(key, { anchor: { block: 0, offset: 42 }, focus: { block: 0, offset: 42 } }, "source");

    expect(cacheService.loadCursor(key, "rich")).toBeUndefined();
    expect(cacheService.loadCursor(key, "source")).toBeDefined();
  });
});
