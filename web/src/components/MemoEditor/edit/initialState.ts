import type { Memo } from "@/types/proto/api/memo_service_pb";
import { cacheService } from "./services/cacheService";
import { memoService } from "./services/memoService";
import { createInitialState, type EditorState } from "./state/types";

export interface InitialEditorStateOptions {
  /** The memo being edited; its draft is never restored from the cache. */
  memo?: Memo;
  /** The persisted draft's key, for a new memo or comment. */
  draftKey: string;
  initialFocusMode?: boolean;
  defaultCreateTime?: Date;
}

/**
 * The draft store's first state, computed once when editing starts: the memo's own
 * fields for an edit, otherwise the persisted draft under `draftKey` over the host's
 * defaults.
 */
export function initialEditorState({
  memo,
  draftKey,
  initialFocusMode = false,
  defaultCreateTime,
}: InitialEditorStateOptions): EditorState {
  const base = createInitialState(initialFocusMode);
  if (memo) {
    cacheService.clear(draftKey);
    return { ...base, ...memoService.fromMemo(memo) };
  }
  const draft = cacheService.loadDraft(draftKey);
  return {
    ...base,
    content: draft.content,
    metadata: {
      ...base.metadata,
      attachments: draft.attachments.length > 0 ? draft.attachments : base.metadata.attachments,
    },
    timestamps: defaultCreateTime ? { createTime: defaultCreateTime, updateTime: defaultCreateTime } : base.timestamps,
  };
}
