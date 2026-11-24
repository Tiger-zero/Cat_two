import { type Timestamp, timestampDate } from "@bufbuild/protobuf/wkt";
import { useMemo } from "react";
import { type MemoTimeBasis, useView } from "@/contexts/ViewContext";
import useCurrentUser from "@/hooks/useCurrentUser";
import { useAllUserStats, useUserStats } from "@/hooks/useUserQueries";
import { ISO_DATE_FORMAT } from "@/lib/calendar-utils";
import { mergeTagCounts } from "@/lib/tag";
import { inAppTimeZone } from "@/lib/time-zone";
import { State } from "@/types/proto/api/common_pb";
import type { UserStats } from "@/types/proto/api/user_service_pb";
import type { StatisticsData } from "@/types/statistics";

export interface FilteredMemoStats {
  statistics: StatisticsData;
  tags: Record<string, number>;
  loading: boolean;
  summary?: CollectionSummary;
}

export interface CollectionSummary {
  memoCount: number;
  tagCount: number;
  recordedDays: number;
}

export type MemoStatsContext = "home" | "collection" | "archived";

export interface UseFilteredMemoStatsOptions {
  userName?: string;
  context?: MemoStatsContext;
  enabled?: boolean;
  filter?: string;
}

const addDailyCounts = (counts: Record<string, number>, timestamps: readonly Timestamp[]) => {
  for (const timestamp of timestamps) {
    if (!timestamp) continue;
    const day = inAppTimeZone(timestampDate(timestamp)).format(ISO_DATE_FORMAT);
    counts[day] = (counts[day] ?? 0) + 1;
  }
};

const timestampsForBasis = (stats: UserStats, basis: MemoTimeBasis) => {
  const createdArray = stats.memoCreatedTimestamps ?? [];
  const updatedArray = stats.memoUpdatedTimestamps ?? [];
  const wantUpdated = basis === "update_time";
  const oldServerFallback = wantUpdated && updatedArray.length === 0 && createdArray.length > 0;
  if (oldServerFallback) {
    console.warn("UserStats.memo_updated_timestamps not present; falling back to memo_created_timestamps");
  }
  return wantUpdated && !oldServerFallback ? updatedArray : createdArray;
};

export const useFilteredMemoStats = (options: UseFilteredMemoStatsOptions = {}): FilteredMemoStats => {
  const { userName, context, enabled = true, filter } = options;
  const currentUser = useCurrentUser();
  const { timeBasis } = useView();

  const grouped = context === "collection" || context === "archived";
  // Owner Home shares the active per-user statistics used by the editor.
  const { data: userStats, isLoading: isLoadingUserStats } = useUserStats(userName, { enabled: enabled && !grouped, filter });
  // Anonymous collections and archives remain scoped through ListUserStats.
  const allUserStatsRequest =
    context === "collection" ? { state: State.NORMAL, filter } : context === "archived" ? { state: State.ARCHIVED, filter } : {};
  const shouldFetchAllUserStats = context === "collection" || (context === "archived" && !!currentUser?.name);
  const { data: allUserStats, isLoading: isLoadingAllUserStats } = useAllUserStats(allUserStatsRequest, {
    enabled: enabled && shouldFetchAllUserStats,
  });

  const data = useMemo(() => {
    const loading = isLoadingUserStats || isLoadingAllUserStats;
    let activityStats: Record<string, number> = {};
    let tagCount: Record<string, number> = mergeTagCounts();
    let summary: CollectionSummary | undefined;

    if (context === "collection" || context === "archived") {
      const groupedStats = allUserStats ?? [];
      tagCount = mergeTagCounts(...groupedStats.map((stats) => stats.tagCounts));
      const createdActivity: Record<string, number> = timeBasis === "create_time" ? activityStats : {};
      let memoCount = 0;
      for (const stats of groupedStats) {
        addDailyCounts(activityStats, timestampsForBasis(stats, timeBasis));
        // The summary counts writing days even when the calendar is showing updates.
        if (context === "collection" && timeBasis === "update_time") {
          addDailyCounts(createdActivity, stats.memoCreatedTimestamps ?? []);
        }
        memoCount += stats.totalMemoCount;
      }
      if (context === "collection" && allUserStats !== undefined) {
        summary = { memoCount, tagCount: Object.keys(tagCount).length, recordedDays: Object.keys(createdActivity).length };
      }
    } else if (userName && userStats) {
      const { dailyActivity } = userStats;
      if (dailyActivity) {
        activityStats = timeBasis === "update_time" ? dailyActivity.updatedCounts : dailyActivity.createdCounts;
      } else {
        addDailyCounts(activityStats, timestampsForBasis(userStats, timeBasis));
      }
      tagCount = mergeTagCounts(userStats.tagCounts);
      if (context === "home") {
        let createdActivity = dailyActivity?.createdCounts ?? activityStats;
        if (!dailyActivity && timeBasis === "update_time") {
          createdActivity = {};
          addDailyCounts(createdActivity, userStats.memoCreatedTimestamps ?? []);
        }
        summary = {
          memoCount: userStats.totalMemoCount,
          tagCount: Object.keys(tagCount).length,
          recordedDays: Object.keys(createdActivity).length,
        };
      }
    }

    return { statistics: { activityStats, timeBasis }, tags: tagCount, loading, summary };
  }, [context, userName, userStats, allUserStats, isLoadingUserStats, isLoadingAllUserStats, timeBasis]);

  return data;
};
