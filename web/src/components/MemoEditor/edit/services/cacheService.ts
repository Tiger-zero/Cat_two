import { fromJson, type JsonValue, toJson } from "@bufbuild/protobuf";
import type { EditorMode } from "@usememos/editor";
import type { Attachment } from "@/types/proto/api/attachment_service_pb";
import { AttachmentSchema } from "@/types/proto/api/attachment_service_pb";
import type { MemoRelation } from "@/types/proto/api/memo_service_pb";
import { MemoRelationSchema } from "@/types/proto/api/memo_service_pb";
import type { EditorSelection } from "../types/editorController";

// A selection's shape depends on the editor mode it was taken in, so each one keeps its mode.
const cursors = new Map<string, { selection: EditorSelection; mode: EditorMode }>();
const STRUCTURED_CACHE_ENTRY_KIND = "memos.editor-cache";
const STRUCTURED_CACHE_ENTRY_VERSION = 4;

export interface EditorDraft {
  content: string;
  attachments: Attachment[];
  space?: string;
  baseRevision?: string;
  relations?: MemoRelation[];
  attachmentOrder?: string[];
  calendarDay?: string;
  createTime?: number;
  updateTime?: number;
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
      const space = typeof parsed.space === "string" && parsed.space.startsWith("spaces/") ? parsed.space : undefined;
      const draft: EditorDraft = { content: parsed.content, attachments, space };
      if (parsed.version !== STRUCTURED_CACHE_ENTRY_VERSION) return draft;
      if (typeof parsed.baseRevision === "string") draft.baseRevision = parsed.baseRevision;
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
      if (typeof parsed.calendarDay === "string") draft.calendarDay = parsed.calendarDay;
      if (typeof parsed.createTime === "number" && Number.isFinite(new Date(parsed.createTime).valueOf()))
        draft.createTime = parsed.createTime;
      if (typeof parsed.updateTime === "number" && Number.isFinite(new Date(parsed.updateTime).valueOf()))
        draft.updateTime = parsed.updateTime;
      return draft;
    }
  } catch {
    // Drafts have historically been stored as raw markdown strings.
  }

  return { content: raw, attachments: [] };
}

function serializeDraft({ attachments, relations, ...draft }: EditorDraft): string {
  return JSON.stringify({
    kind: STRUCTURED_CACHE_ENTRY_KIND,
    version: STRUCTURED_CACHE_ENTRY_VERSION,
    ...draft,
    attachments: attachments.map((attachment) => toJson(AttachmentSchema, attachment)),
    relations: relations?.map((relation) => toJson(MemoRelationSchema, relation)),
  });
}

// The caller's last observed value prevents an older tab from replacing
// a draft written or discarded elsewhere. Null means no saved entry.
function writeEntry(key: string, draft?: EditorDraft, expected?: string | null): string | null | undefined {
  try {
    if (expected !== undefined && localStorage.getItem(key) !== expected) return undefined;
    if (
      draft &&
      (draft.content.trim() ||
        draft.attachments.length ||
        draft.space ||
        draft.baseRevision ||
        draft.relations?.length ||
        draft.createTime !== undefined ||
        draft.updateTime !== undefined ||
        draft.attachmentOrder?.length)
    ) {
      const raw = serializeDraft(draft);
      localStorage.setItem(key, raw);
      return raw;
    }
    localStorage.removeItem(key);
    return null;
  } catch {
    return undefined;
  }
}

function readRaw(key: string): string | null {
  try {
    return localStorage.getItem(key);
  } catch {
    return null;
  }
}

export const cacheService = {
  /** The storage key for a draft: the user, then the identity-derived suffix (see `draftKeyFor`). */
  key: (username: string, draftKey?: string): string => {
    return `${username}-${draftKey || ""}`;
  },

  saveNow: (key: string, draft: EditorDraft, expected?: string | null) => writeEntry(key, draft, expected),

  readRaw,

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
  },
};
