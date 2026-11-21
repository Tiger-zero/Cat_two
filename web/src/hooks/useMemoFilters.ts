import { useMemo } from "react";
import { type MemoFilter, useMemoFilterContext } from "@/contexts/MemoFilterContext";
import { type MemoTimeBasis, useView } from "@/contexts/ViewContext";
import { buildTimestampRangeFilter, getLocalDayTimestampRange, getTimeBasisField } from "@/lib/calendar-utils";
import { combineCELFilters } from "@/lib/cel-filter";
import { buildMemoCreatorFilter } from "@/lib/resource-names";

const escapeFilterValue = (value: string): string => JSON.stringify(value);

export interface UseMemoFiltersOptions {
  creatorName?: string;
  includePinned?: boolean;
}

interface BuildMemoFilterOptions {
  creatorName?: string;
  filters: MemoFilter[];
  includePinned: boolean;
  /** Which timestamp a `displayTime` day filter selects on; defaults to creation time. */
  timeBasis?: MemoTimeBasis;
}

export const buildMemoFilter = ({
  creatorName,
  filters,
  includePinned,
  timeBasis = "create_time",
}: BuildMemoFilterOptions): string | undefined => {
  const conditions: string[] = [];

  if (creatorName) {
    const creatorFilter = buildMemoCreatorFilter(creatorName);
    if (creatorFilter) {
      conditions.push(creatorFilter);
    }
  }

  for (const filter of filters) {
    if (filter.factor === "contentSearch") {
      conditions.push(`content.contains(${escapeFilterValue(filter.value)})`);
    } else if (filter.factor === "celSearch") {
      conditions.push(filter.value);
    } else if (filter.factor === "tagSearch") {
      conditions.push(`tag in [${escapeFilterValue(filter.value)}]`);
    } else if (filter.factor === "source" && (filter.value === "web" || filter.value === "unknown")) {
      conditions.push(`creation_channel == ${escapeFilterValue(filter.value)}`);
    } else if (filter.factor === "pinned") {
      if (includePinned) {
        conditions.push(`pinned`);
      }
    } else if (filter.factor === "property.hasLink") {
      conditions.push(`has_link`);
    } else if (filter.factor === "property.hasTaskList") {
      conditions.push(`has_task_list`);
    } else if (filter.factor === "property.hasCode") {
      conditions.push(`has_code`);
    } else if (filter.factor === "property.hasAttachments") {
      conditions.push(`has_attachments`);
    } else if (filter.factor === "property.hasImage") {
      conditions.push(`has_image`);
    } else if (filter.factor === "displayTime") {
      const range = getLocalDayTimestampRange(filter.value);
      if (range) {
        conditions.push(buildTimestampRangeFilter(getTimeBasisField(timeBasis), range));
      }
    }
  }

  return combineCELFilters(...conditions);
};

export const useMemoFilters = (options: UseMemoFiltersOptions = {}): string | undefined => {
  const { creatorName, includePinned = false } = options;

  const { filters } = useMemoFilterContext();
  // The sidebar calendar buckets days by this basis, so a picked day must select on it too.
  const { timeBasis } = useView();

  return useMemo(
    () =>
      buildMemoFilter({
        creatorName,
        filters,
        includePinned,
        timeBasis,
      }),
    [creatorName, filters, includePinned, timeBasis],
  );
};
