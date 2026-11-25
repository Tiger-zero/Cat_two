import copy from "copy-to-clipboard";
import { type FocusEvent, Suspense, useCallback, useEffect, useRef, useState } from "react";
import { DrawingSheet } from "@/components/DrawingSheet";
import { useAuth } from "@/contexts/AuthContext";
import { useSourceMode } from "@/hooks/useSourceMode";
import { recordMemoEditingActivity } from "@/lib/memo-editing-activity";
import { showErrorToast, toast } from "@/lib/toast";
import { cn } from "@/lib/utils";
import { useTranslate } from "@/utils/i18n";

import { AudioRecorderPanel, EditorContent, EditorMetadata, FocusModeExitButton, FocusModeOverlay, TimestampPopover } from "./components";
import { EditorSuggestions } from "./components/EditorSuggestions";
import { COMPOSER_PANEL_CLASSES, FOCUS_MODE_STYLES } from "./constants";
import type { EditorFileOrigin } from "./extensions";
import {
  splitInlineLocalFiles,
  toLocalFiles,
  useAudioRecorder,
  useAutoSave,
  useBlobUrls,
  useDrawing,
  useFocusMode,
  useInlineImageUpload,
  useMemoInit,
  useMemoSave,
} from "./hooks";
import { useStagedDraftFiles } from "./hooks/useStagedDraftFiles";
import { cacheService } from "./services";
import { useEditorContext, useEditorSelector } from "./state";
import { EditorToolbar } from "./Toolbar";
import type { EditSessionProps } from "./types";
import type { EditorController, EditorPosition } from "./types/editorController";

/**
 * An editing session over the draft store MemoEditor provides: the editor with the
 * composer's chrome around it (timestamp, attachments, suggestions, toolbar, focus
 * mode, audio recording), the draft persisted while it is open, and the save that
 * writes the memo through the memo mutations.
 */
export const EditSession: React.FC<EditSessionProps> = ({
  className,
  draftKey,
  initialDraftRaw,
  reviewDraftRevision,
  onReviewDraft,
  frameless,
  expandOnFocus,
  timestampLead,
  memo,
  parentMemoName,
  suggestions = [],
  autoFocus,
  onFocusModeChange,
  placeholder,
  onConfirm,
  onCancel,
}) => {
  const t = useTranslate();
  const { userGeneralSetting } = useAuth();
  const editorRef = useRef<EditorController>(null);
  const { actions, dispatch, getState } = useEditorContext();
  // Subscribe only to the low-frequency slices this component renders from, so
  // typing (which changes content) does not re-render the editor shell and its
  // toolbar/metadata children.
  const isFocusMode = useEditorSelector((s) => s.ui.isFocusMode);
  // Report focus-mode changes so a host can react; inline hosts pass nothing.
  useEffect(() => {
    onFocusModeChange?.(isFocusMode);
  }, [isFocusMode, onFocusModeChange]);
  const [isAudioRecorderOpen, setIsAudioRecorderOpen] = useState(false);
  const [isComposerExpanded, setIsComposerExpanded] = useState(false);
  const collapseFrame = useRef(0);
  const pressedPointer = useRef(false);
  const pendingCollapse = useRef<(() => void) | undefined>(undefined);
  const canExpandComposer = expandOnFocus && !memo && !parentMemoName && !frameless && !isFocusMode;
  const { createBlobUrl } = useBlobUrls();
  const saveMediaMetadata = userGeneralSetting?.saveMediaMetadata ?? false;
  const inlineImageUpload = useInlineImageUpload(editorRef);
  const toDrawingFile = useCallback(
    (file: File) => toLocalFiles([file], { createBlobUrl, saveMediaMetadata })[0],
    [createBlobUrl, saveMediaMetadata],
  );
  const drawing = useDrawing(toDrawingFile);

  const memoName = memo?.name;

  // Source mode only changes how the text is edited, so every composer offers it.
  const [editorMode, setEditorMode] = useSourceMode();
  const isSourceMode = editorMode === "source";
  const sourceMode = { isSourceMode, onToggle: () => setEditorMode(isSourceMode ? "rich" : "source") };

  const { isInitialized } = useMemoInit({ editorRef, draftKey, autoFocus, mode: editorMode });

  const reportDraftStorageError = useCallback(() => showErrorToast(t("editor.draft.storage-failed")), [t]);
  const {
    discardDraft: discardText,
    flushDraft,
    getBaseRevision,
    getFilesRevision,
    getCreateAttempt,
    recordCreateAttempt,
    rebaseDraft,
    preserveDraft,
  } = useAutoSave(draftKey, isInitialized, memo, reportDraftStorageError, initialDraftRaw, reviewDraftRevision);
  const hasEditConflict = useRef(false);
  const staleDraftToast = useRef<string | undefined>(undefined);
  const warnedAboutStaleDraft = useRef(false);
  const notifyStaleDraft = useCallback(
    (filesRevision?: string) => {
      if (warnedAboutStaleDraft.current) return;
      warnedAboutStaleDraft.current = true;
      const stored = cacheService.loadDraft(draftKey, cacheService.readForEditor(draftKey));
      const canCopy = stored.content && stored.baseRevision !== getBaseRevision();
      const reviewRevision = filesRevision ?? stored.stagedFilesRevision ?? stored.baseRevision;
      const actionProps =
        reviewRevision && onReviewDraft
          ? { children: t("editor.draft.review"), onClick: () => onReviewDraft(reviewRevision) }
          : canCopy
            ? { children: t("editor.draft.copy-text"), onClick: () => copy(stored.content) }
            : undefined;
      staleDraftToast.current = toast.add({
        title: t(hasEditConflict.current ? "editor.draft.conflict" : "editor.draft.outdated"),
        type: "warning",
        timeout: 0,
        actionProps,
      });
    },
    [draftKey, getBaseRevision, onReviewDraft, t],
  );
  useEffect(
    () => () => {
      if (staleDraftToast.current) toast.close(staleDraftToast.current);
    },
    [],
  );

  const discardFiles = useStagedDraftFiles({
    key: draftKey,
    enabled: isInitialized,
    getBaseRevision: getFilesRevision,
    createBlobUrl,
    onStorageError: reportDraftStorageError,
    onStaleDraft: notifyStaleDraft,
    preserveDraft,
  });
  const discardDraft = useCallback(async () => {
    await discardFiles();
    discardText();
  }, [discardText, discardFiles]);
  useEffect(() => {
    const baseRevision = getBaseRevision();
    const stored = cacheService.loadDraft(draftKey, cacheService.readForEditor(draftKey));
    if (!reviewDraftRevision && baseRevision && stored.baseRevision && stored.baseRevision !== baseRevision) notifyStaleDraft();
  }, [draftKey, getBaseRevision, notifyStaleDraft, reviewDraftRevision]);

  const { containerRef: editorContainerRef, placeholderHeight, animateFocusMode } = useFocusMode(isFocusMode);
  useEffect(() => {
    if (!canExpandComposer || !isComposerExpanded) return;
    const browser = editorContainerRef.current?.ownerDocument.defaultView;
    if (!browser) return;

    // Resizing before pointerup can move a control below the composer away from the pointer.
    const press = (event: PointerEvent) => {
      if (event.isPrimary) pressedPointer.current = true;
    };
    const release = () => {
      pressedPointer.current = false;
      pendingCollapse.current?.();
      pendingCollapse.current = undefined;
    };
    browser.addEventListener("pointerdown", press, true);
    browser.addEventListener("pointerup", release, true);
    browser.addEventListener("pointercancel", release, true);
    browser.addEventListener("blur", release);
    return () => {
      browser.removeEventListener("pointerdown", press, true);
      browser.removeEventListener("pointerup", release, true);
      browser.removeEventListener("pointercancel", release, true);
      browser.removeEventListener("blur", release);
      pressedPointer.current = false;
      pendingCollapse.current = undefined;
      cancelAnimationFrame(collapseFrame.current);
    };
  }, [canExpandComposer, isComposerExpanded, editorContainerRef]);

  const audioRecorder = useAudioRecorder({
    onRecordingComplete: (localFile) => {
      dispatch(actions.addLocalFile(localFile));
      setIsAudioRecorderOpen(false);
    },
    onRecordingEmpty: () => setIsAudioRecorderOpen(false),
  });

  // Mirror the recorder's busy state into the store so validationService.canSave
  // (consumed here and by EditorToolbar) can block saves mid-recording without
  // the reducer owning the recorder's full state.
  useEffect(() => {
    dispatch(actions.setRecorderBusy(audioRecorder.isBusy));
  }, [audioRecorder.isBusy, actions, dispatch]);

  useEffect(() => {
    if (!isAudioRecorderOpen) {
      return;
    }

    if (audioRecorder.status === "error" || audioRecorder.status === "unsupported") {
      showErrorToast(audioRecorder.error || t("editor.audio-recorder.error-description"));
      setIsAudioRecorderOpen(false);
    }
  }, [isAudioRecorderOpen, audioRecorder.error, audioRecorder.status, t]);

  const rememberCursor = useCallback(() => {
    const cursor = editorRef.current?.getSelection();
    if (cursor !== undefined) {
      cacheService.saveCursor(draftKey, cursor, editorMode);
    }
  }, [draftKey, editorMode]);

  const handleToggleFocusMode = () => {
    animateFocusMode(() => {
      setIsComposerExpanded(false);
      dispatch(actions.toggleFocusMode());
    });
  };

  const handleComposerFocus = (event: FocusEvent<HTMLDivElement>) => {
    pendingCollapse.current = undefined;
    cancelAnimationFrame(collapseFrame.current);
    if (event.target.closest(".memo-editor-root")) setIsComposerExpanded(true);
  };

  const handleComposerBlur = (event: FocusEvent<HTMLDivElement>) => {
    const frame = event.currentTarget;
    if (!isComposerExpanded || frame.contains(event.relatedTarget)) return;

    // Focus can move to a portal, whose React focus event still reaches this frame.
    // Wait for it before deciding whether to move the toolbar back up.
    const collapseAfterFocus = () => {
      cancelAnimationFrame(collapseFrame.current);
      collapseFrame.current = requestAnimationFrame(() => {
        if (!frame.contains(frame.ownerDocument.activeElement)) setIsComposerExpanded(false);
      });
    };
    if (pressedPointer.current) pendingCollapse.current = collapseAfterFocus;
    else collapseAfterFocus();
  };

  const handleCancel = useCallback(async () => {
    if (memoName) {
      if (hasEditConflict.current || reviewDraftRevision) {
        if (!flushDraft()) return;
      } else {
        await discardDraft();
      }
    } else rememberCursor();
    onCancel?.();
  }, [discardDraft, flushDraft, memoName, onCancel, rememberCursor, reviewDraftRevision]);

  const handleEditConflict = useCallback(() => {
    hasEditConflict.current = true;
    notifyStaleDraft();
  }, [notifyStaleDraft]);

  const handleStartAudioRecording = async () => {
    setIsAudioRecorderOpen(true);
    await audioRecorder.startRecording();
  };

  const handleAudioRecorderClick = () => {
    if (audioRecorder.isBusy) {
      return;
    }

    void handleStartAudioRecording();
  };

  /**
   * Single ingest point for files the editor receives. Inline placement writes
   * images into the text (at `position`, else the caret) and attaches the rest;
   * otherwise everything joins the attachment list.
   */
  const handleFiles = useCallback(
    (files: File[], placement: { inline: false } | { inline: true; position?: EditorPosition | null }) => {
      const { saving, loading } = getState().ui.isLoading;
      if (saving || loading) return;
      const localFiles = toLocalFiles(files, { createBlobUrl, saveMediaMetadata });
      const { inline, attachments } = placement.inline ? splitInlineLocalFiles(localFiles) : { inline: [], attachments: localFiles };
      attachments.forEach((file) => dispatch(actions.addLocalFile(file)));
      const inserted = placement.inline && inlineImageUpload.insertLocalImages(inline, placement.position);
      if (!inserted && attachments.length > 0) recordMemoEditingActivity();
    },
    [actions, createBlobUrl, dispatch, getState, inlineImageUpload.insertLocalImages, saveMediaMetadata],
  );

  /** The ＋ menu's Insert image: inline at the caret. */
  const handleInsertImages = useCallback((files: File[]) => handleFiles(files, { inline: true }), [handleFiles]);

  /**
   * A drop inlines images where they landed. A paste carries no placement
   * gesture, so it attaches like the ＋ menu's upload; the attachment list's
   * own Insert action is how a pasted image becomes inline.
   */
  const handleEditorFiles = useCallback(
    (files: File[], origin: EditorFileOrigin) =>
      handleFiles(files, origin.source === "drop" ? { inline: true, position: origin.position } : { inline: false }),
    [handleFiles],
  );

  const handleCancelAudioRecording = () => {
    audioRecorder.resetRecording();
    setIsAudioRecorderOpen(false);
  };

  const handleSave = useMemoSave({
    memoName,
    parentMemoName,
    discardDraft,
    flushDraft,
    getBaseRevision,
    getCreateAttempt,
    recordCreateAttempt,
    onEditConflict: handleEditConflict,
    rebaseDraft,
    onConfirm,
    onCancel: onCancel ? handleCancel : undefined,
  });

  return (
    <>
      <FocusModeOverlay isActive={isFocusMode} onToggle={handleToggleFocusMode} />

      {/*
        Layout structure:
        - Uses justify-between to push content to top and bottom
        - In focus mode: becomes fixed with specific spacing, editor grows to fill space
        - In normal mode: stays relative with max-height constraint
      */}
      {isFocusMode && placeholderHeight > 0 && (
        <div aria-hidden className={cn("w-full", className)} style={{ height: placeholderHeight }} />
      )}

      <div
        ref={editorContainerRef}
        data-expand-on-focus={canExpandComposer || undefined}
        data-composer-expanded={(canExpandComposer && isComposerExpanded) || undefined}
        onFocusCapture={canExpandComposer ? handleComposerFocus : undefined}
        onBlurCapture={canExpandComposer ? handleComposerBlur : undefined}
        className={cn(
          "group flex w-full flex-col items-start justify-between gap-2",
          // A card is the frame of an in-place edit. Focus mode brings its own frame.
          (!frameless || isFocusMode) && cn("relative", COMPOSER_PANEL_CLASSES),
          isFocusMode && cn(FOCUS_MODE_STYLES.container.base, FOCUS_MODE_STYLES.container.spacing),
          !isFocusMode && className,
        )}
      >
        {(memoName || isFocusMode) && (
          <div className={cn("flex w-full items-center gap-1.5", isFocusMode ? "h-10" : "h-6")}>
            {timestampLead}
            {memoName && <TimestampPopover />}
            {isFocusMode && (
              <div className="ms-auto -me-2">
                <FocusModeExitButton onExit={handleToggleFocusMode} />
              </div>
            )}
          </div>
        )}

        {/* Editor content grows to fill available space in focus mode */}
        <EditorContent
          ref={editorRef}
          placeholder={placeholder}
          sourceMode={sourceMode}
          onSubmit={handleSave}
          onFiles={handleEditorFiles}
        />

        {isAudioRecorderOpen && audioRecorder.isBusy && (
          <AudioRecorderPanel
            audioRecorder={{ status: audioRecorder.status, elapsedSeconds: audioRecorder.elapsedSeconds }}
            mediaStream={audioRecorder.recordingStream}
            onStop={() => {
              recordMemoEditingActivity();
              audioRecorder.stopRecording();
            }}
            onCancel={handleCancelAudioRecording}
          />
        )}

        {/* Metadata and toolbar grouped together at bottom */}
        <div className="w-full flex flex-col gap-2">
          <EditorMetadata
            memoName={memoName}
            uploadingLocalFileURLs={inlineImageUpload.uploadingLocalFileURLs}
            onInsertAttachments={inlineImageUpload.insertRemoteImages}
            onInsertLocalFiles={inlineImageUpload.insertLocalImages}
            onEditDrawing={drawing.openExisting}
            openingDrawing={drawing.opening}
          />
          {!memo && !parentMemoName && isInitialized && suggestions.length > 0 && (
            <EditorSuggestions suggestions={suggestions} controllerRef={editorRef} />
          )}
          <EditorToolbar
            editorRef={editorRef}
            onSave={handleSave}
            onCancel={onCancel ? handleCancel : undefined}
            memoName={memoName}
            parentMemoName={parentMemoName}
            onAudioRecorderClick={handleAudioRecorderClick}
            onToggleFocusMode={handleToggleFocusMode}
            sourceMode={sourceMode}
            onInsertImages={handleInsertImages}
            onDrawingClick={drawing.openNew}
          />
        </div>
      </div>

      {drawing.session && (
        <Suspense fallback={null}>
          <DrawingSheet
            key={drawing.session.id}
            mode={drawing.session.mode}
            initialScene={drawing.session.scene}
            background={drawing.session.background}
            onDone={drawing.done}
            onClose={drawing.close}
          />
        </Suspense>
      )}
    </>
  );
};
