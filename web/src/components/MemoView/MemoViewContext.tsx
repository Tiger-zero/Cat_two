import { createContext, useContext } from "react";
import type { Memo } from "@/types/proto/api/memo_service_pb";
import { MemoRelation_Type } from "@/types/proto/api/memo_service_pb";
import type { User } from "@/types/proto/api/user_service_pb";
import type { PreviewMediaItem } from "@/utils/media-item";

export interface MemoViewContextValue {
  memo: Memo;
  creator: User | undefined;
  currentUser: User | undefined;
  parentPage: string;
  cardWidth: number;
  readonly: boolean;
  showBlurredContent: boolean;
  blurred: boolean;
  /** Whether the card shows the memo's editing session in place of its body. */
  isEditing: boolean;
  openEditor: () => void;
  closeEditor: () => void;
  /** The session reports focus mode so the card's layout can let it take the viewport. */
  onEditorFocusModeChange: (isFocusMode: boolean) => void;
  toggleBlurVisibility: () => void;
  openPreview: (items: string | string[] | PreviewMediaItem[], index?: number) => void;
}

export const MemoViewContext = createContext<MemoViewContextValue | null>(null);

export const useMemoViewContext = (): MemoViewContextValue => {
  const context = useContext(MemoViewContext);
  if (!context) {
    throw new Error("useMemoViewContext must be used within MemoViewContext.Provider");
  }
  return context;
};

export const computeCommentAmount = (memo: Memo): number =>
  memo.relations.filter((r) => r.type === MemoRelation_Type.COMMENT && r.relatedMemo?.name === memo.name).length;
