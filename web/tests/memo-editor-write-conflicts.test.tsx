import { create } from "@bufbuild/protobuf";
import { Code, ConnectError } from "@connectrpc/connect";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, renderHook } from "@testing-library/react";
import type { PropsWithChildren } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useAutoSave } from "@/components/MemoEditor/edit/hooks/useAutoSave";
import { useMemoSave } from "@/components/MemoEditor/edit/hooks/useMemoSave";
import { initialEditorState } from "@/components/MemoEditor/edit/initialState";
import { cacheService } from "@/components/MemoEditor/edit/services/cacheService";
import { EditorProvider, type EditorState, useEditorStore } from "@/components/MemoEditor/edit/state";
import { AttachmentSchema } from "@/types/proto/api/attachment_service_pb";
import { type Memo, MemoSchema } from "@/types/proto/api/memo_service_pb";

const clients = vi.hoisted(() => ({ createMemo: vi.fn(), getMemo: vi.fn(), updateMemo: vi.fn() }));
vi.mock("@/connect", () => ({ memoServiceClient: clients }));
vi.mock("@/contexts/NewMemoContext", () => ({ useNewMemo: () => ({ markNewMemo: vi.fn() }) }));
vi.mock("@/lib/toast", () => ({ toast: { add: vi.fn() }, showErrorToast: vi.fn() }));
vi.mock("@/utils/i18n", () => ({ useTranslate: () => (key: string) => key }));

const draftKey = "users/1-new:home";

function mountEditor({
  memo,
  state,
  onEditConflict,
  reviewDraftRevision,
}: {
  memo?: Memo;
  state: EditorState;
  onEditConflict?: () => void;
  reviewDraftRevision?: string;
}) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  const wrapper = ({ children }: PropsWithChildren) => (
    <QueryClientProvider client={queryClient}>
      <EditorProvider initialEditorState={state}>{children}</EditorProvider>
    </QueryClientProvider>
  );
  return renderHook(
    () => {
      const autosave = useAutoSave(draftKey, true, memo, undefined, undefined, reviewDraftRevision);
      const save = useMemoSave({ memoName: memo?.name, onEditConflict, ...autosave });
      return { save, editor: useEditorStore(), ...autosave };
    },
    { wrapper },
  );
}

describe("editor retries and concurrent writes", () => {
  beforeEach(() => {
    cacheService.clearAll();
    localStorage.clear();
    vi.resetAllMocks();
  });

  it("replays the same create request after losing the response and reloading under a different calendar day", async () => {
    const attachment = create(AttachmentSchema, { name: "attachments/a", filename: "a.txt" });
    cacheService.saveNow(draftKey, { content: "note being saved", attachments: [attachment] });
    clients.createMemo.mockRejectedValueOnce(new ConnectError("response lost", Code.Unavailable));
    clients.createMemo.mockImplementationOnce(async ({ memoId, memo }: { memoId: string; memo: Memo }) => ({
      ...memo,
      name: `memos/${memoId}`,
    }));
    const first = mountEditor({ state: initialEditorState({ draftKey, defaultCreateTime: new Date("2026-11-19T00:00:00+08:00") }) });
    await act(() => first.result.current.save());
    const request = clients.createMemo.mock.calls[0][0];
    expect(request).toMatchObject({ idempotent: true, memoId: expect.any(String), memo: { content: "note being saved" } });
    expect(cacheService.loadDraft(draftKey).createAttempt?.id).toBe(request.memoId);
    first.unmount();

    const second = mountEditor({ state: initialEditorState({ draftKey, defaultCreateTime: new Date("2026-11-20T00:00:00+08:00") }) });
    await act(() => second.result.current.save());
    expect(clients.createMemo.mock.calls[1][0]).toEqual(request);
    expect(cacheService.readRaw(draftKey)).toBeNull();
  });

  it("carries the server revision through to the PATCH and keeps the draft on a late conflict", async () => {
    const attachment = create(AttachmentSchema, { name: "attachments/a", filename: "a.txt" });
    const memo = create(MemoSchema, { name: "memos/a", content: "earlier body", revision: "revision-1", attachments: [attachment] });
    clients.getMemo.mockResolvedValue(memo);
    clients.updateMemo.mockRejectedValue(new ConnectError("memo changed", Code.Aborted));
    const onEditConflict = vi.fn();
    const { result } = mountEditor({ memo, state: initialEditorState({ memo, draftKey }), onEditConflict });
    act(() => result.current.editor.dispatch(result.current.editor.actions.updateContent("my unfinished edit")));
    await act(() => result.current.save());

    expect(clients.updateMemo).toHaveBeenCalledWith(expect.objectContaining({ expectedRevision: "revision-1" }));
    expect(onEditConflict).toHaveBeenCalledOnce();
    expect(cacheService.loadDraft(draftKey)).toMatchObject({
      content: "my unfinished edit",
      attachments: [attachment],
      baseRevision: "revision-1",
    });
  });

  it("still saves and reuses the in-memory ID when browser storage is unavailable", async () => {
    cacheService.saveNow(draftKey, { content: "draft", attachments: [] });
    const unavailable = vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
      throw new DOMException("storage unavailable", "QuotaExceededError");
    });
    try {
      clients.createMemo.mockRejectedValue(new ConnectError("retry", Code.Unavailable));
      const editor = mountEditor({ state: initialEditorState({ draftKey }) });
      await act(() => editor.result.current.save());
      await act(() => editor.result.current.save());
      expect(clients.createMemo).toHaveBeenCalledTimes(2);
      expect(clients.createMemo.mock.calls[1][0]).toEqual(clients.createMemo.mock.calls[0][0]);
      editor.unmount();
    } finally {
      unavailable.mockRestore();
    }
  });

  it("keeps the recovered draft's staged-file identity after adopting the newer server revision", () => {
    cacheService.saveNow(draftKey, { content: "older draft", attachments: [], baseRevision: "revision-1" });
    const memo = create(MemoSchema, { name: "memos/a", content: "current body", revision: "revision-2" });
    const { result, unmount } = mountEditor({
      memo,
      reviewDraftRevision: "revision-1",
      state: initialEditorState({ memo, draftKey, reviewStaleDraft: true }),
    });
    expect(result.current.editor.getState().content).toBe("older draft");
    expect(result.current.getBaseRevision()).toBe("revision-2");
    expect(result.current.getFilesRevision()).toBe("revision-1");
    act(() => result.current.editor.dispatch(result.current.editor.actions.updateContent("merged draft")));
    act(() => {
      result.current.flushDraft();
    });
    expect(cacheService.loadDraft(draftKey)).toMatchObject({ baseRevision: "revision-2", stagedFilesRevision: "revision-1" });
    unmount();
    const reopened = mountEditor({ memo, state: initialEditorState({ memo, draftKey }) });
    expect(reopened.result.current.getFilesRevision()).toBe("revision-1");
  });

  it("uses the staged revision from a file-only draft when there is no saved text", () => {
    const memo = create(MemoSchema, { name: "memos/a", content: "current body", revision: "revision-2" });
    const editor = mountEditor({
      memo,
      reviewDraftRevision: "revision-1",
      state: initialEditorState({ memo, draftKey, reviewStaleDraft: true }),
    });
    expect(editor.result.current.editor.getState().content).toBe("current body");
    expect(editor.result.current.getFilesRevision()).toBe("revision-1");
    expect(editor.result.current.getBaseRevision()).toBe("revision-2");
  });

  it("keeps the other tab's draft and does not submit a stale copy", async () => {
    cacheService.saveNow(draftKey, { content: "draft", attachments: [] });
    const editor = mountEditor({ state: initialEditorState({ draftKey }) });
    cacheService.saveNow(draftKey, { content: "newer draft from another tab", attachments: [] });
    await act(() => editor.result.current.save());
    expect(clients.createMemo).not.toHaveBeenCalled();
    expect(cacheService.loadDraft(draftKey).content).toBe("newer draft from another tab");
  });
});
