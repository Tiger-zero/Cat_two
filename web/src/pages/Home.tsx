import { Checkbox } from "@usememos/ui";
import { useMemo, useState } from "react";
import { MemoEditor } from "@/components/MemoEditor";
import { deriveDefaultCreateTimeFromFilters } from "@/components/MemoEditor/edit/utils/deriveDefaultCreateTime";
import MemoFeedMenu from "@/components/MemoFeedMenu";
import MemoView from "@/components/MemoView";
import PagedMemoList, { EmptyLine, getMemoKey } from "@/components/PagedMemoList";
import { useExportMemos } from "@/components/Settings/useExportMemos";
import { useWelcomeGuide, WelcomeGuideDialog } from "@/components/WelcomeGuide";
import { useAuth } from "@/contexts/AuthContext";
import { useGlobalMemoEditor } from "@/contexts/GlobalMemoEditorContext";
import { useMemoFilterContext } from "@/contexts/MemoFilterContext";
import { NewMemoProvider } from "@/contexts/NewMemoContext";
import { useSpaceContext } from "@/contexts/SpaceContext";
import { useMemoFilters, useMemoSorting } from "@/hooks";
import useCurrentUser from "@/hooks/useCurrentUser";
import { useMemoSuggestions } from "@/hooks/useMemoSuggestions";
import { MAX_SELECTED_MEMO_EXPORT } from "@/lib/memo-export";
import { extractMemoIdFromName } from "@/lib/resource-names";
import { State } from "@/types/proto/api/common_pb";
import { Memo } from "@/types/proto/api/memo_service_pb";
import { useTranslate } from "@/utils/i18n";

const Home = () => {
  const user = useCurrentUser();
  const suggestions = useMemoSuggestions();
  const t = useTranslate();
  const { isUserSettingsInitialized } = useAuth();
  const { claimHomeAutoFocus } = useGlobalMemoEditor();
  const { filters } = useMemoFilterContext();
  const { memoFilter: contextFilter, selectedSpaceName, creatorUsername } = useSpaceContext();
  const defaultCreateTime = useMemo(() => deriveDefaultCreateTimeFromFilters(filters), [filters]);

  const memoFilter = useMemoFilters({
    includeMemoViews: true,
    includePinned: true,
  });
  const canComposeInScope = Boolean(user && (!creatorUsername || creatorUsername === user.username));
  // The welcome guide belongs to the account's own timeline, not to Explore, a space or
  // another member's page.
  const onOwnHome = Boolean(user && creatorUsername === user.username);
  const welcomeGuide = useWelcomeGuide(onOwnHome);
  // Pinning is the owner's arrangement of their own memos, so it only shapes a single
  // creator's feed; a mixed-author feed like Explore stays purely chronological.
  const honorPinned = Boolean(creatorUsername);

  const { listSort, orderBy } = useMemoSorting({
    pinnedFirst: honorPinned,
    state: State.NORMAL,
  });

  const { exportMemos, exporting } = useExportMemos();
  const canExport = Boolean(user);
  const selectionScope = JSON.stringify([user?.name, contextFilter, memoFilter]);
  const [selection, setSelection] = useState<{ scope: string; names: ReadonlySet<string> }>();
  if (selection && selection.scope !== selectionScope) setSelection(undefined);
  const selectedNames = canExport && selection?.scope === selectionScope ? selection.names : undefined;
  const setMemoSelected = (name: string, checked: boolean) => {
    setSelection((previous) => {
      if (!previous || previous.scope !== selectionScope) return previous;
      const names = new Set(previous.names);
      if (checked && names.size < MAX_SELECTED_MEMO_EXPORT) names.add(name);
      if (!checked) names.delete(name);
      return { scope: previous.scope, names };
    });
  };

  const download = () => {
    if (!user || selectedNames?.size === 0) return;
    void exportMemos(user.username, selectedNames ? Array.from(selectedNames, extractMemoIdFromName) : undefined);
  };

  return (
    <div className="w-full min-h-full bg-mu-canvas text-mu-text">
      <WelcomeGuideDialog open={welcomeGuide.open} onOpenChange={welcomeGuide.onOpenChange} />
      <NewMemoProvider>
        <PagedMemoList
          renderer={(memo: Memo, { compact }) => {
            const checked = selectedNames?.has(memo.name) ?? false;
            const memoView = (
              <MemoView
                key={getMemoKey(memo)}
                memo={memo}
                showCreator={!creatorUsername}
                showPinned={honorPinned}
                showSpace={!selectedSpaceName}
                compact={compact}
                className={checked ? "border-mu-accent" : undefined}
              />
            );
            if (!selectedNames || memo.creator !== user?.name) return memoView;
            return (
              <div key={getMemoKey(memo)} className="flex min-w-0 items-start gap-2">
                <div className="shrink-0 px-1 pt-4">
                  <Checkbox
                    aria-label={t("memo-feed.select-memo")}
                    checked={checked}
                    disabled={!checked && selectedNames.size >= MAX_SELECTED_MEMO_EXPORT}
                    onCheckedChange={(value) => setMemoSelected(memo.name, value)}
                  />
                </div>
                <div className="min-w-0 flex-1">{memoView}</div>
              </div>
            );
          }}
          listSort={listSort}
          orderBy={orderBy}
          filter={memoFilter}
          contextFilter={contextFilter}
          renderHeader={() => (
            <MemoFeedMenu
              canExport={canExport}
              selectedCount={selectedNames?.size}
              exporting={exporting}
              onBeginSelection={() => setSelection({ scope: selectionScope, names: new Set() })}
              onEndSelection={() => setSelection(undefined)}
              onExport={download}
            />
          )}
          emptyState={
            <EmptyLine
              message={t("memo.no-memos-yet")}
              action={onOwnHome ? t("welcome.reopen") : undefined}
              onAction={onOwnHome ? welcomeGuide.show : undefined}
            />
          }
          renderLeading={({ useGrid }) => {
            if (!isUserSettingsInitialized || !canComposeInScope) return null;

            return (
              <MemoEditor
                // Each Space keeps its own Home draft; the editor restarts when the Space changes.
                draft={{ scope: "home", space: selectedSpaceName, createTime: defaultCreateTime }}
                autoFocus={claimHomeAutoFocus}
                className={useGrid ? undefined : "mb-2"}
                placeholder={t("editor.any-thoughts")}
                suggestions={suggestions}
              />
            );
          }}
        />
      </NewMemoProvider>
    </div>
  );
};

export default Home;
