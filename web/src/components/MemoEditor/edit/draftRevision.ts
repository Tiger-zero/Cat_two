import { getEditorReferenceRelations } from "@/components/MemoMetadata/Relation/relationHelpers";
import { inAppTimeZone } from "@/lib/time-zone";
import type { Memo } from "@/types/proto/api/memo_service_pb";

/** Before server revisions existed, cached drafts used the editable fields as their baseline. */
function legacyMemoDraftRevision(memo: Memo): string {
  return JSON.stringify([
    memo.content,
    null, // Reserved slot in saved browser drafts for the retired Space field.
    memo.createTime?.seconds.toString(),
    memo.createTime?.nanos,
    memo.updateTime?.seconds.toString(),
    memo.updateTime?.nanos,
    memo.attachments.map((attachment) => attachment.name),
    getEditorReferenceRelations(memo.relations, memo.name).map((relation) => relation.relatedMemo?.name),
  ]);
}

export function memoDraftRevision(memo: Memo): string {
  return memo.revision || legacyMemoDraftRevision(memo);
}

/** Allow a draft saved before the server upgrade to retain its original baseline. */
export function matchesMemoDraftRevision(memo: Memo, revision: string | undefined): boolean {
  if (revision === memoDraftRevision(memo)) return true;
  if (!revision?.startsWith("[")) return false;
  try {
    const fields: unknown = JSON.parse(revision);
    if (!Array.isArray(fields) || fields.length !== 8) return false;
    if (fields[1] !== null && typeof fields[1] !== "string") return false;
    fields[1] = null;
    return JSON.stringify(fields) === legacyMemoDraftRevision(memo);
  } catch {
    return false;
  }
}

/** The day a new memo is being composed for, independent of other feed filters. */
export function draftCalendarDay(date: Date | undefined): string | undefined {
  return date && inAppTimeZone(date).format("YYYY-MM-DD");
}
