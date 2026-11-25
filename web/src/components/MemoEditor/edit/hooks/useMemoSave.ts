import { equals } from "@bufbuild/protobuf";
import { Code, ConnectError } from "@connectrpc/connect";
import { useQueryClient } from "@tanstack/react-query";
import { useCallback } from "react";
import { v4 as uuidv4 } from "uuid";
import { useNewMemo } from "@/contexts/NewMemoContext";
import { attachmentKeys } from "@/hooks/useAttachmentQueries";
import { memoDetailQueryOptions, useCreateMemo, useUpdateMemo } from "@/hooks/useMemoQueries";
import { userKeys } from "@/hooks/useUserQueries";
import { handleError } from "@/lib/error";
import { recordMemoEditingActivity } from "@/lib/memo-editing-activity";
import { showErrorToast, toast } from "@/lib/toast";
import type { Attachment } from "@/types/proto/api/attachment_service_pb";
import { type Memo, MemoSchema } from "@/types/proto/api/memo_service_pb";
import { useTranslate } from "@/utils/i18n";
import { matchesMemoDraftRevision } from "../draftRevision";
import { errorService, memoService, validationService } from "../services";
import type { MemoCreateAttempt } from "../services/cacheService";
import { DrawingNotSavedError, DrawingSavedError, type MemoSaveOperations, mergeUploadedAttachments } from "../services/memoService";
import { useEditorContext } from "../state";
import { renameUploadedKeys } from "../utils/attachmentOrder";

interface UseMemoSaveOptions {
  memoName?: string;
  parentMemoName?: string;
  discardDraft: () => void | Promise<void>;
  flushDraft?: () => boolean;
  getBaseRevision?: () => string | undefined;
  getCreateAttempt?: () => MemoCreateAttempt | undefined;
  recordCreateAttempt?: (attempt: MemoCreateAttempt) => boolean;
  onEditConflict?: () => void;
  rebaseDraft?: (memo: Memo) => void;
  onConfirm?: (memoName: string) => void;
  onCancel?: () => void;
}

class MemoChangedError extends Error {}
class DraftSubmissionStorageError extends Error {}

/**
 * Owns the editor's save transaction and its post-save state. The memo itself is
 * written through the memo mutation hooks, which patch and refresh every query that
 * holds it, so the saved text shows wherever the memo is on the page as soon as the
 * server answers. Saving is identical whether the toolbar or the keyboard triggers it.
 */
export function useMemoSave({
  memoName,
  parentMemoName,
  discardDraft,
  flushDraft,
  getBaseRevision,
  getCreateAttempt,
  recordCreateAttempt,
  onEditConflict,
  rebaseDraft,
  onConfirm,
  onCancel,
}: UseMemoSaveOptions): () => Promise<void> {
  const t = useTranslate();
  const queryClient = useQueryClient();
  const { mutateAsync: createMemo } = useCreateMemo();
  const { mutateAsync: updateMemo } = useUpdateMemo(memoName);
  const { markNewMemo } = useNewMemo();
  const { actions, dispatch, getState } = useEditorContext();

  return useCallback(async () => {
    const state = getState();
    if (state.ui.isLoading.saving) return;
    const { valid, reason, detail } = validationService.canSave(state);
    if (!valid) {
      showErrorToast(reason ? t(reason, detail ? { url: detail } : undefined) : t("editor.validation.cannot-save"));
      return;
    }

    recordMemoEditingActivity();
    dispatch(actions.setLoading("saving", true));

    // Files that finished uploading, keyed by preview URL. If the save fails
    // after they uploaded, they move onto the draft as attachments so that a
    // retry does not upload them again (a refused create would otherwise leak
    // one copy per attempt into the user's storage).
    const uploaded = new Map<string, Attachment>();
    const keepUploadedFiles = () => {
      if (uploaded.size === 0) return;
      const current = getState();
      const moved = current.localFiles.filter((file) => uploaded.has(file.previewUrl));
      if (moved.length === 0) return;
      dispatch(
        actions.setMetadata({
          attachments: mergeUploadedAttachments(
            current.metadata.attachments,
            moved.map((file) => uploaded.get(file.previewUrl) as Attachment),
          ),
        }),
      );
      dispatch(actions.setLocalFiles(current.localFiles.filter((file) => !uploaded.has(file.previewUrl))));
      dispatch(actions.setAttachmentOrder(renameUploadedKeys(current.attachmentOrder, uploaded)));
    };

    const pendingCreate = !memoName ? getCreateAttempt?.() : undefined;
    const createID = !memoName ? (pendingCreate?.id ?? uuidv4()) : undefined;
    const operations: MemoSaveOperations = {
      // The server's current copy, fetched afresh into the detail cache.
      getMemo: async (name) => {
        const memo = await queryClient.fetchQuery({ ...memoDetailQueryOptions(name), staleTime: 0 });
        if (!memo) throw new ConnectError("memo not found", Code.NotFound);
        const baseRevision = getBaseRevision?.();
        if (baseRevision && !matchesMemoDraftRevision(memo, baseRevision)) throw new MemoChangedError();
        return memo;
      },
      updateMemo: async (update, updateMask, expectedRevision) => {
        const updated = await updateMemo({ update, updateMask, expectedRevision });
        rebaseDraft?.(updated);
        return updated;
      },
      createMemo: async (memo, parent) => {
        // Write ahead of the RPC: a timeout or a tab reload cannot tell whether the server committed.
        const previous = pendingCreate?.memo;
        if (previous && equals(MemoSchema, { ...previous, createTime: undefined, updateTime: undefined }, memo)) {
          try {
            return await createMemo({ memo, parent, id: createID });
          } catch (error) {
            // An older client may already have committed this ID with explicit times.
            // Only replay its original fingerprint once the server confirms the ID exists.
            if ((!previous.createTime && !previous.updateTime) || !(error instanceof ConnectError && error.code === Code.AlreadyExists)) {
              throw error;
            }
            return createMemo({ memo: previous, parent, id: createID });
          }
        }
        if (createID && recordCreateAttempt && !recordCreateAttempt({ id: createID, memo })) {
          throw new DraftSubmissionStorageError();
        }
        return createMemo({ memo, parent, id: createID });
      },
    };

    try {
      const result = await memoService.save(
        state,
        {
          memoName,
          parentMemoName,
          onFileUploaded: (file, attachment) => {
            uploaded.set(file.previewUrl, attachment);
            if (file.replaces) dispatch(actions.setDrawingMemoUpdate(true));
            keepUploadedFiles();
          },
        },
        operations,
      );

      if (!result.hasChanges) {
        showErrorToast(t("editor.no-changes-detected"));
        onCancel?.();
        return;
      }

      // Prevent the autosave unmount flush from restoring the saved draft.
      await discardDraft();

      dispatch(actions.reset());
      if (!memoName && !parentMemoName) {
        markNewMemo(result.memoName);
      } else if (memoName) {
        markNewMemo((currentName) => (currentName === memoName ? null : currentName));
      }
      onConfirm?.(result.memoName);
    } catch (error) {
      keepUploadedFiles();
      flushDraft?.();
      if (uploaded.size > 0) {
        // The uploads landed even though the save did not.
        void queryClient.invalidateQueries({ queryKey: attachmentKeys.lists() });
        void queryClient.invalidateQueries({ queryKey: userKeys.stats() });
      }
      if (error instanceof MemoChangedError || (error instanceof ConnectError && error.code === Code.Aborted)) {
        if (memoName && onEditConflict) onEditConflict();
        else showErrorToast(t("editor.draft.conflict"));
        return;
      }
      if (error instanceof DraftSubmissionStorageError) {
        showErrorToast(t("editor.draft.submission-storage-failed"));
        return;
      }
      if (createID && error instanceof ConnectError && error.code === Code.AlreadyExists) {
        const memoPath = `/memos/${encodeURIComponent(createID)}`;
        toast.add({
          title: t("editor.draft.create-conflict"),
          type: "warning",
          timeout: 0,
          actionProps: {
            children: t("editor.draft.open-saved-memo"),
            onClick: () => window.open(memoPath, "_blank", "noopener,noreferrer"),
          },
        });
        return;
      }
      if (error instanceof DrawingSavedError) {
        dispatch(actions.setDrawingSaveError("editor.drawing.saved-needs-update"));
        showErrorToast(t("editor.drawing.saved-needs-update"));
        return;
      }
      if (error instanceof DrawingNotSavedError) {
        dispatch(actions.setDrawingSaveError("editor.drawing.saved-without-drawing"));
        // The rest of the memo is saved; only the drawing waits for another save.
        console.error(error);
        showErrorToast(t("editor.drawing.saved-without-drawing"));
        return;
      }
      handleError(error, showErrorToast, {
        context: "Failed to save memo",
        fallbackMessage: errorService.getErrorMessage(error),
      });
    } finally {
      dispatch(actions.setLoading("saving", false));
    }
  }, [
    actions,
    createMemo,
    discardDraft,
    dispatch,
    flushDraft,
    getState,
    getBaseRevision,
    getCreateAttempt,
    markNewMemo,
    memoName,
    onCancel,
    onConfirm,
    onEditConflict,
    parentMemoName,
    queryClient,
    rebaseDraft,
    recordCreateAttempt,
    t,
    updateMemo,
  ]);
}
