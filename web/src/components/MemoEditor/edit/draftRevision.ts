import { getEditorReferenceRelations } from "@/components/MemoMetadata/Relation/relationHelpers";
import { inAppTimeZone } from "@/lib/time-zone";
import type { Memo } from "@/types/proto/api/memo_service_pb";

/** Before server revisions existed, cached drafts used the editable fields as their baseline. */
function legacyMemoDraftRevision(memo: Memo): string {
  return JSON.stringify([
    memo.content,
    memo.space,
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
  return revision === memoDraftRevision(memo) || (revision?.startsWith("[") === true && revision === legacyMemoDraftRevision(memo));
}

/** The day a new memo is being composed for, independent of other feed filters. */
export function draftCalendarDay(date: Date | undefined): string | undefined {
  return date && inAppTimeZone(date).format("YYYY-MM-DD");
}
