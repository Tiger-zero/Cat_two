import { Button, Tooltip } from "@usememos/ui";
import { isEqual } from "lodash-es";
import {
  BookmarkIcon,
  CalendarIcon,
  CheckCircleIcon,
  CodeIcon,
  HashIcon,
  ImageIcon,
  LinkIcon,
  ListFilterIcon,
  type LucideIcon,
  PaperclipIcon,
  SearchIcon,
  XIcon,
} from "lucide-react";
import { useAppSidebar } from "@/contexts/AppSidebarContext";
import { type FilterFactor, getMemoFilterKey, type MemoFilter, useMemoFilterContext } from "@/contexts/MemoFilterContext";
import i18n from "@/i18n";
import { parseLocalDate } from "@/lib/calendar-utils";
import { APP_TIME_ZONE } from "@/lib/time-zone";
import { cn } from "@/lib/utils";
import { useTranslate } from "@/utils/i18n";

interface FilterConfig {
  icon: LucideIcon;
  getLabel: (value: string, t: ReturnType<typeof useTranslate>) => string;
  /** The chip shows a typed expression: monospace, wider, and clicking it reopens Quick Find to edit. */
  editableQuery?: boolean;
}

const FILTER_CONFIGS: Record<FilterFactor, FilterConfig> = {
  tagSearch: {
    icon: HashIcon,
    getLabel: (value) => value,
  },
  untagged: {
    icon: HashIcon,
    getLabel: (_, t) => t("memo.filters.untagged"),
  },
  source: {
    icon: ListFilterIcon,
    getLabel: (value, t) => t(value === "web" ? "memo-feed.source-web" : "memo-feed.source-unknown"),
  },
  contentSearch: {
    icon: SearchIcon,
    getLabel: (value) => value,
  },
  celSearch: {
    icon: SearchIcon,
    getLabel: (value) => value,
    editableQuery: true,
  },
  displayTime: {
    icon: CalendarIcon,
    getLabel: (value) => {
      const date = parseLocalDate(value);
      return date?.toLocaleDateString(i18n.language, { timeZone: APP_TIME_ZONE, month: "short", day: "numeric", year: "numeric" }) ?? value;
    },
  },
  pinned: {
    icon: BookmarkIcon,
    getLabel: (value) => value,
  },
  "property.hasLink": {
    icon: LinkIcon,
    getLabel: (_, t) => t("memo.filters.has-link"),
  },
  "property.hasTaskList": {
    icon: CheckCircleIcon,
    getLabel: (_, t) => t("memo.filters.has-task-list"),
  },
  "property.hasCode": {
    icon: CodeIcon,
    getLabel: (_, t) => t("memo.filters.has-code"),
  },
  "property.hasAttachments": {
    icon: PaperclipIcon,
    getLabel: (_, t) => t("memo.filters.has-attachments"),
  },
  "property.hasImage": {
    icon: ImageIcon,
    getLabel: (_, t) => t("memo.filters.has-image"),
  },
};

interface FilterChipProps {
  icon?: LucideIcon;
  label: string;
  onRemove: () => void;
  /** When set, the label is a button that edits the filter; the label is also shown in full as a tooltip. */
  onEdit?: { label: string; onClick: () => void };
}

/** Presents each active filter with consistent labeling and a remove action. */
const FilterChip = ({ icon: Icon, label, onRemove, onEdit }: FilterChipProps) => {
  const t = useTranslate();
  const body = (
    <>
      {Icon && <Icon className="mu-icon-sm text-mu-text-secondary shrink-0" />}
      <span className={cn("text-mu-text/80 truncate", onEdit ? "font-mono mu-caption max-w-64" : "font-medium max-w-32")}>{label}</span>
    </>
  );
  return (
    <div
      className={cn(
        "group inline-flex min-w-0 max-w-full items-center gap-1.5 h-7 px-2.5",
        "bg-mu-hover/50 hover:bg-mu-hover border border-mu-line/50 rounded-full mu-body transition-all duration-200",
      )}
    >
      {onEdit ? (
        <button
          type="button"
          onClick={onEdit.onClick}
          title={label}
          aria-label={onEdit.label}
          className="flex min-w-0 items-center gap-1.5"
        >
          {body}
        </button>
      ) : (
        body
      )}
      <span className="ml-0.5 -mr-1">
        <Tooltip.Root>
          <Tooltip.Trigger
            render={<Button variant="ghost" size="xs" onClick={onRemove} aria-label={t("memo.filters.remove")} icon={<XIcon />} />}
          />
          <Tooltip.Popup>{t("memo.filters.remove")}</Tooltip.Popup>
        </Tooltip.Root>
      </span>
    </div>
  );
};

const MemoFilters = ({ className }: { className?: string }) => {
  const t = useTranslate();
  const { filters, removeFilter } = useMemoFilterContext();
  const { setQuickFindOpen } = useAppSidebar();

  const handleRemoveFilter = (filter: MemoFilter) => {
    removeFilter((f: MemoFilter) => isEqual(f, filter));
  };

  const getFilterDisplayText = (filter: MemoFilter): string => {
    const config = FILTER_CONFIGS[filter.factor];
    if (!config) {
      return filter.value || filter.factor;
    }
    return config.getLabel(filter.value, t);
  };

  if (filters.length === 0) {
    return null;
  }

  return (
    <div className={cn("w-full flex flex-row justify-start items-center flex-wrap gap-2", className)}>
      {filters.map((filter) => (
        <FilterChip
          key={getMemoFilterKey(filter)}
          icon={FILTER_CONFIGS[filter.factor]?.icon}
          label={getFilterDisplayText(filter)}
          onRemove={() => handleRemoveFilter(filter)}
          onEdit={
            FILTER_CONFIGS[filter.factor]?.editableQuery
              ? { label: t("search.edit-query"), onClick: () => setQuickFindOpen(true) }
              : undefined
          }
        />
      ))}
    </div>
  );
};

MemoFilters.displayName = "MemoFilters";

export default MemoFilters;
