import copy from "copy-to-clipboard";
import { Suspense, useCallback, useEffect, useRef, useState } from "react";
import { DrawingSheet } from "@/components/DrawingSheet";
import { useAuth } from "@/contexts/AuthContext";
import { useSourceMode } from "@/hooks/useSourceMode";
import { showErrorToast, toast } from "@/lib/toast";
import { cn } from "@/lib/utils";
import { useTranslate } from "@/utils/i18n";

import { AudioRecorderPanel, EditorContent, EditorMetadata, FocusModeExitButton, FocusModeOverlay, TimestampPopover } from "./components";
import { EditorSuggestions } from "./components/EditorSuggestions";
import { COMPOSER_PANEL_CLASSES, FOCUS_MODE_STYLES } from "./constants";
import { draftCalendarDay } from "./draftRevision";
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
import type { EditorViewToggles, EditSessionProps } from "./types";
import type { EditorController, EditorPosition } from "./types/editorController";

/**
 * An editing session over the draft store MemoEditor provides: the editor with the
 * composer's chrome around it (timestamp, attachments, suggestions, toolbar, focus
 * mode, audio recording), the draft persisted while it is open, and the save that
 * writes the memo through the memo mutations. A host that presents the editor
 * full-screen supplies `onFocusModeExit`; its presence is what makes an instance
 * hosted, so focus mode starts on and stays on.
 */
export const EditSession: React.FC<EditSessionProps> = ({
  className,
  draftKey,
  initialDraftRaw,
  frameless,
  timestampLead,
  memo,
  parentMemoName,
  suggestions = [],
  autoFocus,
  onFocusModeExit,
  onFocusModeChange,
  placeholder,
  defaultCreateTime,
  onConfirm,
  onCancel,
  onSavingChange,
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
  const isSaving = useEditorSelector((s) => s.ui.isLoading.saving);
  const hasTimestamp = useEditorSelector((s) => Boolean(s.timestamps.createTime));
  const [isAudioRecorderOpen, setIsAudioRecorderOpen] = useState(false);
  const { createBlobUrl } = useBlobUrls();
  const saveMediaMetadata = userGeneralSetting?.saveMediaMetadata ?? false;
  const inlineImageUpload = useInlineImageUpload(editorRef);
  const toDrawingFile = useCallback(
    (file: File) => toLocalFiles([file], { createBlobUrl, saveMediaMetadata })[0],
    [createBlobUrl, saveMediaMetadata],
  );
  const drawing = useDrawing(toDrawingFile);

  const memoName = memo?.name;
  const showTimestamp = Boolean(memoName || (!memo && hasTimestamp));

  // Source mode only changes how the text is edited, so every composer offers it.
  const [editorMode, setEditorMode] = useSourceMode();
  const isSourceMode = editorMode === "source";
  const sourceMode = { isSourceMode, onToggle: () => setEditorMode(isSourceMode ? "rich" : "source") };

  const { isInitialized } = useMemoInit({ editorRef, draftKey, autoFocus, mode: editorMode });
  useEffect(() => {
    onSavingChange?.(isSaving);
  }, [isSaving, onSavingChange]);

  const reportDraftStorageError = useCallback(() => showErrorToast(t("editor.draft.storage-failed")), [t]);
  const {
    discardDraft: discardText,
    getBaseRevision,
    rebaseDraft,
  } = useAutoSave(draftKey, isInitialized, memo, reportDraftStorageError, initialDraftRaw);
  const warnedAboutStaleDraft = useRef(false);
  const notifyStaleDraft = useCallback(() => {
    if (warnedAboutStaleDraft.current) return;
    warnedAboutStaleDraft.current = true;
    const stored = cacheService.loadDraft(draftKey);
    const canCopy = stored.content && stored.baseRevision !== getBaseRevision();
    toast.add({
      title: t("editor.draft.outdated"),
      type: "warning",
      timeout: 0,
      actionProps: canCopy ? { children: t("editor.draft.copy-text"), onClick: () => copy(stored.content) } : undefined,
    });
  }, [draftKey, getBaseRevision, t]);

  const discardFiles = useStagedDraftFiles({
    key: draftKey,
    enabled: isInitialized,
    getBaseRevision,
    createBlobUrl,
    onStorageError: reportDraftStorageError,
    onStaleDraft: notifyStaleDraft,
  });
  const discardDraft = useCallback(async () => {
    discardText();
    await discardFiles();
  }, [discardText, discardFiles]);
  useEffect(() => {
    const baseRevision = getBaseRevision();
    const stored = cacheService.loadDraft(draftKey);
    if (baseRevision && stored.baseRevision && stored.baseRevision !== baseRevision) notifyStaleDraft();
  }, [draftKey, getBaseRevision, notifyStaleDraft]);

  const { containerRef: editorContainerRef, placeholderHeight, animateFocusMode } = useFocusMode(isFocusMode);

  // Initial state restores manual times for the selected day. Only an actual
  // day change, including clearing the filter, resets the calendar defaults.
  useEffect(() => {
    if (memo || !isInitialized || draftCalendarDay(getState().calendarDefault) === draftCalendarDay(defaultCreateTime)) return;
    dispatch(actions.setCalendarDefault(defaultCreateTime));
  }, [defaultCreateTime, memo, isInitialized, actions, dispatch, getState]);

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

  // Hosted: focus mode is the host's frame, so leaving it dismisses the host.
  // Inline: focus mode is a view this editor owns and toggles in place.
  const handleToggleFocusMode = () => {
    if (onFocusModeExit) {
      rememberCursor();
      onFocusModeExit();
      return;
    }
    animateFocusMode(() => dispatch(actions.toggleFocusMode()));
  };

  const handleCancel = useCallback(async () => {
    if (memoName) await discardDraft();
    else rememberCursor();
    onCancel?.();
  }, [discardDraft, memoName, onCancel, rememberCursor]);

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
      if (placement.inline) inlineImageUpload.insertLocalImages(inline, placement.position);
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

  // The ＋ menu's view toggle only describes how an inline editor presents
  // itself, so a hosted editor does not offer it: its host owns the frame.
  const viewToggles: EditorViewToggles | undefined = onFocusModeExit
    ? undefined
    : {
        onToggleFocusMode: handleToggleFocusMode,
      };

  const handleSave = useMemoSave({
    memoName,
    parentMemoName,
    defaultCreateTime,
    discardDraft,
    getBaseRevision,
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
        className={cn(
          "group w-full flex flex-col justify-between items-start gap-2",
          // A card that is the frame already supplies the background, border and padding; a session floating over the page in
          // focus mode always brings its own. The bottom is 16px like the sides: the toolbar's
          // hard edge needs more room than the text's soft top.
          (!frameless || isFocusMode) && cn("relative", COMPOSER_PANEL_CLASSES),
          isFocusMode && cn(FOCUS_MODE_STYLES.container.base, FOCUS_MODE_STYLES.container.spacing),
          !isFocusMode && className,
        )}
      >
        {/* Focus mode always shows the memo's time ("Now" before it has one) and adds
            the button that leaves the current frame — minimize inline, close when hosted. */}
        {(showTimestamp || isFocusMode) && (
          <div className={cn("flex w-full items-center gap-1.5", isFocusMode ? "h-10" : "h-6")}>
            {timestampLead}
            <TimestampPopover />
            {isFocusMode && (
              <div className="ms-auto -me-2">
                <FocusModeExitButton action={onFocusModeExit ? "close" : "minimize"} onExit={handleToggleFocusMode} />
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
            onStop={audioRecorder.stopRecording}
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
            viewToggles={viewToggles}
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
