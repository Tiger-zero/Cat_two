import { ChevronRightIcon, HashIcon, PinIcon } from "lucide-react";
import { type ReactNode, useEffect, useMemo } from "react";
import {
  SIDEBAR_ROW_BOX_CLASSES,
  SIDEBAR_ROW_COUNT_RAIL_CLASSES,
  SIDEBAR_ROW_ICON_CLASSES,
  SIDEBAR_ROW_LABEL_CLASSES,
  SIDEBAR_ROW_SLOT_BUTTON_CLASSES,
  SidebarRowIconSlot,
  sidebarRowStateAttributes,
  sidebarRowStateClasses,
} from "@/components/AppSidebar/SidebarRow";
import { SIDEBAR_LIST_GAP_CLASSES, SIDEBAR_PINNED_TAGS_DIVIDER_CLASSES } from "@/components/AppSidebar/sidebar-layout";
import { useLocalStorage, useOverflowTitle } from "@/hooks";
import { cn } from "@/lib/utils";
import { useTranslate } from "@/utils/i18n";

/** Matches the memo outline's indent, the app's other nested rail list. */
const INDENT_STEP = 12;
// The branch line sits at the center of the 20px leading slot.
const BRANCH_LINE_OFFSET = 10;

interface TagTreeNode {
  key: string;
  text: string;
  amount?: number;
  subTags: TagTreeNode[];
}

interface Props {
  tagAmounts: [tag: string, amount: number][];
  activeTag?: string;
  /** Identifies whose tags these are, so expansion persists per tenant rather than per browser. */
  scope: string;
  onTagClick: (tag: string) => void;
  pinnedPaths?: readonly string[];
  renderActions?: (tag: string) => ReactNode;
}

/** Persisted per scope: which branches are open, and the selection already revealed. */
interface TagTreeExpansion {
  expanded: string[];
  revealedFor?: string;
}

const EMPTY_EXPANSION: TagTreeExpansion = { expanded: [] };

/** One announcement for a tag row in either layout, so tree and flat mode never drift apart. */
export const tagRowAriaLabel = (t: ReturnType<typeof useTranslate>, tag: string, amount?: number) =>
  amount === undefined ? `#${tag}` : `#${tag}, ${t("setting.tags.used-count", { count: amount })}`;

// Keep count widths stable while revealing every row from the same desktop tag section.
export const TAG_ROW_COUNT_CLASSES = `${SIDEBAR_ROW_COUNT_RAIL_CLASSES} in-[[data-sidebar-collection-list]]:opacity-0 group-hover/tags:opacity-100 group-has-[:focus-visible]/tags:opacity-100`;

export const buildTagTree = (tagAmounts: [tag: string, amount: number][]) => {
  const root: TagTreeNode = {
    key: "",
    text: "",
    subTags: [],
  };

  for (const [tag, amount] of [...tagAmounts].sort(([left], [right]) => left.localeCompare(right))) {
    const segments = tag.split("/");
    let parent = root;
    let path = "";

    segments.forEach((segment, index) => {
      path = path ? `${path}/${segment}` : segment;
      let node = parent.subTags.find((item) => item.key === segment);

      if (!node) {
        node = {
          key: segment,
          text: path,
          subTags: [],
        };
        parent.subTags.push(node);
      }

      if (index === segments.length - 1) {
        node.amount = amount;
      }
      parent = node;
    });
  }

  return root.subTags;
};

const parentPathsOf = (tag: string): string[] => {
  const segments = tag.split("/");
  return segments.slice(0, -1).map((_, index) => segments.slice(0, index + 1).join("/"));
};

/** Lift pinned branches out of their usual position while retaining their descendants. */
function partitionPinnedTags(tags: TagTreeNode[], pinnedPaths: readonly string[]) {
  if (pinnedPaths.length === 0) return { pinned: [], unpinned: tags };
  const paths = new Set(pinnedPaths);
  const pinnedByPath = new Map<string, TagTreeNode>();
  const partition = (nodes: TagTreeNode[]): TagTreeNode[] =>
    nodes.flatMap((tag) => {
      const node = { ...tag, subTags: partition(tag.subTags) };
      if (!paths.has(tag.text)) return [node];
      pinnedByPath.set(tag.text, node);
      return [];
    });
  const unpinned = partition(tags);
  const pinned = pinnedPaths.flatMap((path) => {
    const tag = pinnedByPath.get(path);
    return tag ? [tag] : [];
  });
  return { pinned, unpinned };
}

const Chevron = ({ open, className }: { open: boolean; className?: string }) => (
  <ChevronRightIcon
    aria-hidden="true"
    className={cn(
      "mu-icon-sm",
      "me-auto shrink-0 text-mu-text-secondary/70 transition-transform",
      open ? "rotate-90" : "rtl:rotate-180",
      className,
    )}
  />
);

interface TagItemProps {
  tag: TagTreeNode;
  depth: number;
  activeTag?: string;
  expanded: ReadonlySet<string>;
  onTagClick: (tag: string) => void;
  onToggle: (path: string) => void;
  pinned?: boolean;
  renderActions?: (tag: string) => ReactNode;
}

const TagItem = ({ tag, depth, activeTag, expanded, onTagClick, onToggle, pinned, renderActions }: TagItemProps) => {
  const t = useTranslate();
  const isActive = activeTag === tag.text;
  const hasSubTags = tag.subTags.length > 0;
  const open = hasSubTags && expanded.has(tag.text);
  const { ref: labelRef, title } = useOverflowTitle<HTMLSpanElement>(`#${tag.text}`);
  const Icon = pinned ? PinIcon : HashIcon;
  const state = isActive ? "checked" : "idle";
  const paddingInlineStart = 8 + depth * INDENT_STEP;

  return (
    <div className="w-full min-w-0">
      <div
        role="treeitem"
        aria-level={depth + 1}
        aria-selected={isActive || undefined}
        aria-expanded={hasSubTags ? open : undefined}
        {...sidebarRowStateAttributes(state)}
        className={cn(SIDEBAR_ROW_BOX_CLASSES, sidebarRowStateClasses(state), "text-mu-text")}
        // Overrides the start half of the box's `px-2`, leaving the trailing 8px intact.
        style={{ paddingInlineStart }}
      >
        {hasSubTags && (
          // At rest the slot holds the row's # mark like any other tag; pointing at the row
          // (or tabbing into it) swaps in the chevron, so branches only look different while
          // the disclosure is actually reachable.
          <button
            type="button"
            aria-label={`${open ? t("common.collapse") : t("common.expand")} #${tag.text}`}
            aria-expanded={open}
            className={SIDEBAR_ROW_SLOT_BUTTON_CLASSES}
            onClick={() => onToggle(tag.text)}
          >
            <Icon aria-hidden="true" className={cn(SIDEBAR_ROW_ICON_CLASSES, "group-hover:hidden group-has-[:focus-visible]:hidden")} />
            <Chevron open={open} className="hidden group-hover:block group-has-[:focus-visible]:block" />
          </button>
        )}
        <button
          type="button"
          aria-label={tagRowAriaLabel(t, tag.text, tag.amount)}
          aria-pressed={isActive || undefined}
          title={title}
          className={SIDEBAR_ROW_LABEL_CLASSES}
          onClick={() => onTagClick(tag.text)}
        >
          {!hasSubTags && <SidebarRowIconSlot icon={Icon} />}
          <span ref={labelRef} className="min-w-0 flex-1 truncate">
            {pinned ? tag.text : tag.key}
          </span>
          <span className={TAG_ROW_COUNT_CLASSES}>{tag.amount}</span>
        </button>
        {renderActions?.(tag.text)}
      </div>

      {open && (
        <div
          className={cn("relative mt-0.5 flex w-full min-w-0 flex-col in-[[data-sidebar-collection-list]]:mt-1", SIDEBAR_LIST_GAP_CLASSES)}
          role="group"
        >
          <span
            aria-hidden="true"
            className="pointer-events-none absolute inset-y-1 w-px bg-mu-line"
            style={{ insetInlineStart: paddingInlineStart + BRANCH_LINE_OFFSET }}
          />
          {tag.subTags.map((subTag) => (
            <TagItem
              key={subTag.text}
              tag={subTag}
              depth={depth + 1}
              activeTag={activeTag}
              expanded={expanded}
              onTagClick={onTagClick}
              onToggle={onToggle}
              renderActions={renderActions}
            />
          ))}
        </div>
      )}
    </div>
  );
};

const NO_PINS: readonly string[] = [];

const TagTree = ({ tagAmounts, activeTag, scope, onTagClick, pinnedPaths = NO_PINS, renderActions }: Props) => {
  const t = useTranslate();
  const tags = useMemo(() => buildTagTree(tagAmounts), [tagAmounts]);
  const { pinned, unpinned } = useMemo(() => partitionPinnedTags(tags, pinnedPaths), [tags, pinnedPaths]);
  // Scoped per tenant: the paths are one account's tag names, so another user's profile tree
  // must not decide what your own library shows.
  const [state, setState] = useLocalStorage<TagTreeExpansion>(`tag-tree-expanded:${scope}`, EMPTY_EXPANSION);
  const expandedPaths = state?.expanded ?? EMPTY_EXPANSION.expanded;
  const expanded = useMemo(() => new Set(expandedPaths), [expandedPaths]);

  const handleToggle = (path: string) => {
    setState((current) => {
      const paths = current?.expanded ?? [];
      return { ...current, expanded: paths.includes(path) ? paths.filter((item) => item !== path) : [...paths, path] };
    });
  };

  // Reveal the active tag when a filter arrives from elsewhere (URL, memo content). `revealedFor`
  // is persisted so this happens once per selection: collapsing afterwards sticks even though the
  // effect runs again on every mount, and re-picking the same tag later reveals it again.
  useEffect(() => {
    setState((current) => {
      if (!activeTag) return current?.revealedFor === undefined ? current : { ...current, revealedFor: undefined };
      if (current?.revealedFor === activeTag) return current;

      const parents = parentPathsOf(activeTag);
      const paths = current?.expanded ?? [];
      const missing = parents.some((path) => !paths.includes(path));
      return { expanded: missing ? [...new Set([...paths, ...parents])] : paths, revealedFor: activeTag };
    });
  }, [activeTag, setState]);

  const renderTree = (nodes: TagTreeNode[], isPinned: boolean, label: string) =>
    nodes.length > 0 && (
      <div
        className={cn(
          "relative flex h-auto w-full flex-col items-stretch",
          SIDEBAR_LIST_GAP_CLASSES,
          isPinned && unpinned.length > 0 && SIDEBAR_PINNED_TAGS_DIVIDER_CLASSES,
        )}
        role="tree"
        aria-label={label}
      >
        {nodes.map((tag) => (
          <TagItem
            key={tag.text}
            tag={tag}
            depth={0}
            activeTag={activeTag}
            expanded={expanded}
            onTagClick={onTagClick}
            onToggle={handleToggle}
            pinned={isPinned}
            renderActions={renderActions}
          />
        ))}
      </div>
    );

  return (
    <>
      {renderTree(pinned, true, t("setting.tags.pinned"))}
      {renderTree(unpinned, false, t("common.tags"))}
    </>
  );
};

export default TagTree;
