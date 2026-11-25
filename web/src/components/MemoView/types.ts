import type { Memo } from "@/types/proto/api/memo_service_pb";

export interface MemoViewProps {
  memo: Memo;
  compact?: boolean;
  showCreator?: boolean;
  showPinned?: boolean;
  className?: string;
  parentPage?: string;
  shareImageDialogOpen?: boolean;
  onShareImageDialogOpenChange?: (open: boolean) => void;
  /** Controls whether tag-blurred content is revealed; uncontrolled when omitted. */
  showBlurredContent?: boolean;
  onShowBlurredContentChange?: (show: boolean) => void;
}

export interface MemoViewHandle {
  openEditor: () => void;
}

export interface MemoHeaderProps {
  showCreator?: boolean;
}

export interface MemoBodyProps {
  compact?: boolean;
  /** Mirrors the header's: while editing, the session's timestamp row leads with the creator. */
  showCreator?: boolean;
}
