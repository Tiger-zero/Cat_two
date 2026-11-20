import { useMemo } from "react";
import { useNearViewport } from "@/hooks/useNearViewport";
import type { Attachment } from "@/types/proto/api/attachment_service_pb";
import type { MemoRelation } from "@/types/proto/api/memo_service_pb";
import { AttachmentRows } from "./Attachment/AttachmentListView";
import { METADATA_ROW_LIST_CLASSES } from "./MetadataSection";
import RelationRows, { getRelationRowItems } from "./Relation/RelationRows";

interface MemoMetadataRowsProps {
  audio: Attachment[];
  docs: Attachment[];
  relations: MemoRelation[];
  currentMemoName: string;
  parentPage?: string;
}

/**
 * Everything attached to a memo that is not its content or media, as one list of 28px
 * rows 2px apart: audio, files, and related memos. Renders nothing when
 * the memo has none of them, so the body never carries an empty list.
 */
const MemoMetadataRows = ({ audio, docs, relations, currentMemoName, parentPage }: MemoMetadataRowsProps) => {
  const { ref: viewportRef, isNearViewport } = useNearViewport<HTMLDivElement>();
  const relationItems = useMemo(() => getRelationRowItems(relations, currentMemoName), [relations, currentMemoName]);

  if (audio.length === 0 && docs.length === 0 && relationItems.length === 0) {
    return null;
  }

  return (
    <div ref={viewportRef} className={METADATA_ROW_LIST_CLASSES}>
      <AttachmentRows audio={audio} docs={docs} />
      {relationItems.length > 0 && <RelationRows items={relationItems} parentPage={parentPage} enabled={isNearViewport} />}
    </div>
  );
};

export default MemoMetadataRows;
