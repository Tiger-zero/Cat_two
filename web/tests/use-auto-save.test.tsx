import { create } from "@bufbuild/protobuf";
import { act, render, renderHook } from "@testing-library/react";
import type { PropsWithChildren } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useAutoSave } from "@/components/MemoEditor/edit/hooks/useAutoSave";
import { useStagedDraftFiles } from "@/components/MemoEditor/edit/hooks/useStagedDraftFiles";
import { initialEditorState } from "@/components/MemoEditor/edit/initialState";
import { cacheService } from "@/components/MemoEditor/edit/services/cacheService";
import * as stagedDraftFiles from "@/components/MemoEditor/edit/services/stagedDraftFiles";
import { EditorProvider, useEditorContext, useEditorStore } from "@/components/MemoEditor/edit/state";
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
    <EditorProvider initialEditorState={initialEditorState({ draftKey: key })}>
      <Probe enabled={enabled} />
    </EditorProvider>,
  );
}

const flush = () => act(() => vi.advanceTimersByTime(500));

describe("useAutoSave (store-subscribed)", () => {
  beforeEach(() => {
    cacheService.clearAll();
    vi.useFakeTimers();
    const storage = new Map<string, string>();
    vi.stubGlobal("localStorage", {
      getItem: (key: string) => storage.get(key) ?? null,
      setItem: (key: string, value: string) => storage.set(key, value),
      removeItem: (key: string) => storage.delete(key),
    });
  });

  afterEach(() => {
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
    vi.useRealTimers();
  });

  it("persists content to the draft cache when content changes", () => {
    mountEditor();
    act(() => api.dispatch(api.actions.updateContent("hello world")));
    flush();
    expect(cacheService.load(key)).toBe("hello world");
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

  it("recovers this tab's text after navigating away without changing the sibling draft", () => {
    cacheService.saveNow(key, { content: "Starting text", attachments: [] });
    const { unmount } = mountEditor();
    act(() => api.dispatch(api.actions.updateContent("This tab's changes")));
    const sibling = cacheService.saveNow(key, { content: "Sibling changes", attachments: [] });
    unmount();
    expect(cacheService.readRaw(key)).toBe(sibling);
    expect(window.dispatchEvent(new Event("beforeunload", { cancelable: true }))).toBe(false);

    const reopened = mountEditor();
    expect(api.getState().content).toBe("This tab's changes");
    act(() => api.dispatch(api.actions.updateContent("More edits in this tab")));
    flush();
    expect(cacheService.readRaw(key)).toBe(sibling);
    act(() => {
      api.discardDraft();
      api.dispatch(api.actions.reset());
    });
    flush();
    reopened.unmount();
    expect(cacheService.readForEditor(key)).toBe(sibling);
    expect(window.dispatchEvent(new Event("beforeunload", { cancelable: true }))).toBe(true);
  });

  it("recovers staged files, text, and file order after a concurrent file write rejects", async () => {
    vi.stubGlobal("indexedDB", {});
    const read = vi.spyOn(stagedDraftFiles, "loadStagedDraftFiles").mockResolvedValue(undefined);
    const write = vi.spyOn(stagedDraftFiles, "writeStagedDraftFiles").mockResolvedValue(false);
    const sibling = cacheService.saveNow(key, { content: "Shared text", attachments: [] });
    const report = vi.fn();
    const stale = vi.fn();
    const createBlobUrl = () => "blob:restored";
    const mount = () =>
      renderHook(
        () => {
          const autosave = useAutoSave(key);
          const discardFiles = useStagedDraftFiles({
            key,
            enabled: true,
            getBaseRevision: autosave.getFilesRevision,
            createBlobUrl,
            onStorageError: report,
            onStaleDraft: stale,
            preserveDraft: autosave.preserveDraft,
          });
          return { ...autosave, discardFiles, editor: useEditorStore() };
        },
        {
          wrapper: ({ children }: PropsWithChildren) => (
            <EditorProvider initialEditorState={initialEditorState({ draftKey: key })}>{children}</EditorProvider>
          ),
        },
      );

    const first = mount();
    await act(async () => {});
    const file = new File(["my attachment"], "draft.txt", { type: "text/plain" });
    await act(async () => {
      const { actions, dispatch } = first.result.current.editor;
      dispatch(actions.updateContent("This tab's text and file"));
      dispatch(actions.setLocalFiles([{ file, previewUrl: "blob:previous" }]));
      dispatch(actions.setAttachmentOrder(["blob:previous"]));
    });
    expect(report).toHaveBeenCalledOnce();
    expect(write).toHaveBeenCalled();
    first.unmount();

    const reopened = mount();
    expect(reopened.result.current.editor.getState()).toMatchObject({
      content: "This tab's text and file",
      localFiles: [{ file, previewUrl: "blob:restored" }],
      attachmentOrder: ["blob:restored"],
    });
    expect(read).toHaveBeenCalledOnce();
    await act(async () => {
      await reopened.result.current.discardFiles();
      reopened.result.current.discardDraft();
    });
    reopened.unmount();
    expect(cacheService.readForEditor(key)).toBe(sibling);
  });
});
