import { equals } from "@bufbuild/protobuf";
import { useCallback, useEffect, useRef } from "react";
import { AttachmentSchema } from "@/types/proto/api/attachment_service_pb";
import { type Memo, MemoRelationSchema } from "@/types/proto/api/memo_service_pb";
import { draftCalendarDay, matchesMemoDraftRevision, memoDraftRevision } from "../draftRevision";
import { cacheService, type EditorDraft, memoService } from "../services";
import type { MemoCreateAttempt } from "../services/cacheService";
import { createInitialState, type EditorState, useEditorStore } from "../state";

function draftFromState(
  { content, metadata, timestamps, calendarDefault, attachmentOrder }: EditorState,
  baseRevision?: string,
): EditorDraft {
  const draft: EditorDraft = { content, attachments: metadata.attachments, space: metadata.space };
  if (baseRevision) draft.baseRevision = baseRevision;
  if (metadata.relations.length || baseRevision) draft.relations = metadata.relations;
  if (attachmentOrder.length) draft.attachmentOrder = attachmentOrder;
  if (timestamps.createTime !== calendarDefault && timestamps.createTime) draft.createTime = timestamps.createTime.valueOf();
  if (timestamps.updateTime !== calendarDefault && timestamps.updateTime) draft.updateTime = timestamps.updateTime.valueOf();
  if (draft.createTime !== undefined || draft.updateTime !== undefined) draft.calendarDay = draftCalendarDay(calendarDefault);
  return draft;
}

const sameDraft = (left: EditorDraft, right: EditorDraft): boolean =>
  left.content === right.content &&
  left.space === right.space &&
  left.baseRevision === right.baseRevision &&
  left.stagedFilesRevision === right.stagedFilesRevision &&
  left.createAttempt === right.createAttempt &&
  left.createTime === right.createTime &&
  left.updateTime === right.updateTime &&
  left.calendarDay === right.calendarDay &&
  (left.attachmentOrder?.length ?? 0) === (right.attachmentOrder?.length ?? 0) &&
  (left.attachmentOrder?.every((key, index) => key === right.attachmentOrder?.[index]) ?? true) &&
  left.attachments.length === right.attachments.length &&
  left.attachments.every((attachment, index) => {
    const other = right.attachments[index];
    return other !== undefined && equals(AttachmentSchema, attachment, other);
  }) &&
  (left.relations?.length ?? 0) === (right.relations?.length ?? 0) &&
  (left.relations?.every((relation, index) => {
    const other = right.relations?.[index];
    return other !== undefined && equals(MemoRelationSchema, relation, other);
  }) ??
    true);

function baselineFor(memo: Memo): EditorDraft {
  return draftFromState({ ...createInitialState(), ...memoService.fromMemo(memo) }, memoDraftRevision(memo));
}

/** Persists document state without rerendering the editor shell on every keystroke. */
export const useAutoSave = (
  key: string,
  enabled = true,
  memo?: Memo,
  onStorageError?: () => void,
  initialDraftRaw?: string | null,
  reviewDraftRevision?: string,
) => {
  const store = useEditorStore();
  const initialDraftRef = useRef<EditorDraft | undefined>(undefined);
  initialDraftRef.current ??= cacheService.loadDraft(key, initialDraftRaw);
  const baselineRef = useRef<EditorDraft | null | undefined>(undefined);
  const fileRevisionRef = useRef<string | undefined>(undefined);
  if (baselineRef.current === undefined) {
    const baseline = memo ? baselineFor(memo) : null;
    const storedRevision = initialDraftRef.current.baseRevision;
    if (memo && baseline && storedRevision) {
      const matches = matchesMemoDraftRevision(memo, storedRevision);
      if (matches) baseline.baseRevision = storedRevision;
      if (matches) fileRevisionRef.current = initialDraftRef.current.stagedFilesRevision ?? storedRevision;
    }
    if (reviewDraftRevision) fileRevisionRef.current = reviewDraftRevision;
    baselineRef.current = baseline;
  }
  const attemptRef = useRef(memo ? undefined : initialDraftRef.current.createAttempt);
  const getFilesRevision = useCallback(() => fileRevisionRef.current ?? baselineRef.current?.baseRevision, []);
  const onDiscard = useRef<(() => void) | undefined>(undefined);
  const onFlush = useRef<(() => boolean) | undefined>(undefined);
  const onRecordAttempt = useRef<((attempt: MemoCreateAttempt) => boolean) | undefined>(undefined);

  useEffect(() => {
    if (!enabled) return;
    const readDraft = () => {
      const baseRevision = baselineRef.current?.baseRevision;
      const filesRevision = getFilesRevision();
      return {
        ...draftFromState(store.getState(), baseRevision),
        stagedFilesRevision: filesRevision !== baseRevision ? filesRevision : undefined,
        createAttempt: attemptRef.current,
      };
    };
    let expected = initialDraftRaw === undefined ? cacheService.readRaw(key) : initialDraftRaw;
    const storedBase = cacheService.loadDraft(key, expected).baseRevision;
    let latest = readDraft();
    let timer: ReturnType<typeof setTimeout> | undefined;
    let hasDraft =
      expected !== null &&
      (Boolean(reviewDraftRevision) || baselineRef.current === null || storedBase === baselineRef.current?.baseRevision);
    let changed = false;
    let discarded = false;
    let storageFailed = false;

    const preventLoss = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = true;
    };
    const reportWrite = (stored: string | null | undefined) => {
      if (stored === undefined) {
        if (!storageFailed) onStorageError?.();
        storageFailed = true;
        window.addEventListener("beforeunload", preventLoss);
        return false;
      }
      expected = stored;
      hasDraft = stored !== null;
      changed = false;
      storageFailed = false;
      window.removeEventListener("beforeunload", preventLoss);
      return true;
    };
    const flushDraft = () => {
      clearTimeout(timer);
      if (!changed || discarded) return !storageFailed;
      const baseline = baselineRef.current;
      if (!baseline || !sameDraft(latest, baseline)) {
        return reportWrite(cacheService.saveNow(key, latest, expected));
      }
      return hasDraft ? reportWrite(cacheService.clear(key, expected)) : true;
    };
    const unsubscribe = store.subscribe(() => {
      const state = store.getState();
      if (attemptRef.current && !state.content.trim() && !state.metadata.attachments.length && !state.localFiles.length) {
        attemptRef.current = undefined;
      }
      const draft = readDraft();
      if (sameDraft(draft, latest)) return;
      latest = draft;
      // An existing memo's editor closes just after the store is reset.
      if (discarded && baselineRef.current) return;
      discarded = false;
      changed = true;
      clearTimeout(timer);
      timer = setTimeout(flushDraft, 500);
    });

    onDiscard.current = () => {
      discarded = true;
      attemptRef.current = undefined;
      clearTimeout(timer);
      if (hasDraft) {
        const removed = cacheService.clear(key, expected);
        if (removed === null) {
          expected = null;
          hasDraft = false;
        }
      }
      window.removeEventListener("beforeunload", preventLoss);
    };
    onFlush.current = flushDraft;
    onRecordAttempt.current = (attempt) => {
      const previous = attemptRef.current;
      attemptRef.current = attempt;
      latest = readDraft();
      discarded = false;
      changed = true;
      clearTimeout(timer);
      const result = cacheService.saveSubmission(key, latest, expected);
      if (result.kind === "saved") return reportWrite(result.raw);
      reportWrite(undefined);
      // Storage failure leaves retries safe in this tab. An observed newer draft belongs to another tab.
      if (result.kind === "unavailable") return true;
      attemptRef.current = previous;
      latest = readDraft();
      return false;
    };
    const handleVisibilityChange = () => {
      if (document.visibilityState === "hidden") flushDraft();
    };
    window.addEventListener("pagehide", flushDraft);
    document.addEventListener("visibilitychange", handleVisibilityChange);

    return () => {
      unsubscribe();
      onDiscard.current = undefined;
      onFlush.current = undefined;
      onRecordAttempt.current = undefined;
      flushDraft();
      window.removeEventListener("beforeunload", preventLoss);
      window.removeEventListener("pagehide", flushDraft);
      document.removeEventListener("visibilitychange", handleVisibilityChange);
    };
  }, [store, key, enabled, onStorageError, initialDraftRaw, reviewDraftRevision, getFilesRevision]);

  const discardDraft = useCallback(() => onDiscard.current?.(), []);
  const flushDraft = useCallback(() => onFlush.current?.() ?? false, []);
  const getCreateAttempt = useCallback(() => attemptRef.current, []);
  const recordCreateAttempt = useCallback((attempt: MemoCreateAttempt) => onRecordAttempt.current?.(attempt) ?? false, []);

  const getBaseRevision = useCallback(() => baselineRef.current?.baseRevision, []);
  const rebaseDraft = useCallback((updated: Memo) => {
    baselineRef.current = baselineFor(updated);
  }, []);

  return { discardDraft, flushDraft, getBaseRevision, getFilesRevision, getCreateAttempt, recordCreateAttempt, rebaseDraft };
};
