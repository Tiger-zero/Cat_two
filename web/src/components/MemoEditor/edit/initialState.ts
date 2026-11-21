import type { Memo } from "@/types/proto/api/memo_service_pb";
import { draftCalendarDay, memoDraftRevision } from "./draftRevision";
import { cacheService } from "./services/cacheService";
import { memoService } from "./services/memoService";
import { createInitialState, type EditorState } from "./state/types";

export interface InitialEditorStateOptions {
  /** The memo being edited; cached edits are restored only for this revision. */
  memo?: Memo;
  /** The persisted draft's key, including the current account. */
  draftKey: string;
  defaultCreateTime?: Date;
  rawDraft?: string | null;
}

/**
 * The draft store's first state, computed once when editing starts: the memo's own
 * fields for an edit, otherwise the persisted draft under `draftKey` over the host's
 * defaults.
 */
export function initialEditorState({
  memo,
  draftKey,
  defaultCreateTime,
  rawDraft = cacheService.readRaw(draftKey),
}: InitialEditorStateOptions): EditorState {
  const base = {
    ...createInitialState(),
    ...(memo ? memoService.fromMemo(memo) : undefined),
    calendarDefault: defaultCreateTime,
  };
  const draft = cacheService.loadDraft(draftKey, rawDraft);
  if (memo && draft.baseRevision !== memoDraftRevision(memo)) {
    // Older versions never wrote an existing memo draft.
    if (draft.baseRevision === undefined && rawDraft !== null) cacheService.clear(draftKey, rawDraft);
    return base;
  }
  const restoreTimes = Boolean(memo) || draft.calendarDay === draftCalendarDay(defaultCreateTime);
  return {
    ...base,
    content: draft.content,
    metadata: {
      ...base.metadata,
      attachments: draft.attachments,
      relations: draft.relations ?? base.metadata.relations,
    },
    attachmentOrder: draft.attachmentOrder ?? base.attachmentOrder,
    timestamps: {
      createTime:
        restoreTimes && draft.createTime !== undefined ? new Date(draft.createTime) : (defaultCreateTime ?? base.timestamps.createTime),
      updateTime:
        restoreTimes && draft.updateTime !== undefined ? new Date(draft.updateTime) : (defaultCreateTime ?? base.timestamps.updateTime),
    },
  };
}
