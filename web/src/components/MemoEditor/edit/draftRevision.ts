import { getEditorReferenceRelations } from "@/components/MemoMetadata/Relation/relationHelpers";
import { inAppTimeZone } from "@/lib/time-zone";
import type { Memo } from "@/types/proto/api/memo_service_pb";

/** The editable fields identify a revision even if two updates share one timestamp. */
export function memoDraftRevision(memo: Memo): string {
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

/** The day a new memo is being composed for, independent of other feed filters. */
export function draftCalendarDay(date: Date | undefined): string | undefined {
  return date && inAppTimeZone(date).format("YYYY-MM-DD");
}
