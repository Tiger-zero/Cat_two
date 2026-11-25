import { Badge, Button } from "@usememos/ui";
import { SmilePlusIcon } from "lucide-react";
import { useNewMemo } from "@/contexts/NewMemoContext";
import type { User } from "@/types/proto/api/user_service_pb";
import { useTranslate } from "@/utils/i18n";
import MemoActionMenu from "../../MemoActionMenu";
import { ReactionSelector } from "../../MemoReactionListView";
import UserAvatar from "../../UserAvatar";
import { useMemoViewContext } from "../MemoViewContext";
import type { MemoHeaderProps } from "../types";

const MemoHeader: React.FC<MemoHeaderProps> = ({ showCreator }) => {
  const t = useTranslate();
  const { memo, creator, currentUser, parentPage, readonly, isEditing, openEditor } = useMemoViewContext();
  const { newMemoName } = useNewMemo();

  if (isEditing) return null;

  const isNew = memo.name === newMemoName;
  return (
    <>
      {((showCreator && creator) || isNew) && (
        <div className="col-start-1 flex min-w-0 items-center gap-2">
          {showCreator && creator && <CreatorDisplay creator={creator} />}
          {isNew && (
            <Badge size="xs" tone="primary" className="shrink-0">
              {t("memo.new-badge")}
            </Badge>
          )}
        </div>
      )}
      <div data-slot="memo-header-actions" className="col-start-2 row-start-1 flex select-none items-center justify-end gap-1">
        {currentUser && (
          // On desktop the picker's trigger shows only while the card is engaged or the picker is open.
          <span className="flex sm:invisible sm:group-hover:visible sm:group-focus-within:visible sm:has-[[data-popup-open]]:visible">
            <ReactionSelector memo={memo} trigger={<Button variant="ghost" size="sm" icon={<SmilePlusIcon />} />} />
          </span>
        )}

        <MemoActionMenu memo={memo} parentPage={parentPage} readonly={readonly} onEdit={openEditor} />
      </div>
    </>
  );
};

export const CreatorDisplay: React.FC<{ creator: User }> = ({ creator }) => (
  <span className="flex min-w-0 shrink items-center gap-1.5 mu-label-sm text-mu-text">
    <UserAvatar className="size-5 shrink-0" avatarUrl={creator.avatarUrl} name={creator.displayName || creator.username} />
    <span className="min-w-0 truncate">{creator.displayName || creator.username}</span>
  </span>
);

export default MemoHeader;
