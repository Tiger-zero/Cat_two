import { Menu as MenuPrimitive } from "@base-ui/react/menu";
import { Button, Menu, menuClasses, Tooltip } from "@usememos/ui";
import {
  ArrowUpRightIcon,
  BookmarkMinusIcon,
  BookmarkPlusIcon,
  CheckCheckIcon,
  CopyIcon,
  Edit3Icon,
  FileTextIcon,
  LinkIcon,
  ListRestartIcon,
  MoreVerticalIcon,
  TrashIcon,
} from "lucide-react";
import { useState } from "react";
import { Link } from "react-router-dom";
import ConfirmDialog from "@/components/ConfirmDialog";
import { useTranslate } from "@/utils/i18n";
import { createMemoNavigationState } from "../MemoView/navigation";
import { useMemoActionHandlers } from "./hooks";
import type { MemoActionMenuProps } from "./types";

/** The open link is a real link, so its row takes the package's menu row classes. */
const MENU_ROW_CLASSES = menuClasses();

const MemoActionMenu = (props: MemoActionMenuProps) => {
  const { memo, parentPage, readonly } = props;
  const t = useTranslate();

  // Dialog state
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);

  // Derived state
  const isComment = Boolean(memo.parent);
  const hasTaskList = Boolean(memo.property?.hasTaskList);
  const hasOpenTasks = Boolean(memo.property?.hasIncompleteTasks);

  // Action handlers
  const {
    isInMemoDetailPage,
    handleTogglePinMemoBtnClick,
    handleEditMemoClick,
    handleCopyLink,
    handleCopyContent,
    handleCheckAllTaskListItemsClick,
    handleUncheckAllTaskListItemsClick,
    handleDeleteMemoClick,
    confirmDeleteMemo,
  } = useMemoActionHandlers({
    memo,
    parentPage,
    onEdit: props.onEdit,
    setDeleteDialogOpen,
  });

  // A comment opens in the context of its parent memo, never on its own.
  const openItem = !isComment && !isInMemoDetailPage && (
    <MenuPrimitive.LinkItem
      closeOnClick
      className={MENU_ROW_CLASSES.item}
      render={
        <Link to={`/${memo.name}`} state={parentPage !== undefined ? createMemoNavigationState(parentPage) : undefined} viewTransition />
      }
    >
      <ArrowUpRightIcon className="mu-icon" />
      <span className={MENU_ROW_CLASSES.label}>{t("common.open")}</span>
    </MenuPrimitive.LinkItem>
  );

  const copySubmenu = (
    <Menu.SubmenuRoot>
      <Menu.SubmenuTrigger icon={<CopyIcon />}>{t("common.copy")}</Menu.SubmenuTrigger>
      <Menu.Popup>
        <Menu.Item onClick={handleCopyLink} icon={<LinkIcon />}>
          {t("memo.copy-link")}
        </Menu.Item>
        <Menu.Item onClick={handleCopyContent} icon={<FileTextIcon />}>
          {t("memo.copy-content")}
        </Menu.Item>
      </Menu.Popup>
    </Menu.SubmenuRoot>
  );

  const deleteItem = (
    <Menu.Item onClick={handleDeleteMemoClick} icon={<TrashIcon />} danger>
      {t("common.delete")}
    </Menu.Item>
  );

  const renderItems = () => {
    if (readonly) {
      return (
        <>
          {openItem}
          {copySubmenu}
        </>
      );
    }

    return (
      <>
        <Menu.Item onClick={handleEditMemoClick} icon={<Edit3Icon />}>
          {t("common.edit")}
        </Menu.Item>
        {/* One task action at a time: finish the open tasks, or reset a finished list. */}
        {hasTaskList &&
          (hasOpenTasks ? (
            <Menu.Item onClick={handleCheckAllTaskListItemsClick} icon={<CheckCheckIcon />}>
              {t("memo.task-actions.check-all")}
            </Menu.Item>
          ) : (
            <Menu.Item onClick={handleUncheckAllTaskListItemsClick} icon={<ListRestartIcon />}>
              {t("memo.task-actions.uncheck-all")}
            </Menu.Item>
          ))}

        {/* A comment has no place of its own to pin; it lives under its parent. */}
        {!isComment && (
          <>
            <Menu.Separator />
            <Menu.Item onClick={handleTogglePinMemoBtnClick} icon={memo.pinned ? <BookmarkMinusIcon /> : <BookmarkPlusIcon />}>
              {memo.pinned ? t("common.unpin") : t("common.pin")}
            </Menu.Item>
          </>
        )}

        <Menu.Separator />
        {openItem}
        {copySubmenu}

        <Menu.Separator />
        {deleteItem}
      </>
    );
  };

  return (
    <Menu.Root modal={false}>
      <Tooltip.Root>
        <Tooltip.Trigger
          render={<Menu.Trigger render={<Button variant="ghost" size="sm" aria-label={t("common.more")} icon={<MoreVerticalIcon />} />} />}
        />
        <Tooltip.Popup>{t("common.more")}</Tooltip.Popup>
      </Tooltip.Root>
      <Menu.Popup align="end">{renderItems()}</Menu.Popup>

      {/* Delete confirmation dialog */}
      <ConfirmDialog
        open={deleteDialogOpen}
        onOpenChange={setDeleteDialogOpen}
        title={t("memo.delete-confirm")}
        confirmLabel={t("trash.move")}
        description={t("memo.delete-confirm-description")}
        cancelLabel={t("common.cancel")}
        onConfirm={confirmDeleteMemo}
        confirmVariant="danger"
      />
    </Menu.Root>
  );
};

export default MemoActionMenu;
