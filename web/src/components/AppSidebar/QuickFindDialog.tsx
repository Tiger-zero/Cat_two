import { Button, Dialog, Input, Textarea, Toggle, ToggleGroup, Tooltip } from "@usememos/ui";
import { CornerDownLeftIcon, HashIcon, SearchIcon, XIcon } from "lucide-react";
import { type ChangeEvent, type FormEvent, type KeyboardEvent, useEffect, useId, useLayoutEffect, useRef, useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { useAppSidebar } from "@/contexts/AppSidebarContext";
import {
  getFilterSearch,
  isSearchFilter,
  type MemoFilter,
  replaceFiltersByFactor,
  useMemoFilterContext,
} from "@/contexts/MemoFilterContext";
import useCurrentUser from "@/hooks/useCurrentUser";
import { type MemoScope, resolveMemoScope } from "@/lib/memo-collection";
import { ROUTES } from "@/router/routes";
import { useTranslate } from "@/utils/i18n";
import { getRouteActionPolicy } from "./routes";

export type QuickFindMode = "text" | "cel";

const buildSearchFilters = (query: string, mode: QuickFindMode): MemoFilter[] => {
  const trimmed = query.trim();
  if (mode === "cel") return trimmed ? [{ factor: "celSearch", value: trimmed }] : [];
  return Array.from(new Set(trimmed.split(/\s+/).filter(Boolean))).map((value) => ({ factor: "contentSearch", value }));
};

export const buildQuickFindFilters = (
  query: string,
  currentFilters: MemoFilter[],
  preserveCurrentScope: boolean,
  mode: QuickFindMode,
): MemoFilter[] => {
  const scopeFilters = preserveCurrentScope ? currentFilters.filter((filter) => !isSearchFilter(filter)) : [];
  return [...scopeFilters, ...buildSearchFilters(query, mode)];
};

/** The inverse of buildQuickFindFilters: the query and mode that the active filters were submitted with. */
export const readQuickFindQuery = (filters: MemoFilter[]): { query: string; mode: QuickFindMode } => {
  const celSearch = filters.find((filter) => filter.factor === "celSearch");
  if (celSearch) return { query: celSearch.value, mode: "cel" };
  return {
    query: filters
      .filter((filter) => filter.factor === "contentSearch")
      .map((filter) => filter.value)
      .join(" "),
    mode: "text",
  };
};

export interface QuickFindSubmission {
  filters: MemoFilter[];
  destination?: string;
}

export const resolveQuickFindSubmission = (
  pathname: string,
  query: string,
  currentFilters: MemoFilter[],
  mode: QuickFindMode,
  scope = resolveMemoScope(pathname),
  untagged = false,
): QuickFindSubmission => {
  const routePolicy = getRouteActionPolicy(pathname);
  const searchFilters = buildQuickFindFilters(query, currentFilters, routePolicy.searchScope !== "all", mode);
  const filters = untagged ? replaceFiltersByFactor(searchFilters, "untagged", [{ factor: "untagged", value: "" }]) : searchFilters;
  const target =
    scope === resolveMemoScope(pathname) ? routePolicy.searchDestination : scope === "archived" ? ROUTES.ARCHIVED : ROUTES.HOME;
  return {
    filters,
    destination: target ? `${target}${getFilterSearch(filters)}` : undefined,
  };
};

const QuickFindDialog = () => {
  const t = useTranslate();
  const { pathname } = useLocation();
  const navigate = useNavigate();
  const currentUser = useCurrentUser();
  const { filters, setFilters } = useMemoFilterContext();
  const { quickFindOpen, setQuickFindOpen } = useAppSidebar();
  const [query, setQuery] = useState("");
  const [mode, setMode] = useState<QuickFindMode>("text");
  const [scope, setScope] = useState<MemoScope>(() => resolveMemoScope(pathname));
  const [autoFocusField, setAutoFocusField] = useState(true);
  const fieldRef = useRef<HTMLInputElement | HTMLTextAreaElement>(null);
  const previousMode = useRef(mode);
  const hintId = useId();
  const effectiveScope = currentUser ? scope : "home";
  const scopeLabel = t(effectiveScope === "archived" ? "common.archived" : "common.memos");

  useEffect(() => {
    if (quickFindOpen) setScope(resolveMemoScope(pathname));
  }, [pathname, quickFindOpen]);

  useEffect(() => {
    if (!quickFindOpen) return;
    setAutoFocusField(true);
    const active = readQuickFindQuery(filters);
    setMode(active.mode);
    setQuery(active.query);
  }, [filters, quickFindOpen]);

  useLayoutEffect(() => {
    const changedMode = previousMode.current !== mode;
    previousMode.current = mode;
    if (quickFindOpen && changedMode && autoFocusField) fieldRef.current?.focus();
  }, [autoFocusField, mode, quickFindOpen]);

  const submitQuery = (untagged = false) => {
    const submission = resolveQuickFindSubmission(pathname, query, filters, mode, effectiveScope, untagged);

    if (submission.destination) {
      navigate(submission.destination);
    } else {
      setFilters(submission.filters);
    }

    setQuickFindOpen(false);
  };

  const handleSubmit = (event: FormEvent) => {
    event.preventDefault();
    submitQuery();
  };

  const handleKeyDown = (event: KeyboardEvent<HTMLInputElement | HTMLTextAreaElement>) => {
    // Shift+Enter inserts a newline in an expression; every other Enter submits.
    if (event.key !== "Enter" || (mode === "cel" && event.shiftKey)) return;
    // Enter that commits an IME composition must not also submit the search.
    if (event.nativeEvent.isComposing || event.keyCode === 229) return;
    event.preventDefault();
    submitQuery();
  };

  const fieldProps = {
    ref: (element: HTMLInputElement | HTMLTextAreaElement | null) => {
      fieldRef.current = element;
    },
    value: query,
    onChange: (event: ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => setQuery(event.target.value),
    onKeyDown: handleKeyDown,
  };

  return (
    <Dialog.Root open={quickFindOpen} onOpenChange={setQuickFindOpen}>
      {/* Anchored near the top, not centered, so switching modes grows the panel downward instead of shifting it. */}
      <Dialog.Popup
        variant="bare"
        sheet={false}
        initialFocus={fieldRef}
        className="w-xl self-start bg-mu-surface mt-[max(0px,calc(12vh-var(--mu-space-6)))] sm:mt-[max(0px,calc(16vh-var(--mu-space-6)))]"
      >
        <header className="flex items-center justify-between gap-3 px-5 pt-4 pb-3 sm:px-6">
          <Dialog.Title className="mu-title">{t("common.search")}</Dialog.Title>
          <Dialog.Close render={<Button variant="ghost" size="sm" icon={<XIcon />} aria-label={t("common.close")} />} />
        </header>
        <Dialog.Description className="mu-visually-hidden">
          {t("common.search")} {scopeLabel}
        </Dialog.Description>
        <form onSubmit={handleSubmit}>
          <div className="mx-5 flex flex-wrap items-center justify-between gap-x-3 gap-y-2 rounded-mu-lg border border-mu-line/65 bg-mu-canvas/65 p-1.5 sm:mx-6">
            {currentUser ? (
              <ToggleGroup
                size="sm"
                aria-label={t("search.scope")}
                value={[effectiveScope]}
                onValueChange={([next]: string[]) => {
                  if (next === "home" || next === "archived") setScope(next);
                }}
              >
                <Toggle value="home">{t("search.unarchived")}</Toggle>
                <Toggle value="archived">{t("common.archived")}</Toggle>
              </ToggleGroup>
            ) : (
              <span className="px-2 mu-caption text-mu-text-secondary">{scopeLabel}</span>
            )}
            <ToggleGroup
              size="sm"
              className="shrink-0"
              aria-label={t("search.mode")}
              value={[mode]}
              onValueChange={([next]: string[]) => next && setMode(next as QuickFindMode)}
              // Keyboard choices keep focus on the switch; clicking a mode focuses its field.
              onKeyDownCapture={() => setAutoFocusField(false)}
              onClickCapture={() => setAutoFocusField(true)}
            >
              {(["text", "cel"] as const).map((value) => (
                <Toggle key={value} value={value}>
                  {value === "cel" ? t("search.expression-mode") : t("search.text")}
                </Toggle>
              ))}
            </ToggleGroup>
          </div>
          {mode === "cel" ? (
            <div className="space-y-3 px-5 py-4 sm:px-6">
              <Textarea
                {...fieldProps}
                rows={3}
                className="field-sizing-fixed max-h-[40dvh] resize-y font-mono"
                placeholder={'"work" in tags && !content.contains("GitHub")'}
                aria-label={t("search.expression")}
                aria-describedby={hintId}
              />
              <div className="flex items-end justify-between gap-3">
                <div className="min-w-0 space-y-1 mu-caption text-mu-text-secondary">
                  <p id={hintId}>{t("search.keyboard-hint")}</p>
                  <a
                    href="https://github.com/Tiger-zero/Cat_two/blob/main/filter/README.md#sql-generation-notes"
                    target="_blank"
                    rel="noopener noreferrer"
                    className="inline-block rounded-mu-sm border border-transparent mu-focus underline underline-offset-2 hover:text-mu-text"
                  >
                    {t("search.syntax-help")}
                  </a>
                </div>
                <Button variant="primary" type="submit" size="sm" className="shrink-0">
                  {t("common.search")}
                </Button>
              </div>
            </div>
          ) : (
            <div className="px-5 py-4 sm:px-6">
              <Input
                {...fieldProps}
                prefix={<SearchIcon />}
                placeholder={`${t("common.search")} ${scopeLabel}`}
                aria-label={`${t("common.search")} ${scopeLabel}`}
                suffix={
                  <Tooltip.Root>
                    <Tooltip.Trigger
                      render={
                        <Button type="submit" variant="ghost" size="sm" aria-label={t("common.search")} icon={<CornerDownLeftIcon />} />
                      }
                    />
                    <Tooltip.Popup>{t("common.search")}</Tooltip.Popup>
                  </Tooltip.Root>
                }
              />
            </div>
          )}
          <div className="flex items-center border-t border-mu-line/65 bg-mu-canvas/65 px-5 py-2.5 sm:px-6">
            <Button type="button" variant="ghost" size="sm" icon={<HashIcon />} onClick={() => submitQuery(true)}>
              {t("memo.filters.untagged")}
            </Button>
          </div>
        </form>
      </Dialog.Popup>
    </Dialog.Root>
  );
};

export default QuickFindDialog;
