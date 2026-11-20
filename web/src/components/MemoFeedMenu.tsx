import { Button, Menu } from "@usememos/ui";
import { ArrowDownUpIcon, CheckCircle2Icon, ChevronDownIcon, DownloadIcon, ListFilterIcon, SearchIcon } from "lucide-react";
import { useAppSidebar } from "@/contexts/AppSidebarContext";
import { replaceFiltersByFactor, useMemoFilterContext } from "@/contexts/MemoFilterContext";
import { type MemoTimeBasis, useView } from "@/contexts/ViewContext";
import { MAX_SELECTED_MEMO_EXPORT } from "@/lib/memo-export";
import { useTranslate } from "@/utils/i18n";

interface Props {
  canExport: boolean;
  selectedCount?: number;
  exporting: boolean;
  onBeginSelection: () => void;
  onEndSelection: () => void;
  onExport: () => void;
}

const SORT_ORDERS = [
  { timeBasis: "create_time", ascending: false },
  { timeBasis: "create_time", ascending: true },
  { timeBasis: "update_time", ascending: false },
  { timeBasis: "update_time", ascending: true },
] as const;

const sortValue = (timeBasis: MemoTimeBasis, ascending: boolean) => `${timeBasis}-${ascending ? "asc" : "desc"}`;

function MemoFeedMenu({ canExport, selectedCount, exporting, onBeginSelection, onEndSelection, onExport }: Props) {
  const t = useTranslate();
  const { setQuickFindOpen } = useAppSidebar();
  const { filters, setFilters } = useMemoFilterContext();
  const { timeBasis, orderByTimeAsc, setTimeBasis, setOrderByTimeAsc } = useView();
  const selecting = selectedCount !== undefined;
  const orderValue = sortValue(timeBasis, orderByTimeAsc);
  const source = filters.find((filter) => filter.factor === "source")?.value ?? "all";
  const sortLabel = (basis: MemoTimeBasis, ascending: boolean) =>
    t("memo-feed.sort-detail", {
      field: t(basis === "create_time" ? "common.created-at" : "common.last-updated-at"),
      direction: t(ascending ? "memo.oldest-first" : "memo.newest-first"),
    });
  const exportLabel = selecting ? t("memo-feed.export-selected") : t("memo-feed.export-all");
  const sourceOptions = [
    { value: "all", label: t("common.all") },
    { value: "web", label: t("memo-feed.source-web") },
    { value: "unknown", label: t("memo-feed.source-unknown") },
  ];

  return (
    <div className="mb-3 flex w-full flex-col gap-2">
      <div className="flex min-w-0 items-center justify-between gap-2">
        <h1 className="min-w-0">
          <Menu.Root>
            <Menu.Trigger
              render={
                <Button variant="ghost" iconEnd={<ChevronDownIcon />} className="max-w-full px-2">
                  <span className="mu-heading">{t("memo-feed.title")}</span>
                </Button>
              }
            />
            <Menu.Popup align="start" className="w-56">
              <Menu.SubmenuRoot>
                <Menu.SubmenuTrigger icon={<ArrowDownUpIcon />} aria-label={t("memo-feed.sort")}>
                  <span className="block">{t("memo-feed.sort")}</span>
                  <span className="mu-caption block text-mu-text-secondary">{sortLabel(timeBasis, orderByTimeAsc)}</span>
                </Menu.SubmenuTrigger>
                <Menu.Popup className="w-56">
                  <Menu.RadioGroup
                    value={orderValue}
                    onValueChange={(value) => {
                      const next = SORT_ORDERS.find((option) => sortValue(option.timeBasis, option.ascending) === value);
                      if (!next) return;
                      setTimeBasis(next.timeBasis);
                      setOrderByTimeAsc(next.ascending);
                    }}
                  >
                    {SORT_ORDERS.map(({ timeBasis, ascending }) => (
                      <Menu.RadioItem key={sortValue(timeBasis, ascending)} value={sortValue(timeBasis, ascending)} closeOnClick>
                        {sortLabel(timeBasis, ascending)}
                      </Menu.RadioItem>
                    ))}
                  </Menu.RadioGroup>
                </Menu.Popup>
              </Menu.SubmenuRoot>
              <Menu.SubmenuRoot>
                <Menu.SubmenuTrigger icon={<ListFilterIcon />} aria-label={t("memo-feed.source")}>
                  <span className="block">{t("memo-feed.source")}</span>
                  <span className="mu-caption block text-mu-text-secondary">
                    {sourceOptions.find((option) => option.value === source)?.label}
                  </span>
                </Menu.SubmenuTrigger>
                <Menu.Popup>
                  <Menu.RadioGroup
                    value={source}
                    onValueChange={(value) =>
                      setFilters(replaceFiltersByFactor(filters, "source", value === "all" ? [] : [{ factor: "source", value }]))
                    }
                  >
                    {sourceOptions.map(({ value, label }) => (
                      <Menu.RadioItem key={value} value={value} closeOnClick>
                        {label}
                      </Menu.RadioItem>
                    ))}
                  </Menu.RadioGroup>
                </Menu.Popup>
              </Menu.SubmenuRoot>
              {canExport && (
                <>
                  <Menu.Separator />
                  <Menu.Item icon={<CheckCircle2Icon />} onClick={selecting ? onEndSelection : onBeginSelection}>
                    {selecting ? t("memo-feed.end-selection") : t("memo-feed.select")}
                  </Menu.Item>
                  <Menu.Item icon={<DownloadIcon />} disabled={exporting || selectedCount === 0} onClick={onExport}>
                    {exportLabel}
                  </Menu.Item>
                </>
              )}
            </Menu.Popup>
          </Menu.Root>
        </h1>
        <Button variant="neutral" icon={<SearchIcon />} aria-label={t("memo-feed.search")} onClick={() => setQuickFindOpen(true)}>
          <span className="hidden sm:inline">{t("memo-feed.search")}</span>
        </Button>
      </div>
      {selecting && (
        <div className="memos-panel flex min-w-0 flex-wrap items-center justify-between gap-2 p-2">
          <span role="status" className="mu-body text-mu-text-secondary">
            {t("memo-feed.selected-count", { count: selectedCount })}
            {selectedCount === MAX_SELECTED_MEMO_EXPORT && ` · ${t("memo-feed.selection-limit")}`}
          </span>
          <div className="flex flex-wrap items-center gap-2">
            <Button variant="ghost" onClick={onEndSelection}>
              {t("common.cancel")}
            </Button>
            <Button variant="primary" icon={<DownloadIcon />} disabled={exporting || selectedCount === 0} onClick={onExport}>
              {exportLabel}
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}

export default MemoFeedMenu;
