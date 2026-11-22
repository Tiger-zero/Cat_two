import { Button, Menu, Tooltip } from "@usememos/ui";
import { uniqBy } from "lodash-es";
import { HashIcon, ListTreeIcon, MoreHorizontalIcon, PencilIcon, PinIcon, PinOffIcon, Trash2Icon } from "lucide-react";
import { forwardRef, type ReactNode, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { getFilterSearch, getMemoFilterKey, replaceFiltersByFactor, useMemoFilterContext } from "@/contexts/MemoFilterContext";
import { useLocalStorage, useOverflowTitle } from "@/hooks";
import useCurrentUser from "@/hooks/useCurrentUser";
import { cn } from "@/lib/utils";
import { useTranslate } from "@/utils/i18n";
import TagTree, { compareTagNames, TAG_ROW_COUNT_CLASSES, tagRowAriaLabel } from "../TagTree";
import {
  SIDEBAR_ROW_BOX_CLASSES,
  SIDEBAR_ROW_LABEL_CLASSES,
  SidebarRowIconSlot,
  sidebarRowStateAttributes,
  sidebarRowStateClasses,
} from "./SidebarRow";
import SidebarSection from "./SidebarSection";
import { SIDEBAR_LIST_GAP_CLASSES, SIDEBAR_PINNED_TAGS_DIVIDER_CLASSES } from "./sidebar-layout";
import TagDeleteDialog from "./TagDeleteDialog";
import TagRenameDialog from "./TagRenameDialog";

interface Props {
  tagCount: Record<string, number>;
  onSelect?: () => void;
  destination?: string;
  /** Whose tags these are; keeps tree expansion state separate for each account and collection. */
  scope: string;
}

const TagPath = forwardRef<HTMLSpanElement, { tag: string }>(({ tag }, ref) => {
  const segments = tag.split("/");

  return (
    <span ref={ref} className="min-w-0 flex-1 truncate text-start">
      {segments.map((segment, index) => (
        <span key={`${segment}-${index}`}>
          {index > 0 && <span className="px-0.5 text-mu-text-secondary group-data-checked:text-(--memos-on-sidebar-selected)">/</span>}
          {segment}
        </span>
      ))}
    </span>
  );
});
TagPath.displayName = "TagPath";

interface FlatTagRowProps {
  tag: string;
  amount?: number;
  active: boolean;
  /** Computed by the parent, which already holds the translator — rows stay subscription-free. */
  ariaLabel: string;
  onClick: () => void;
  pinned?: boolean;
  actions?: ReactNode;
}

const FlatTagRow = ({ tag, amount, active, ariaLabel, onClick, pinned, actions }: FlatTagRowProps) => {
  const { ref, title } = useOverflowTitle<HTMLSpanElement>(`#${tag}`);
  const state = active ? "checked" : "idle";

  return (
    <div
      {...sidebarRowStateAttributes(state)}
      className={cn(SIDEBAR_ROW_BOX_CLASSES, sidebarRowStateClasses(state), !active && "text-mu-text")}
    >
      <button
        type="button"
        aria-label={ariaLabel}
        aria-pressed={active || undefined}
        title={title}
        className={SIDEBAR_ROW_LABEL_CLASSES}
        onClick={onClick}
      >
        <SidebarRowIconSlot icon={pinned ? PinIcon : HashIcon} />
        <TagPath ref={ref} tag={tag} />
        <span className={TAG_ROW_COUNT_CLASSES}>{amount}</span>
      </button>
      {actions}
    </div>
  );
};

const NO_PINS: string[] = [];
const isInTagBranch = (path: string, root: string) => path === root || path.startsWith(`${root}/`);

const TagsSectionForAccount = ({ tagCount, onSelect, destination, scope, account }: Props & { account?: string }) => {
  const t = useTranslate();
  const navigate = useNavigate();
  const { filters, removeFilter, setFilters } = useMemoFilterContext();
  const [treeMode, setTreeMode] = useLocalStorage<boolean>("tag-view-as-tree", true);
  const [pinnedPaths, setPinnedPaths] = useLocalStorage<string[]>(`memos-pinned-tags:${account ?? "guest"}`, NO_PINS);
  const pins = account && Array.isArray(pinnedPaths) ? pinnedPaths : NO_PINS;
  const pinnedSet = new Set(pins);
  const [tagAction, setTagAction] = useState<{ tag: string; action: "rename" | "delete" }>();
  const activeTags = new Set(filters.filter((filter) => filter.factor === "tagSearch").map((filter) => filter.value));
  const activeTag = activeTags.values().next().value as string | undefined;
  const tags = useMemo(() => {
    const entries = Object.entries(tagCount);
    return treeMode ? entries : entries.sort(([left], [right]) => compareTagNames(left, right));
  }, [tagCount, treeMode]);

  if (tags.length === 0) {
    return null;
  }

  const handleTagClick = (tag: string) => {
    const nextFilters =
      !destination && activeTags.has(tag)
        ? filters.filter((filter) => filter.factor !== "tagSearch" || filter.value !== tag)
        : replaceFiltersByFactor(filters, "tagSearch", [{ factor: "tagSearch", value: tag }]);
    if (destination) void navigate({ pathname: destination, search: getFilterSearch(nextFilters) });
    else setFilters(nextFilters);
    onSelect?.();
  };

  const togglePin = (tag: string) => {
    setPinnedPaths((saved) => {
      const paths = Array.isArray(saved) ? saved : NO_PINS;
      return paths.includes(tag) ? paths.filter((path) => path !== tag) : [...paths, tag];
    });
  };

  const handleRenamed = (oldName: string, newName: string) => {
    const renamePath = (path: string) => (isInTagBranch(path, oldName) ? `${newName}${path.slice(oldName.length)}` : path);
    setPinnedPaths((saved) => (Array.isArray(saved) ? Array.from(new Set(saved.map(renamePath))) : NO_PINS));
    setFilters(
      uniqBy(
        filters.map((filter) => (filter.factor === "tagSearch" ? { ...filter, value: renamePath(filter.value) } : filter)),
        getMemoFilterKey,
      ),
    );
  };

  const handleDeleted = (tag: string) => {
    setPinnedPaths((saved) => (Array.isArray(saved) ? saved.filter((path) => !isInTagBranch(path, tag)) : NO_PINS));
    removeFilter((filter) => filter.factor === "tagSearch" && isInTagBranch(filter.value, tag));
  };

  const renderActions = (tag: string) => {
    if (!account) return null;
    const pinned = pinnedSet.has(tag);
    return (
      <Menu.Root modal={false}>
        <Menu.Trigger
          aria-label={t("setting.tags.actions", { tag: `#${tag}` })}
          render={
            <Button
              variant="ghost"
              size="xs"
              icon={<MoreHorizontalIcon />}
              className="in-[[data-sidebar-collection-list]]:opacity-0 group-hover:opacity-100 group-has-[:focus-visible]:opacity-100 focus-visible:opacity-100 data-popup-open:opacity-100 group-data-checked:text-(--memos-on-sidebar-selected) group-data-checked:hover:bg-(--memos-on-sidebar-selected)/15"
            />
          }
        />
        <Menu.Popup align="end">
          <Menu.Item icon={pinned ? <PinOffIcon /> : <PinIcon />} onClick={() => togglePin(tag)}>
            {t(pinned ? "common.unpin" : "common.pin")}
          </Menu.Item>
          <Menu.Item icon={<PencilIcon />} onClick={() => setTagAction({ tag, action: "rename" })}>
            {t("setting.tags.rename")}
          </Menu.Item>
          <Menu.Item icon={<Trash2Icon />} onClick={() => setTagAction({ tag, action: "delete" })}>
            {t("common.delete")}
          </Menu.Item>
        </Menu.Popup>
      </Menu.Root>
    );
  };

  const flatRow = (tag: string, amount?: number, pinned = false) => (
    <FlatTagRow
      key={tag}
      tag={tag}
      amount={amount}
      active={activeTags.has(tag)}
      ariaLabel={tagRowAriaLabel(t, tag, amount)}
      onClick={() => handleTagClick(tag)}
      pinned={pinned}
      actions={renderActions(tag)}
    />
  );
  const visiblePins = treeMode
    ? NO_PINS
    : pins.filter((tag) => Object.hasOwn(tagCount, tag) || tags.some(([name]) => name.startsWith(`${tag}/`)));
  const flatTags = treeMode ? [] : tags.filter(([tag]) => !pinnedSet.has(tag));

  return (
    <SidebarSection
      className="group/tags"
      label={t("common.tags")}
      action={
        <Menu.Root modal={false}>
          <Tooltip.Root>
            <Tooltip.Trigger
              render={
                <Menu.Trigger
                  aria-label={`${t("common.tags")}: ${t("common.more")}`}
                  render={<Button variant="ghost" size="xs" icon={<MoreHorizontalIcon />} />}
                />
              }
            />
            <Tooltip.Popup side="top">{t("common.more")}</Tooltip.Popup>
          </Tooltip.Root>
          <Menu.Popup align="end">
            <Menu.Group>
              <Menu.GroupLabel>{t("common.tags")}</Menu.GroupLabel>
              <Menu.CheckboxItem checked={treeMode} onCheckedChange={setTreeMode} closeOnClick icon={<ListTreeIcon />}>
                {t("common.tree-mode")}
              </Menu.CheckboxItem>
            </Menu.Group>
          </Menu.Popup>
        </Menu.Root>
      }
    >
      {treeMode ? (
        <TagTree
          key={scope}
          tagAmounts={tags}
          activeTag={activeTag}
          scope={scope}
          onTagClick={handleTagClick}
          pinnedPaths={pins}
          renderActions={renderActions}
        />
      ) : (
        <>
          {visiblePins.length > 0 && (
            <div
              className={cn("flex flex-col", SIDEBAR_LIST_GAP_CLASSES, flatTags.length > 0 && SIDEBAR_PINNED_TAGS_DIVIDER_CLASSES)}
              role="group"
              aria-label={t("setting.tags.pinned")}
            >
              {visiblePins.map((tag) => flatRow(tag, Object.hasOwn(tagCount, tag) ? tagCount[tag] : undefined, true))}
            </div>
          )}
          {flatTags.map(([tag, amount]) => flatRow(tag, amount))}
        </>
      )}
      {tagAction?.action === "rename" && account && (
        <TagRenameDialog
          oldName={tagAction.tag}
          onClose={() => setTagAction(undefined)}
          onRenamed={(name) => handleRenamed(tagAction.tag, name)}
        />
      )}
      {tagAction?.action === "delete" && account && (
        <TagDeleteDialog tagName={tagAction.tag} onClose={() => setTagAction(undefined)} onDeleted={() => handleDeleted(tagAction.tag)} />
      )}
    </SidebarSection>
  );
};

const TagsSection = (props: Props) => {
  const account = useCurrentUser()?.name;
  return <TagsSectionForAccount key={account} {...props} account={account} />;
};

export default TagsSection;
