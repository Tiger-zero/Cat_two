import { uniqBy } from "lodash-es";
import type { LucideIcon } from "lucide-react";
import type { MemoFilter } from "@/contexts/MemoFilterContext";
import { getContextSuggestionCandidates } from "./context-suggestions";
import { getTagSuggestionCandidates } from "./tag-suggestions";

export type SuggestionSource = "selection" | "search";

/** Shared presentation contract; the bar does not interpret suggestion payloads. */
export interface SuggestionItem {
  id: string;
  label: string;
  accessibleLabel?: string;
  icon?: LucideIcon;
}

/** Add future suggestion kinds to this discriminated union, with their own payloads. */
export type MemoSuggestion = {
  id: string;
  source: SuggestionSource;
} & ({ kind: "tag"; tag: string } | { kind: "checklist" });

export const MAX_MEMO_SUGGESTIONS = 3;

const sourcePriority: Record<SuggestionSource, number> = { selection: 0, search: 1 };

/** Merge providers by context priority and retain the strongest source for each id. */
export function getMemoSuggestions(filters: MemoFilter[]): MemoSuggestion[] {
  const candidates: MemoSuggestion[] = getTagSuggestionCandidates(filters).map(({ tag, source }) => ({
    id: `tag:${tag}`,
    kind: "tag",
    source,
    tag,
  }));
  candidates.push(...getContextSuggestionCandidates(filters));
  candidates.sort((left, right) => sourcePriority[left.source] - sourcePriority[right.source]);
  return uniqBy(candidates, "id");
}
