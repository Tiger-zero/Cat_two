import { useQueryClient } from "@tanstack/react-query";
import copy from "copy-to-clipboard";
import { useCallback } from "react";
import { useLocation } from "react-router-dom";
import { useInstance } from "@/contexts/InstanceContext";
import { memoKeys, useDeleteMemo, useUpdateMemo } from "@/hooks/useMemoQueries";
import useNavigateTo from "@/hooks/useNavigateTo";
import { userKeys } from "@/hooks/useUserQueries";
import { handleError } from "@/lib/error";
import { recordMemoEditingActivity } from "@/lib/memo-editing-activity";
import { extractMemoIdFromName } from "@/lib/resource-names";
import { showErrorToast, toast } from "@/lib/toast";
import { ROUTES } from "@/router/routes";
import type { Memo } from "@/types/proto/api/memo_service_pb";
import { useTranslate } from "@/utils/i18n";
import { checkAllTasks, uncheckAllTasks } from "@/utils/markdown-task-actions";
import { isMemoDetailPath } from "../MemoView/navigation";

interface UseMemoActionHandlersOptions {
  memo: Memo;
  parentPage?: string;
  onEdit?: () => void;
  setDeleteDialogOpen: (open: boolean) => void;
}

export const useMemoActionHandlers = ({ memo, parentPage, onEdit, setDeleteDialogOpen }: UseMemoActionHandlersOptions) => {
  const t = useTranslate();
  const location = useLocation();
  const navigateTo = useNavigateTo();
  const queryClient = useQueryClient();
  const { profile } = useInstance();
  const { mutateAsync: updateMemo } = useUpdateMemo(memo.name);
  const { mutateAsync: deleteMemo } = useDeleteMemo();
  const isInMemoDetailPage = isMemoDetailPath(location.pathname, memo.name);

  const memoUpdatedCallback = useCallback(() => {
    // Invalidate user stats to trigger refetch
    queryClient.invalidateQueries({ queryKey: userKeys.stats() });
  }, [queryClient]);

  const updateMemoContent = useCallback(
    async (nextContent: string) => {
      if (nextContent === memo.content) {
        return;
      }

      recordMemoEditingActivity();
      try {
        await updateMemo({
          update: {
            name: memo.name,
            content: nextContent,
          },
          updateMask: ["content", "update_time"],
          expectedRevision: memo.revision,
          taskBaseContent: memo.content,
        });
        toast.add({ title: t("memo.task-actions.updated"), type: "success" });
      } catch {
        showErrorToast(t("editor.task-update-failed"));
      }
    },
    [memo.content, memo.name, memo.revision, t, updateMemo],
  );

  const handleTogglePinMemoBtnClick = useCallback(async () => {
    try {
      await updateMemo({
        update: {
          name: memo.name,
          pinned: !memo.pinned,
        },
        updateMask: ["pinned"],
      });
    } catch {
      // do nothing
    }
  }, [memo.name, memo.pinned, updateMemo]);

  const handleEditMemoClick = useCallback(() => {
    onEdit?.();
  }, [onEdit]);

  const handleCopyLink = useCallback(() => {
    let host = profile.instanceUrl;
    if (host === "") {
      host = window.location.origin;
    }
    // A comment's link lands on the comment inside its parent memo, as the comment previews do.
    const path = memo.parent ? `${memo.parent}#${extractMemoIdFromName(memo.name)}` : memo.name;
    copy(`${host}/${path}`);
    toast.add({ title: t("message.succeed-copy-link"), type: "success" });
  }, [memo.name, memo.parent, t, profile.instanceUrl]);

  const handleCopyContent = useCallback(() => {
    copy(memo.content);
    toast.add({ title: t("message.succeed-copy-content"), type: "success" });
  }, [memo.content, t]);

  const handleCheckAllTaskListItemsClick = useCallback(async () => {
    await updateMemoContent(checkAllTasks(memo.content));
  }, [memo.content, updateMemoContent]);

  const handleUncheckAllTaskListItemsClick = useCallback(async () => {
    await updateMemoContent(uncheckAllTasks(memo.content));
  }, [memo.content, updateMemoContent]);

  const handleDeleteMemoClick = useCallback(() => {
    setDeleteDialogOpen(true);
  }, [setDeleteDialogOpen]);

  const confirmDeleteMemo = useCallback(async () => {
    try {
      await deleteMemo(memo.name);
    } catch (error: unknown) {
      handleError(error, showErrorToast, { context: "Delete memo", fallbackMessage: t("common.error-occurred") });
      return;
    }
    toast.add({ title: t("trash.moved"), type: "success" });
    if (memo.parent) {
      queryClient.invalidateQueries({ queryKey: memoKeys.comments(memo.parent) });
      queryClient.invalidateQueries({ queryKey: memoKeys.detail(memo.parent) });
    }
    if (isInMemoDetailPage) {
      navigateTo(parentPage || ROUTES.HOME);
    }
    memoUpdatedCallback();
  }, [memo.name, memo.parent, t, isInMemoDetailPage, parentPage, navigateTo, memoUpdatedCallback, deleteMemo, queryClient]);

  return {
    isInMemoDetailPage,
    handleTogglePinMemoBtnClick,
    handleEditMemoClick,
    handleCopyLink,
    handleCopyContent,
    handleCheckAllTaskListItemsClick,
    handleUncheckAllTaskListItemsClick,
    handleDeleteMemoClick,
    confirmDeleteMemo,
  };
};
