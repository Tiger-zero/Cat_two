import { Button, Menu, Spinner, Tooltip } from "@usememos/ui";
import { uniqBy } from "lodash-es";
import { CodeXmlIcon, ImageIcon, LinkIcon, Maximize2Icon, MicIcon, PaletteIcon, PaperclipIcon, PlusIcon } from "lucide-react";
import { useCallback, useRef, useState } from "react";
import { LinkMemoDialog } from "@/components/MemoMetadata";
import type { MemoRelation } from "@/types/proto/api/memo_service_pb";
import { useTranslate } from "@/utils/i18n";
import { useFileUpload, useLinkMemo } from "../hooks";
import { useEditorContext, useEditorSelector } from "../state";
import type { InsertMenuProps } from "../types";
import type { LocalFile } from "../types/attachment";

const InsertMenu = (props: InsertMenuProps) => {
  const t = useTranslate();
  const { actions, dispatch, getState } = useEditorContext();
  const relations = useEditorSelector((s) => s.metadata.relations);
  const { viewToggles, sourceMode, isUploading: isUploadingProp } = props;

  const [linkDialogOpen, setLinkDialogOpen] = useState(false);
  const inlineImageInputRef = useRef<HTMLInputElement>(null);

  const { fileInputRef, selectingFlag, handleFileInputChange, handleUploadClick } = useFileUpload((newFiles: LocalFile[]) => {
    if (getState().ui.isLoading.saving) return;
    newFiles.forEach((file) => dispatch(actions.addLocalFile(file)));
  });

  const linkMemo = useLinkMemo({
    isOpen: linkDialogOpen,
    currentMemoName: props.memoName,
    existingRelations: relations,
    onAddRelation: (relation: MemoRelation) => {
      dispatch(actions.setMetadata({ relations: uniqBy([...relations, relation], (r) => r.relatedMemo?.name) }));
      setLinkDialogOpen(false);
    },
  });

  const isUploading = selectingFlag || isUploadingProp;
  const insertionDisabled = isUploading || props.isSaving;

  const handleOpenLinkDialog = useCallback(() => {
    setLinkDialogOpen(true);
  }, []);

  const handleAttachmentUploadClick = useCallback(() => {
    if (getState().ui.isLoading.saving) return;
    handleUploadClick();
  }, [getState, handleUploadClick]);

  const handleInlineImageUploadClick = useCallback(() => {
    if (getState().ui.isLoading.saving) return;
    inlineImageInputRef.current?.click();
  }, [getState]);

  const handleInlineImageInputChange = useCallback(
    (event: React.ChangeEvent<HTMLInputElement>) => {
      const files = Array.from(event.target.files ?? []);
      if (files.length > 0) props.onInsertImages(files);
      event.target.value = "";
    },
    [props.onInsertImages],
  );

  // Insert actions (add content).
  const insertItems = [
    { key: "attachment", label: t("editor.insert-menu.add-attachment"), icon: PaperclipIcon, onClick: handleAttachmentUploadClick },
    { key: "inline-image", label: t("editor.insert-menu.insert-image"), icon: ImageIcon, onClick: handleInlineImageUploadClick },
    ...(props.onDrawingClick
      ? [{ key: "drawing", label: t("editor.insert-menu.drawing"), icon: PaletteIcon, onClick: props.onDrawingClick }]
      : []),
    { key: "audio", label: t("editor.audio-recorder.trigger"), icon: MicIcon, onClick: props.onAudioRecorderClick },
    { key: "link", label: t("editor.insert-menu.link-memo"), icon: LinkIcon, onClick: handleOpenLinkDialog },
  ];

  return (
    <>
      <Menu.Root modal={false}>
        <Tooltip.Root>
          <Tooltip.Trigger
            render={
              <Menu.Trigger
                render={
                  <Button
                    variant="neutral"
                    size="md"
                    disabled={insertionDisabled}
                    aria-label={t("common.add")}
                    icon={isUploading ? <Spinner label={t("editor.blocks.loading")} /> : <PlusIcon />}
                  />
                }
              />
            }
          />
          <Tooltip.Popup>{t("common.add")}</Tooltip.Popup>
        </Tooltip.Root>
        <Menu.Popup align="start">
          {insertItems.map((item) => (
            <Menu.Item key={item.key} onClick={item.onClick} disabled={props.isSaving} icon={<item.icon />}>
              {item.label}
            </Menu.Item>
          ))}
          {/* How the composer presents itself. Focus mode is absent when a host owns the
              editor's presentation; Markdown mode (the editor's source mode) only changes how
              the text is edited, so every composer has it. Checked while on.
              The code glyph says this mode shows the markup. */}
          <Menu.Separator />
          {viewToggles && (
            <Menu.Item onClick={viewToggles.onToggleFocusMode} icon={<Maximize2Icon />}>
              {t("editor.focus-mode")}
            </Menu.Item>
          )}
          <Menu.CheckboxItem checked={sourceMode.isSourceMode} onCheckedChange={sourceMode.onToggle} closeOnClick icon={<CodeXmlIcon />}>
            {t("editor.markdown-mode")}
          </Menu.CheckboxItem>
        </Menu.Popup>
      </Menu.Root>

      {/* Hidden file input */}
      <input
        className="hidden"
        ref={fileInputRef}
        disabled={insertionDisabled}
        onChange={handleFileInputChange}
        type="file"
        multiple={true}
        accept=""
      />

      <input
        className="hidden"
        ref={inlineImageInputRef}
        disabled={insertionDisabled}
        onChange={handleInlineImageInputChange}
        type="file"
        multiple={true}
        accept="image/*"
      />

      <LinkMemoDialog
        open={linkDialogOpen}
        onOpenChange={setLinkDialogOpen}
        searchText={linkMemo.searchText}
        onSearchChange={linkMemo.setSearchText}
        filteredMemos={linkMemo.filteredMemos}
        isFetching={linkMemo.isFetching}
        onSelectMemo={linkMemo.addMemoRelation}
        isAlreadyLinked={linkMemo.isAlreadyLinked}
      />
    </>
  );
};

export default InsertMenu;
