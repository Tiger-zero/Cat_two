import { create } from "@bufbuild/protobuf";
import { act, render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useAutoSave } from "@/components/MemoEditor/edit/hooks/useAutoSave";
import { cacheService } from "@/components/MemoEditor/edit/services/cacheService";
import { EditorProvider, useEditorContext } from "@/components/MemoEditor/edit/state";
import { AttachmentSchema } from "@/types/proto/api/attachment_service_pb";

const key = cacheService.key("users/steven", "new:home");
let api: ReturnType<typeof useEditorContext> & { discardDraft: () => void };

function Probe({ enabled }: { enabled: boolean }) {
  const editor = useEditorContext();
  const { discardDraft } = useAutoSave(key, enabled);
  api = { ...editor, discardDraft };
  return null;
}

function mountEditor(enabled = true) {
  return render(
    <EditorProvider>
      <Probe enabled={enabled} />
    </EditorProvider>,
  );
}

const flush = () => act(() => vi.advanceTimersByTime(500));

describe("useAutoSave (store-subscribed)", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    const storage = new Map<string, string>();
    vi.stubGlobal("localStorage", {
      getItem: (key: string) => storage.get(key) ?? null,
      setItem: (key: string, value: string) => storage.set(key, value),
      removeItem: (key: string) => storage.delete(key),
    });
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    vi.useRealTimers();
  });

  it("persists content to the draft cache when content changes", () => {
    mountEditor();
    act(() => api.dispatch(api.actions.updateContent("hello world")));
    flush();
    expect(cacheService.load(key)).toBe("hello world");
  });

  it("persists destination changes without content changes", () => {
    mountEditor();
    act(() => api.dispatch(api.actions.setMetadata({ space: "spaces/work" })));
    flush();
    expect(cacheService.loadDraft(key).space).toBe("spaces/work");
    act(() => api.dispatch(api.actions.setMetadata({ space: undefined })));
    flush();
    expect(cacheService.loadDraft(key).space).toBeUndefined();
  });

  it("persists uploaded attachment metadata, including changes to the same name", () => {
    mountEditor();
    const initial = create(AttachmentSchema, { name: "attachments/image-one", filename: "image.png", type: "image/png" });
    act(() => api.dispatch(api.actions.setMetadata({ attachments: [initial] })));
    flush();
    expect(cacheService.loadDraft(key).attachments).toEqual([initial]);

    const updated = { ...initial, externalLink: "https://cdn.example.com/image.png" };
    act(() => api.dispatch(api.actions.setMetadata({ attachments: [updated] })));
    flush();
    expect(cacheService.loadDraft(key).attachments).toEqual([updated]);
  });

  it("does not persist when disabled", () => {
    mountEditor(false);
    act(() => api.dispatch(api.actions.updateContent("ignored")));
    flush();
    expect(cacheService.readRaw(key)).toBeNull();
  });

  it("discardDraft clears the cache and suppresses the unmount flush", () => {
    const { unmount } = mountEditor();
    act(() => api.dispatch(api.actions.updateContent("draft body")));
    flush();
    expect(cacheService.load(key)).toBe("draft body");

    act(() => api.discardDraft());
    unmount();
    expect(cacheService.readRaw(key)).toBeNull();
  });

  it.each(["cancel", "close"])("does not overwrite a draft changed by another tab on %s", (action) => {
    cacheService.saveNow(key, { content: "Previous draft", attachments: [] });
    const { unmount } = mountEditor();
    act(() => api.dispatch(api.actions.updateContent("Stale tab text")));
    cacheService.saveNow(key, { content: "Newer tab text", attachments: [] });
    if (action === "cancel") act(() => api.discardDraft());
    unmount();

    expect(cacheService.load(key)).toBe("Newer tab text");
  });
});
