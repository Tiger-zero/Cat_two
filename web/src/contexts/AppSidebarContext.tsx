import { createContext, type ReactNode, useCallback, useContext, useEffect, useMemo, useRef, useState } from "react";
import { useLocation } from "react-router-dom";
import type { MemoParentStatus } from "@/components/MemoParentPlaceholder";
import type { Memo } from "@/types/proto/api/memo_service_pb";

export interface MemoDetailSidebarDescriptor {
  memo: Memo;
  parentMemo?: Memo;
  parentStatus?: MemoParentStatus;
  onParentRetry?: () => void;
  from?: string;
  commentCount?: number;
  /** The memo body is tag-blurred and not yet revealed, so its outline stays hidden. */
  contentHidden?: boolean;
  onEdit?: () => void;
  onCommentsOpen?: () => void;
  onCommentCreate?: () => void;
  onShareImageOpen?: () => void;
}

interface AppSidebarContextValue {
  memoDetail: MemoDetailSidebarDescriptor | undefined;
  setMemoDetail: (descriptor?: MemoDetailSidebarDescriptor) => void;
  mobileOpen: boolean;
  setMobileOpen: (open: boolean) => void;
  closeMobileThen: (action: () => void) => void;
  completeMobileClose: (open: boolean) => void;
  quickFindOpen: boolean;
  setQuickFindOpen: (open: boolean) => void;
  settingsOpen: boolean;
  settingsReturnFocus: React.RefObject<HTMLElement | null>;
  openSettings: (opener: HTMLElement | null) => void;
  closeSettings: () => void;
}

const AppSidebarContext = createContext<AppSidebarContextValue | null>(null);

export const AppSidebarProvider = ({ children }: { children: ReactNode }) => {
  const location = useLocation();
  const [memoDetail, setMemoDetailState] = useState<MemoDetailSidebarDescriptor>();
  const [mobileOpen, setMobileOpen] = useState(false);
  const pendingMobileCloseActionRef = useRef<(() => void) | undefined>(undefined);
  const scheduledMobileCloseActionFrameRef = useRef<number | undefined>(undefined);
  const [quickFindOpen, setQuickFindOpen] = useState(false);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const settingsReturnFocus = useRef<HTMLElement>(null);

  useEffect(() => {
    pendingMobileCloseActionRef.current = undefined;
    if (scheduledMobileCloseActionFrameRef.current !== undefined) {
      window.cancelAnimationFrame(scheduledMobileCloseActionFrameRef.current);
      scheduledMobileCloseActionFrameRef.current = undefined;
    }
    setMobileOpen(false);
    setSettingsOpen(false);

    return () => {
      pendingMobileCloseActionRef.current = undefined;
      if (scheduledMobileCloseActionFrameRef.current !== undefined) {
        window.cancelAnimationFrame(scheduledMobileCloseActionFrameRef.current);
        scheduledMobileCloseActionFrameRef.current = undefined;
      }
    };
  }, [location.pathname]);

  const setMemoDetail = useCallback((descriptor?: MemoDetailSidebarDescriptor) => {
    setMemoDetailState(descriptor);
  }, []);

  const closeMobileThen = useCallback(
    (action: () => void) => {
      if (!mobileOpen) {
        action();
        return;
      }

      pendingMobileCloseActionRef.current = action;
      setMobileOpen(false);
    },
    [mobileOpen],
  );

  const completeMobileClose = useCallback((open: boolean) => {
    if (open) return;
    const action = pendingMobileCloseActionRef.current;
    pendingMobileCloseActionRef.current = undefined;
    if (!action) return;

    // Base UI restores focus in a microtask after the Sheet unmounts. Run the destination
    // action on the next frame so an editor's focus cannot be overwritten by that cleanup.
    scheduledMobileCloseActionFrameRef.current = window.requestAnimationFrame(() => {
      scheduledMobileCloseActionFrameRef.current = undefined;
      action();
    });
  }, []);

  const openSettings = useCallback((opener: HTMLElement | null) => {
    settingsReturnFocus.current = opener;
    setSettingsOpen(true);
  }, []);

  const closeSettings = useCallback(() => setSettingsOpen(false), []);

  const value = useMemo(
    () => ({
      memoDetail,
      setMemoDetail,
      mobileOpen,
      setMobileOpen,
      closeMobileThen,
      completeMobileClose,
      quickFindOpen,
      setQuickFindOpen,
      settingsOpen,
      settingsReturnFocus,
      openSettings,
      closeSettings,
    }),
    [memoDetail, setMemoDetail, mobileOpen, closeMobileThen, completeMobileClose, quickFindOpen, settingsOpen, openSettings, closeSettings],
  );

  return <AppSidebarContext.Provider value={value}>{children}</AppSidebarContext.Provider>;
};

export const useAppSidebar = () => {
  const context = useContext(AppSidebarContext);
  if (!context) {
    throw new Error("useAppSidebar must be used within AppSidebarProvider");
  }
  return context;
};
