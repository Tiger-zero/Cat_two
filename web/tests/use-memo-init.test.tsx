import { create } from "@bufbuild/protobuf";
import { act, render } from "@testing-library/react";
import type { EditorMode } from "@usememos/editor";
import type { RefObject } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { memoDraftRevision } from "@/components/MemoEditor/edit/draftRevision";
import { useMemoInit } from "@/components/MemoEditor/edit/hooks/useMemoInit";
import { initialEditorState } from "@/components/MemoEditor/edit/initialState";
import { cacheService } from "@/components/MemoEditor/edit/services/cacheService";
import { EditorProvider, useEditorContext } from "@/components/MemoEditor/edit/state";
import type { EditorController } from "@/components/MemoEditor/edit/types/editorController";
import { AttachmentSchema } from "@/types/proto/api/attachment_service_pb";
import { MemoSchema, Visibility } from "@/types/proto/api/memo_service_pb";

const editorRef = { current: null } as RefObject<EditorController | null>;
const draftKey = cacheService.key("users/steven", "restored-draft");
let getEditorState: ReturnType<typeof useEditorContext>["getState"];

function Probe({ autoFocus, mode = "rich" }: { autoFocus?: boolean | (() => boolean); mode?: EditorMode }) {
  getEditorState = useEditorContext().getState;
  useMemoInit({ editorRef, draftKey, autoFocus, mode });
  return null;
}

/** Mounts a session the way MemoEditor does: the draft read once, then the caret restored. */
const mountSession = (props: { autoFocus?: boolean | (() => boolean); mode?: EditorMode }) =>
  render(
    <EditorProvider
      initialEditorState={initialEditorState({
        draftKey,
      })}
    >
      <Probe autoFocus={props.autoFocus} mode={props.mode} />
    </EditorProvider>,
  );

describe("the initial draft and useMemoInit", () => {
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
    editorRef.current = null;
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it.each([true, false])("restores the draft and cursor before evaluating autofocus (%s)", (allowed) => {
    vi.useFakeTimers();
    cacheService.saveNow(draftKey, { content: "An unfinished memo", attachments: [] });
    const selection = { anchor: { block: 0, offset: 5 }, focus: { block: 0, offset: 5 } };
    cacheService.saveCursor(draftKey, selection, "rich");
    const focus = vi.fn();
    const setSelection = vi.fn();
    editorRef.current = { focus, setSelection } as unknown as EditorController;
    const autoFocus = vi.fn(() => {
      expect(getEditorState().content).toBe("An unfinished memo");
      expect(setSelection).toHaveBeenCalledWith(selection);
      return allowed;
    });

    mountSession({ autoFocus });
    expect(getEditorState().content).toBe("An unfinished memo");
    expect(autoFocus).not.toHaveBeenCalled();
    act(() => vi.advanceTimersByTime(100));
    expect(autoFocus).toHaveBeenCalledOnce();
    expect(focus).toHaveBeenCalledTimes(allowed ? 1 : 0);
  });

  it("does not restore a cursor taken in the other editor mode", () => {
    vi.useFakeTimers();
    cacheService.saveNow(draftKey, { content: "An unfinished memo", attachments: [] });
    cacheService.saveCursor(draftKey, { anchor: { block: 0, offset: 12 }, focus: { block: 0, offset: 12 } }, "source");
    const setSelection = vi.fn();
    editorRef.current = { focus: vi.fn(), setSelection } as unknown as EditorController;

    mountSession({ mode: "rich" });
    act(() => vi.advanceTimersByTime(100));
    expect(getEditorState().content).toBe("An unfinished memo");
    expect(setSelection).not.toHaveBeenCalled();
  });

  it("restores uploaded attachment bindings with a new memo draft", () => {
    const attachment = create(AttachmentSchema, { name: "attachments/image-one", filename: "image.png", type: "image/png" });
    cacheService.saveNow(draftKey, { content: "![image](/file/attachments/image-one)", attachments: [attachment] });

    mountSession({});

    expect(getEditorState().content).toBe("![image](/file/attachments/image-one)");
    expect(getEditorState().metadata.attachments).toEqual([attachment]);
  });

  it("starts an edit from the memo and drops any draft under its key", () => {
    cacheService.saveNow(draftKey, { content: "stale draft", attachments: [] });
    const memo = create(MemoSchema, { name: "memos/one", content: "The memo", visibility: Visibility.PUBLIC });

    const state = initialEditorState({ memo, draftKey });

    expect(state.content).toBe("The memo");
    expect(state.ui.isFocusMode).toBe(false);
    expect(cacheService.loadDraft(draftKey).content).toBe("");
  });

  it("recovers older drafts after the server drops their old space metadata", () => {
    const memo = create(MemoSchema, { name: "memos/one", content: "Original", revision: "r1" });
    const oldRevision = JSON.stringify(["Original", "spaces/work", null, null, null, null, [], []]);
    cacheService.saveNow(draftKey, { content: "Edited", attachments: [], baseRevision: oldRevision });

    expect(initialEditorState({ memo, draftKey }).content).toBe("Edited");
    expect(initialEditorState({ memo: { ...memo, content: "Newer content" }, draftKey }).content).toBe("Newer content");
    expect(cacheService.loadDraft(draftKey).content).toBe("Edited");
  });

  it("restores an edit for the same revision, including attachment removals and timestamps", () => {
    const attachment = create(AttachmentSchema, { name: "attachments/original", filename: "original.png" });
    const memo = create(MemoSchema, { name: "memos/one", content: "Original", attachments: [attachment] });
    const createTime = Date.parse("2026-11-21T01:02:03+08:00");
    cacheService.saveNow(draftKey, { content: "Edited", attachments: [], createTime, baseRevision: memoDraftRevision(memo) });

    const state = initialEditorState({ memo, draftKey });

    expect(state.content).toBe("Edited");
    expect(state.metadata.attachments).toEqual([]);
    expect(state.timestamps.createTime).toEqual(new Date(createTime));
  });

  it("keeps the current memo when its content changed without a timestamp change", () => {
    const memo = create(MemoSchema, { name: "memos/one", content: "Original" });
    cacheService.saveNow(draftKey, { content: "Unfinished edit", attachments: [], baseRevision: memoDraftRevision(memo) });

    const state = initialEditorState({ memo: { ...memo, content: "Newer content" }, draftKey });

    expect(state.content).toBe("Newer content");
    expect(cacheService.loadDraft(draftKey).content).toBe("Unfinished edit");
  });

  it("restores a manual calendar time only for the selected Shanghai day", () => {
    const createTime = Date.parse("2026-11-21T01:02:03+08:00");
    cacheService.saveNow(draftKey, { content: "Backdated note", attachments: [], calendarDay: "2026-11-21", createTime });
    const sameDay = new Date("2026-11-21T21:02:03+08:00");
    const nextDay = new Date("2026-11-22T00:00:00+08:00");

    expect(initialEditorState({ draftKey, defaultCreateTime: sameDay }).timestamps.createTime).toEqual(new Date(createTime));
    expect(initialEditorState({ draftKey, defaultCreateTime: nextDay }).timestamps.createTime).toBe(nextDay);
    expect(initialEditorState({ draftKey }).timestamps.createTime).toBeUndefined();
  });
});
