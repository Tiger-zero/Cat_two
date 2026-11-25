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
import { ROUTES } from "@/router/routes";
import { useTranslate } from "@/utils/i18n";

type QuickFindMode = "text" | "cel";

export const buildQuickFindFilters = (query: string, currentFilters: MemoFilter[], mode: QuickFindMode, untagged = false): MemoFilter[] => {
  const filters = currentFilters.filter((filter) => !isSearchFilter(filter));
  const trimmed = query.trim();
  if (trimmed) {
    if (mode === "cel") {
      filters.push({ factor: "celSearch", value: trimmed });
    } else {
      for (const value of new Set(trimmed.split(/\s+/))) {
        filters.push({ factor: "contentSearch", value });
      }
    }
  }
  return untagged ? replaceFiltersByFactor(filters, "untagged", [{ factor: "untagged", value: "" }]) : filters;
};

const readQuickFindQuery = (filters: MemoFilter[]): { query: string; mode: QuickFindMode } => {
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

const QuickFindDialog = () => {
  const t = useTranslate();
  const { pathname } = useLocation();
  const navigate = useNavigate();
  const onHome = pathname === ROUTES.HOME;
  const { filters, setFilters } = useMemoFilterContext();
  const { quickFindOpen, setQuickFindOpen } = useAppSidebar();
  const [query, setQuery] = useState("");
  const [mode, setMode] = useState<QuickFindMode>("text");
  const [autoFocusField, setAutoFocusField] = useState(true);
  const fieldRef = useRef<HTMLInputElement | HTMLTextAreaElement>(null);
  const previousMode = useRef(mode);
  const hintId = useId();

  useEffect(() => {
    if (!quickFindOpen) return;
    setAutoFocusField(true);
    const active = readQuickFindQuery(onHome ? filters : []);
    setMode(active.mode);
    setQuery(active.query);
  }, [filters, onHome, quickFindOpen]);

  useLayoutEffect(() => {
    const changedMode = previousMode.current !== mode;
    previousMode.current = mode;
    if (quickFindOpen && changedMode && autoFocusField) fieldRef.current?.focus();
  }, [autoFocusField, mode, quickFindOpen]);

  const submitQuery = (untagged = false) => {
    const nextFilters = buildQuickFindFilters(query, onHome ? filters : [], mode, untagged);
    if (onHome) setFilters(nextFilters);
    else navigate(`${ROUTES.HOME}${getFilterSearch(nextFilters)}`);
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
        <header className="flex items-center gap-3 px-5 pt-4 pb-1 sm:px-6">
          <Dialog.Title className="me-auto mu-title">{t("common.search")}</Dialog.Title>
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
          <Dialog.Close render={<Button variant="ghost" size="sm" icon={<XIcon />} aria-label={t("common.close")} />} />
        </header>
        <Dialog.Description className="mu-visually-hidden">{t("memo-feed.search")}</Dialog.Description>
        <form onSubmit={handleSubmit}>
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
                placeholder={t("memo-feed.search")}
                aria-label={t("memo-feed.search")}
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
