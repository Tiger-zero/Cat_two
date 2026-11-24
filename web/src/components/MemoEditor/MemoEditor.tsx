import { useQueryClient } from "@tanstack/react-query";
import { type ReactNode, Suspense, useCallback, useEffect, useState } from "react";
import useCurrentUser from "@/hooks/useCurrentUser";
import { memoDetailQueryOptions, useMemoByName, useUpdateMemo } from "@/hooks/useMemoQueries";
import { recordMemoEditingActivity } from "@/lib/memo-editing-activity";
import type { MemoSuggestion } from "@/lib/memo-suggestions";
import { showErrorToast, toast } from "@/lib/toast";
import { cn } from "@/lib/utils";
import type { Memo } from "@/types/proto/api/memo_service_pb";
import { useTranslate } from "@/utils/i18n";
import { canManageMemo } from "@/utils/user";
import { COMPOSER_PANEL_CLASSES } from "./edit/constants";
import { draftKeyFor, type MemoEditIdentity, migrateLegacyDraft, resolveHomeDraftKey } from "./edit/draftKey";
import { initialEditorState } from "./edit/initialState";
import { getEditSession } from "./edit/loadEditSession";
import { cacheService } from "./edit/services/cacheService";
import { EditorProvider } from "./edit/state";
import { MemoContentView } from "./view/MemoContentView";

export interface MemoEditorViewProps {
  mode?: "view";
  /** The memo's resource name, such as `memos/abc`. */
  name: string;
  /** The card renders collapsed, so in-memo anchor links open the detail page instead of scrolling. */
  compact?: boolean;
  /** Collection page that rendered the memo, preserved when an inline link opens its detail page. */
  parentPage?: string;
  className?: string;
  contentClassName?: string;
  onClick?: (event: React.MouseEvent) => void;
  onDoubleClick?: (event: React.MouseEvent) => void;
}

/** What a new memo or comment starts from. */
export interface MemoDraftSeed {
  /** The memo this draft comments on. A comment has no scope or timestamps of its own. */
  parent?: string;
  /**
   * Where a new memo is composed, such as `home` or `global`.
   * Each scope keeps its own draft.
   */
  scope?: string;
  /**
   * Seeds `createTime` and `updateTime` and shows the timestamp popover.
   * Updated while the draft is open when the calendar's selected day changes.
   */
  createTime?: Date;
}

export interface MemoEditingProps {
  /** A callback can decide whether focus is still appropriate after the draft is restored. */
  autoFocus?: boolean | (() => boolean);
  placeholder?: string;
  /** Context-derived suggestions for a new top-level memo. */
  suggestions?: readonly MemoSuggestion[];
  className?: string;
  /** Inside a card that is the frame already: no background, border or padding of its own outside focus mode. */
  frameless?: boolean;
  /** Leads the timestamp row, before the time: the card's creator while it edits in place. */
  timestampLead?: ReactNode;
  /** Reports inline focus-mode changes, for a host whose layout must let the editor take the viewport. */
  onFocusModeChange?: (isFocusMode: boolean) => void;
  onConfirm?: (memoName: string) => void;
  /** Offers Cancel; without it the session has no way out but saving, as the Home composer. */
  onCancel?: () => void;
}

/** Editing an existing memo by name. */
export interface MemoEditorEditProps extends MemoEditingProps {
  mode: "edit";
  name: string;
}

/** Composing a new memo or comment. */
export interface MemoEditorDraftProps extends MemoEditingProps {
  mode?: "edit";
  draft: MemoDraftSeed;
  /** Give the desktop inline composer more writing room while it has focus. */
  expandOnFocus?: boolean;
}

export type MemoEditorProps = MemoEditorViewProps | MemoEditorEditProps | MemoEditorDraftProps;

/**
 * A memo, shown or edited. In view mode it reads the memo by name from whatever query
 * already holds it (a list, a comment page, a detail) and shows it read-only; a viewer
 * who can manage the memo ticks its task items, and each tick is saved through the memo
 * mutation, so every copy of the memo on the page follows at once. In edit mode, for an
 * existing memo by name or a new memo from a draft seed, it owns the draft store,
 * initialized once from the memo or the persisted draft, and renders the editing
 * session over it. The parent owns the mode and hears back through `onConfirm` and
 * `onCancel`.
 */
export const MemoEditor = (props: MemoEditorProps) => {
  if ("draft" in props) return <MemoEditorDraft {...props} />;
  if (props.mode === "edit") return <MemoEditorEdit {...props} />;
  return <MemoEditorView {...props} />;
};

const MemoEditorView = ({ name, compact, parentPage, className, contentClassName, onClick, onDoubleClick }: MemoEditorViewProps) => {
  const t = useTranslate();
  const { data: memo } = useMemoByName(name);
  const currentUser = useCurrentUser();
  const { mutateAsync: updateMemo } = useUpdateMemo(name);
  if (!memo) return null;

  const handleTaskToggle = (content: string, taskBaseContent: string) => {
    recordMemoEditingActivity();
    void updateMemo({
      update: { name, content },
      updateMask: ["content", "update_time"],
      expectedRevision: memo.revision,
      taskBaseContent,
    }).catch(() => showErrorToast(t("editor.task-update-failed")));
  };

  return (
    <MemoContentView
      memoName={name}
      content={memo.content}
      attachments={memo.attachments}
      parentPage={parentPage}
      compact={compact}
      onTaskToggle={canManageMemo(memo, currentUser) ? handleTaskToggle : undefined}
      className={className}
      contentClassName={contentClassName}
      onClick={onClick}
      onDoubleClick={onDoubleClick}
    />
  );
};

const MemoEditorEdit = ({ name, ...editing }: MemoEditorEditProps) => {
  const { data: memo } = useMemoByName(name);
  // The memo stays readable while the session's code loads, and until the memo itself has.
  const fallback = <MemoEditorView name={name} className={editing.className} />;
  if (!memo) return fallback;
  const identity: MemoEditIdentity = { kind: "edit", name };
  return (
    <Suspense fallback={fallback}>
      <EditingSession key={draftKeyFor(identity)} identity={identity} memo={memo} {...editing} />
    </Suspense>
  );
};

const MemoEditorDraft = ({ draft, ...editing }: MemoEditorDraftProps) => {
  const identity: MemoEditIdentity = draft.parent
    ? { kind: "comment", parent: draft.parent }
    : { kind: "new", scope: draft.scope ?? "default" };
  // While the session's code loads, a still of the empty composer: the same frame, the
  // placeholder line and the toolbar's height, so the page below does not move when the
  // session takes its place.
  const fallback = editing.frameless ? null : (
    <div aria-hidden className={cn("flex w-full flex-col gap-2", COMPOSER_PANEL_CLASSES, editing.className)}>
      <div className={cn("memo-text w-full text-mu-text-secondary", editing.expandOnFocus && "md:min-h-9")}>
        {editing.placeholder ?? " "}
      </div>
      <div className="h-[37px] w-full border-t border-mu-line/75" />
    </div>
  );
  return (
    <Suspense fallback={fallback}>
      <EditingSession
        key={draftKeyFor(identity)}
        identity={identity}
        parentMemoName={draft.parent}
        defaultCreateTime={draft.createTime}
        fallback={fallback}
        {...editing}
      />
    </Suspense>
  );
};

interface EditingSessionProps extends MemoEditingProps {
  identity: MemoEditIdentity;
  expandOnFocus?: boolean;
  memo?: Parameters<typeof initialEditorState>[0]["memo"];
  parentMemoName?: string;
  defaultCreateTime?: Date;
}

const isHomeDraft = (identity: MemoEditIdentity) => identity.kind === "new" && identity.scope === "home";

/** Remount the draft store if either its subject or its account changes. */
const EditingSession = ({ fallback, ...props }: EditingSessionProps & { fallback?: ReactNode }) => {
  const currentUser = useCurrentUser();
  const username = currentUser?.name ?? "";
  const draftKey = cacheService.key(username, draftKeyFor(props.identity));
  if (isHomeDraft(props.identity)) {
    return <HomeDraftSession key={draftKey} draftKey={draftKey} username={username} fallback={fallback} {...props} />;
  }
  return <DraftEditingSession key={draftKey} draftKey={draftKey} username={username} {...props} />;
};

type DraftEditingSessionProps = EditingSessionProps & { draftKey: string; username: string };

const HomeDraftSession = ({ fallback, ...props }: DraftEditingSessionProps & { fallback?: ReactNode }) => {
  const { draftKey, username } = props;
  const [selectedKey, setSelectedKey] = useState(() => (cacheService.readForEditor(draftKey) === null ? undefined : draftKey));
  useEffect(() => {
    if (selectedKey !== undefined) return;
    let active = true;
    void resolveHomeDraftKey(username).then((key) => {
      if (active) setSelectedKey(key);
    });
    return () => {
      active = false;
    };
  }, [selectedKey, username]);

  return selectedKey === undefined ? fallback : <DraftEditingSession {...props} draftKey={selectedKey} />;
};

const DraftEditingSession = ({
  identity,
  memo,
  parentMemoName,
  defaultCreateTime,
  draftKey,
  username,
  ...editing
}: DraftEditingSessionProps) => {
  const queryClient = useQueryClient();
  const t = useTranslate();
  const [seed, setSeed] = useState(() => {
    if (!isHomeDraft(identity)) migrateLegacyDraft(username, identity);
    const rawDraft = cacheService.readForEditor(draftKey);
    const initialState = initialEditorState({
      memo,
      draftKey,
      rawDraft,
      defaultCreateTime,
    });
    // Initialization can remove a legacy edit draft that cannot be restored.
    return {
      initialState,
      initialDraftRaw: rawDraft && !cacheService.readForEditor(draftKey) ? null : rawDraft,
      key: 0,
      reviewed: undefined as { memo: Memo; filesRevision: string } | undefined,
    };
  });
  const memoName = memo?.name;
  const reviewDraft = useCallback(
    async (filesRevision: string) => {
      if (!memoName) return;
      try {
        const current = await queryClient.fetchQuery({ ...memoDetailQueryOptions(memoName), staleTime: 0 });
        const rawDraft = cacheService.readForEditor(draftKey);
        if (!current) {
          showErrorToast(t("editor.draft.review-failed"));
          return;
        }
        setSeed((previous) => ({
          initialState: initialEditorState({ memo: current, draftKey, rawDraft, reviewStaleDraft: true }),
          initialDraftRaw: rawDraft,
          key: previous.key + 1,
          reviewed: { memo: current, filesRevision },
        }));
        toast.add({ title: t("editor.draft.review-loaded"), type: "warning" });
      } catch {
        showErrorToast(t("editor.draft.review-failed"));
      }
    },
    [draftKey, memoName, queryClient, t],
  );
  const EditSession = getEditSession();
  return (
    <EditorProvider key={seed.key} initialEditorState={seed.initialState}>
      <EditSession
        memo={seed.reviewed?.memo ?? memo}
        parentMemoName={parentMemoName}
        defaultCreateTime={defaultCreateTime}
        draftKey={draftKey}
        initialDraftRaw={seed.initialDraftRaw}
        reviewDraftRevision={seed.reviewed?.filesRevision}
        onReviewDraft={memoName ? reviewDraft : undefined}
        {...editing}
      />
    </EditorProvider>
  );
};
