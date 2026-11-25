import type { Memo } from "@/types/proto/api/memo_service_pb";
import { matchesMemoDraftRevision } from "./draftRevision";
import { cacheService } from "./services/cacheService";
import { memoService } from "./services/memoService";
import { createInitialState, type EditorState } from "./state/types";

export interface InitialEditorStateOptions {
  /** The memo being edited; cached edits are restored only for this revision. */
  memo?: Memo;
  /** The persisted draft's key, including the current account. */
  draftKey: string;
  rawDraft?: string | null;
  /** The user chose to review an older draft against the memo now on the server. */
  reviewStaleDraft?: boolean;
}

/**
 * The draft store's first state, computed once when editing starts: the memo's own
 * fields for an edit, otherwise the persisted draft under `draftKey` over the host's
 * defaults.
 */
export function initialEditorState({
  memo,
  draftKey,
  rawDraft = cacheService.readForEditor(draftKey),
  reviewStaleDraft = false,
}: InitialEditorStateOptions): EditorState {
  const base = {
    ...createInitialState(),
    ...(memo ? memoService.fromMemo(memo) : undefined),
  };
  const draft = cacheService.loadDraft(draftKey, rawDraft);
  if (memo && !matchesMemoDraftRevision(memo, draft.baseRevision) && !(reviewStaleDraft && draft.baseRevision)) {
    // Older versions never wrote an existing memo draft.
    if (draft.baseRevision === undefined && rawDraft !== null) cacheService.clear(draftKey, rawDraft);
    return base;
  }
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
      ...base.timestamps,
      updateTime: memo && draft.updateTime !== undefined ? new Date(draft.updateTime) : base.timestamps.updateTime,
    },
  };
}
