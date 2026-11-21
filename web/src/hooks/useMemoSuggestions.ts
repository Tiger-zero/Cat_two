import { useMemo } from "react";
import { useMemoFilterContext } from "@/contexts/MemoFilterContext";
import { getMemoSuggestions, type MemoSuggestion } from "@/lib/memo-suggestions";

/** Suggest tags and checklists from the current browsing filters. */
export function useMemoSuggestions(): MemoSuggestion[] {
  const { filters } = useMemoFilterContext();
  return useMemo(() => getMemoSuggestions(filters), [filters]);
}
