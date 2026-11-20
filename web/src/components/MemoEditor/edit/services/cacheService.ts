import { fromJson, type JsonValue, toJson } from "@bufbuild/protobuf";
import type { EditorMode } from "@usememos/editor";
import type { Attachment } from "@/types/proto/api/attachment_service_pb";
import { AttachmentSchema } from "@/types/proto/api/attachment_service_pb";
import type { EditorSelection } from "../types/editorController";

export const CACHE_DEBOUNCE_DELAY = 500;

const pendingSaves = new Map<string, number>();
// A selection's shape depends on the editor mode it was taken in, so each one keeps its mode.
const cursors = new Map<string, { selection: EditorSelection; mode: EditorMode }>();
const STRUCTURED_CACHE_ENTRY_KIND = "memos.editor-cache";
const STRUCTURED_CACHE_ENTRY_VERSION = 3;

export interface EditorDraft {
  content: string;
  attachments: Attachment[];
  space?: string;
}

function deserializeDraft(raw: string): EditorDraft {
  try {
    const parsed = JSON.parse(raw) as {
      kind?: unknown;
      version?: unknown;
      content?: unknown;
      attachments?: unknown;
      space?: unknown;
    };
    if (parsed.kind === STRUCTURED_CACHE_ENTRY_KIND && parsed.version === 1 && typeof parsed.content === "string") {
      return { content: parsed.content, attachments: [] };
    }
    if (
      parsed.kind === STRUCTURED_CACHE_ENTRY_KIND &&
      (parsed.version === 2 || parsed.version === STRUCTURED_CACHE_ENTRY_VERSION) &&
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
      return { content: parsed.content, attachments, space };
    }
  } catch {
    // Drafts have historically been stored as raw markdown strings.
  }

  return { content: raw, attachments: [] };
}

function serializeDraft({ content, attachments, space }: EditorDraft): string {
  return JSON.stringify({
    kind: STRUCTURED_CACHE_ENTRY_KIND,
    version: STRUCTURED_CACHE_ENTRY_VERSION,
    space,
    content,
    attachments: attachments.map((attachment) => toJson(AttachmentSchema, attachment)),
  });
}

function writeEntry(key: string, draft: EditorDraft): void {
  if (draft.content.trim() || draft.attachments.length > 0 || draft.space) {
    localStorage.setItem(key, serializeDraft(draft));
  } else {
    localStorage.removeItem(key);
  }
}

export const cacheService = {
  /** The storage key for a draft: the user, then the identity-derived suffix (see `draftKeyFor`). */
  key: (username: string, draftKey?: string): string => {
    return `${username}-${draftKey || ""}`;
  },

  save: (key: string, draft: EditorDraft) => {
    const pendingSave = pendingSaves.get(key);
    if (pendingSave) {
      window.clearTimeout(pendingSave);
    }

    const timeoutId = window.setTimeout(() => {
      pendingSaves.delete(key);

      writeEntry(key, draft);
    }, CACHE_DEBOUNCE_DELAY);

    pendingSaves.set(key, timeoutId);
  },

  saveNow: (key: string, draft: EditorDraft) => {
    const pendingSave = pendingSaves.get(key);
    if (pendingSave) {
      window.clearTimeout(pendingSave);
      pendingSaves.delete(key);
    }

    writeEntry(key, draft);
  },

  load(key: string): string {
    const raw = localStorage.getItem(key);
    return raw ? deserializeDraft(raw).content : "";
  },

  loadDraft(key: string): EditorDraft {
    const raw = localStorage.getItem(key);
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

  clear(key: string): void {
    const pendingSave = pendingSaves.get(key);
    if (pendingSave) {
      window.clearTimeout(pendingSave);
      pendingSaves.delete(key);
    }

    localStorage.removeItem(key);
    cursors.delete(key);
  },

  clearAll(): void {
    for (const timeoutId of pendingSaves.values()) {
      window.clearTimeout(timeoutId);
    }
    pendingSaves.clear();
    cursors.clear();
  },
};
