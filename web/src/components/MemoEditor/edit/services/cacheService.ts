import { fromJson, type JsonValue, toJson } from "@bufbuild/protobuf";
import type { EditorMode } from "@usememos/editor";
import type { Attachment } from "@/types/proto/api/attachment_service_pb";
import { AttachmentSchema } from "@/types/proto/api/attachment_service_pb";
import type { Memo, MemoRelation } from "@/types/proto/api/memo_service_pb";
import { MemoRelationSchema, MemoSchema } from "@/types/proto/api/memo_service_pb";
import type { EditorSelection } from "../types/editorController";
import type { StagedDraftFiles } from "./stagedDraftFiles";

// A selection's shape depends on the editor mode it was taken in, so each one keeps its mode.
const cursors = new Map<string, { selection: EditorSelection; mode: EditorMode }>();
/** A sibling tab owns the shared draft. Keep this tab's branch separate until save or discard. */
const recoveredDrafts = new Map<string, { raw: string; files: StagedDraftFiles; reopened: boolean }>();
const STRUCTURED_CACHE_ENTRY_KIND = "memos.editor-cache";
const STRUCTURED_CACHE_ENTRY_VERSION = 4;

export interface MemoCreateAttempt {
  id: string;
  memo: Memo;
}

export interface EditorDraft {
  content: string;
  attachments: Attachment[];
  baseRevision?: string;
  /** Kept when a recovered draft adopts a new server baseline but its files have the original draft identity. */
  stagedFilesRevision?: string;
  relations?: MemoRelation[];
  attachmentOrder?: string[];
  updateTime?: number;
  createAttempt?: MemoCreateAttempt;
}

function deserializeDraft(raw: string): EditorDraft {
  try {
    const parsed = JSON.parse(raw) as Record<string, unknown>;
    if (parsed.kind === STRUCTURED_CACHE_ENTRY_KIND && parsed.version === 1 && typeof parsed.content === "string") {
      return { content: parsed.content, attachments: [] };
    }
    if (
      parsed.kind === STRUCTURED_CACHE_ENTRY_KIND &&
      (parsed.version === 2 || parsed.version === 3 || parsed.version === STRUCTURED_CACHE_ENTRY_VERSION) &&
      typeof parsed.content === "string"
    ) {
      const attachments = Array.isArray(parsed.attachments)
        ? parsed.attachments.flatMap((value) => {
            try {
              return [fromJson(AttachmentSchema, value as JsonValue, { ignoreUnknownFields: true })];
            } catch {
              return [];
            }
          })
        : [];
      const draft: EditorDraft = { content: parsed.content, attachments };
      if (parsed.version !== STRUCTURED_CACHE_ENTRY_VERSION) return draft;
      if (typeof parsed.baseRevision === "string") draft.baseRevision = parsed.baseRevision;
      if (typeof parsed.stagedFilesRevision === "string") draft.stagedFilesRevision = parsed.stagedFilesRevision;
      if (Array.isArray(parsed.relations)) {
        draft.relations = parsed.relations.flatMap((value) => {
          try {
            return [fromJson(MemoRelationSchema, value as JsonValue, { ignoreUnknownFields: true })];
          } catch {
            return [];
          }
        });
      }
      if (Array.isArray(parsed.attachmentOrder)) {
        draft.attachmentOrder = parsed.attachmentOrder.filter((key): key is string => typeof key === "string");
      }
      if (typeof parsed.updateTime === "number" && Number.isFinite(new Date(parsed.updateTime).valueOf()))
        draft.updateTime = parsed.updateTime;
      const attempt = parsed.createAttempt as { id?: unknown; memo?: JsonValue } | undefined;
      if (attempt?.memo && typeof attempt.id === "string") {
        try {
          draft.createAttempt = {
            id: attempt.id,
            memo: fromJson(MemoSchema, attempt.memo),
          };
        } catch {
          // A malformed submission does not prevent recovering the rest of the draft.
        }
      }
      return draft;
    }
  } catch {
    // Drafts have historically been stored as raw markdown strings.
  }

  return { content: raw, attachments: [] };
}

function serializeDraft({ attachments, relations, createAttempt, ...draft }: EditorDraft): string {
  return JSON.stringify({
    kind: STRUCTURED_CACHE_ENTRY_KIND,
    version: STRUCTURED_CACHE_ENTRY_VERSION,
    ...draft,
    attachments: attachments.map((attachment) => toJson(AttachmentSchema, attachment)),
    relations: relations?.map((relation) => toJson(MemoRelationSchema, relation)),
    createAttempt: createAttempt && { ...createAttempt, memo: toJson(MemoSchema, createAttempt.memo) },
  });
}

export function hasDraftContent(draft: EditorDraft): boolean {
  return Boolean(
    draft.content.trim() ||
      draft.attachments.length ||
      draft.baseRevision ||
      draft.relations?.length ||
      draft.updateTime !== undefined ||
      draft.attachmentOrder?.length ||
      draft.createAttempt,
  );
}

// The caller's last observed value prevents an older tab from replacing
// a draft written or discarded elsewhere. Null means no saved entry.
function storeEntry(
  key: string,
  draft?: EditorDraft,
  expected?: string | null,
): { kind: "saved"; raw: string | null } | { kind: "conflict" | "unavailable" } {
  try {
    if (expected !== undefined && localStorage.getItem(key) !== expected) return { kind: "conflict" };
    if (draft && hasDraftContent(draft)) {
      const raw = serializeDraft(draft);
      localStorage.setItem(key, raw);
      return { kind: "saved", raw };
    }
    localStorage.removeItem(key);
    return { kind: "saved", raw: null };
  } catch {
    return { kind: "unavailable" };
  }
}

function writeEntry(key: string, draft?: EditorDraft, expected?: string | null): string | null | undefined {
  const result = storeEntry(key, draft, expected);
  return result.kind === "saved" ? result.raw : undefined;
}

function readRaw(key: string): string | null {
  try {
    return localStorage.getItem(key);
  } catch {
    return null;
  }
}

function warnAboutRecoveredDraft(event: BeforeUnloadEvent): void {
  event.preventDefault();
  event.returnValue = true;
}

export const cacheService = {
  /** The storage key for a draft: the user, then the identity-derived suffix (see `draftKeyFor`). */
  key: (username: string, draftKey?: string): string => {
    return `${username}-${draftKey || ""}`;
  },

  saveNow: (key: string, draft: EditorDraft, expected?: string | null) => writeEntry(key, draft, expected),
  saveSubmission: (key: string, draft: EditorDraft, expected: string | null) => storeEntry(key, draft, expected),

  readRaw,

  readForEditor(key: string): string | null {
    const recovery = recoveredDrafts.get(key);
    if (!recovery) return readRaw(key);
    recovery.reopened = true;
    return recovery.raw;
  },

  recoveredFiles: (key: string): StagedDraftFiles | undefined => recoveredDrafts.get(key)?.files,

  preserveRecovery(key: string, draft: EditorDraft, files: StagedDraftFiles): void {
    recoveredDrafts.set(key, { raw: serializeDraft(draft), files, reopened: false });
    window.addEventListener("beforeunload", warnAboutRecoveredDraft);
  },

  discardRecovery(key: string): void {
    recoveredDrafts.delete(key);
    if (recoveredDrafts.size === 0) window.removeEventListener("beforeunload", warnAboutRecoveredDraft);
  },

  /** A file write finished after navigation. Keep the snapshot only if it already seeded another editor. */
  discardCommittedRecovery(key: string, files: StagedDraftFiles): void {
    const recovery = recoveredDrafts.get(key);
    if (recovery?.files === files && !recovery.reopened) this.discardRecovery(key);
  },

  load(key: string): string {
    return this.loadDraft(key).content;
  },

  loadDraft(key: string, raw = readRaw(key)): EditorDraft {
    return raw ? deserializeDraft(raw) : { content: "", attachments: [] };
  },

  saveCursor(key: string, selection: EditorSelection, mode: EditorMode): void {
    cursors.set(key, { selection, mode });
  },

  /** The caret saved for `key`, unless it was taken in another editor mode. */
  loadCursor(key: string, mode: EditorMode): EditorSelection | undefined {
    const cursor = cursors.get(key);
    return cursor?.mode === mode ? cursor.selection : undefined;
  },

  clear(key: string, expected?: string | null): string | null | undefined {
    const saved = writeEntry(key, undefined, expected);
    if (saved !== undefined) cursors.delete(key);
    return saved;
  },

  clearAll(): void {
    cursors.clear();
    recoveredDrafts.clear();
    window.removeEventListener("beforeunload", warnAboutRecoveredDraft);
  },
};
