import { Checkbox } from "@usememos/ui";
import { useMemo, useState } from "react";
import { MemoEditor } from "@/components/MemoEditor";
import { deriveDefaultCreateTimeFromDate } from "@/components/MemoEditor/edit/utils/deriveDefaultCreateTime";
import MemoFeedMenu from "@/components/MemoFeedMenu";
import MemoView from "@/components/MemoView";
import PagedMemoList, { EmptyLine, getMemoKey } from "@/components/PagedMemoList";
import { useExportMemos } from "@/components/Settings/useExportMemos";
import { useWelcomeGuide, WelcomeGuideDialog } from "@/components/WelcomeGuide";
import { useAuth } from "@/contexts/AuthContext";
import { useMemoFilterContext } from "@/contexts/MemoFilterContext";
import { NewMemoProvider } from "@/contexts/NewMemoContext";
import { useMemoFilters, useMemoSorting } from "@/hooks";
import useCurrentUser from "@/hooks/useCurrentUser";
import { useMemoSuggestions } from "@/hooks/useMemoSuggestions";
import { MAX_SELECTED_MEMO_EXPORT } from "@/lib/memo-export";
import { buildMemoCreatorFilter, extractMemoIdFromName } from "@/lib/resource-names";
import { State } from "@/types/proto/api/common_pb";
import { Memo } from "@/types/proto/api/memo_service_pb";
import { useTranslate } from "@/utils/i18n";

const Home = () => {
  const user = useCurrentUser();
  const suggestions = useMemoSuggestions();
  const t = useTranslate();
  const { isUserSettingsInitialized } = useAuth();
  const { filters } = useMemoFilterContext();
  const contextFilter = user && buildMemoCreatorFilter(user.name);
  const calendarDay = filters.find((filter) => filter.factor === "displayTime")?.value;
  const defaultCreateTime = useMemo(() => (calendarDay ? deriveDefaultCreateTimeFromDate(calendarDay) : undefined), [calendarDay]);

  const memoFilter = useMemoFilters({
    includePinned: true,
  });
  const welcomeGuide = useWelcomeGuide(Boolean(user));

  const { listSort, orderBy } = useMemoSorting({
    pinnedFirst: true,
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
    <div className="min-h-full w-full bg-mu-canvas pb-8 text-mu-text md:pt-5">
      <WelcomeGuideDialog open={welcomeGuide.open} onOpenChange={welcomeGuide.onOpenChange} />
      <NewMemoProvider>
        <PagedMemoList
          renderer={(memo: Memo, { compact }) => {
            const checked = selectedNames?.has(memo.name) ?? false;
            const memoView = (
              <MemoView
                key={getMemoKey(memo)}
                memo={memo}
                showPinned
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
          renderStickyHeader={() => (
            <>
              <MemoFeedMenu
                canExport={canExport}
                selectedCount={selectedNames?.size}
                exporting={exporting}
                onBeginSelection={() => setSelection({ scope: selectionScope, names: new Set() })}
                onEndSelection={() => setSelection(undefined)}
                onExport={download}
              />
              {isUserSettingsInitialized && user && (
                <MemoEditor
                  draft={{ scope: "home", createTime: defaultCreateTime }}
                  expandOnFocus
                  className="memo-list-gap"
                  placeholder={t("editor.any-thoughts")}
                  suggestions={suggestions}
                />
              )}
            </>
          )}
          emptyState={
            <EmptyLine
              message={t("memo.no-memos-yet")}
              action={user ? t("welcome.reopen") : undefined}
              onAction={user ? welcomeGuide.show : undefined}
            />
          }
        />
      </NewMemoProvider>
    </div>
  );
};

export default Home;
