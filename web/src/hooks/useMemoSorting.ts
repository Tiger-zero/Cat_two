import { timestampDate } from "@bufbuild/protobuf/wkt";
import dayjs from "dayjs";
import { useMemo } from "react";
import { type MemoTimeBasis, useView } from "@/contexts/ViewContext";
import type { Memo } from "@/types/proto/api/memo_service_pb";

export interface UseMemoSortingOptions {
  pinnedFirst?: boolean;
}

export interface UseMemoSortingResult {
  listSort: (memos: Memo[]) => Memo[];
  orderBy: string;
}

/** The timestamp a memo is ordered and bucketed by under `timeBasis`. */
export const getMemoSortTime = (memo: Memo, timeBasis: MemoTimeBasis): Date | undefined => {
  const timestamp = timeBasis === "update_time" ? memo.updateTime : memo.createTime;
  return timestamp ? timestampDate(timestamp) : undefined;
};

// Moves the memo with the given name to the front of the list, above pinned
// memos, so a freshly created memo is immediately visible. Returns the input
// unchanged when the name is null, absent, or already first.
export const hoistMemoToFront = (memos: Memo[], name: string | null): Memo[] => {
  if (!name) return memos;
  const index = memos.findIndex((memo) => memo.name === name);
  if (index <= 0) return memos;
  const reordered = memos.slice();
  const [memo] = reordered.splice(index, 1);
  reordered.unshift(memo);
  return reordered;
};

export const useMemoSorting = (options: UseMemoSortingOptions = {}): UseMemoSortingResult => {
  const { pinnedFirst = false } = options;
  const { orderByTimeAsc, timeBasis } = useView();

  // Generate orderBy string for API
  const orderBy = useMemo(() => {
    const timeOrder = orderByTimeAsc ? `${timeBasis} asc` : `${timeBasis} desc`;
    return pinnedFirst ? `pinned desc, ${timeOrder}` : timeOrder;
  }, [pinnedFirst, orderByTimeAsc, timeBasis]);

  // Generate listSort function for client-side sorting
  const listSort = useMemo(() => {
    return (memos: Memo[]): Memo[] => {
      return [...memos].sort((a, b) => {
        // First, sort by pinned status if enabled
        if (pinnedFirst && a.pinned !== b.pinned) {
          return b.pinned ? 1 : -1;
        }

        // Then sort by the selected time field.
        const aTime = getMemoSortTime(a, timeBasis);
        const bTime = getMemoSortTime(b, timeBasis);
        return orderByTimeAsc ? dayjs(aTime).unix() - dayjs(bTime).unix() : dayjs(bTime).unix() - dayjs(aTime).unix();
      });
    };
  }, [pinnedFirst, orderByTimeAsc, timeBasis]);

  return { listSort, orderBy };
};
