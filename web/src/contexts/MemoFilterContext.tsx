import { uniqBy } from "lodash-es";
import { createContext, type ReactNode, useCallback, useContext, useEffect, useMemo, useRef, useState } from "react";
import { useLocation, useSearchParams } from "react-router-dom";

export type FilterFactor =
  | "tagSearch"
  | "untagged"
  | "source"
  | "contentSearch"
  | "celSearch"
  | "displayTime"
  | "pinned"
  | "property.hasLink"
  | "property.hasTaskList"
  | "property.hasCode"
  | "property.hasAttachments"
  | "property.hasImage";

export interface MemoFilter {
  factor: FilterFactor;
  value: string;
}

export const getMemoFilterKey = (filter: MemoFilter): string => `${filter.factor}:${filter.value}`;

/** The most recently applied tag mode wins when a URL or filter change contains both kinds. */
const normalizeTagFilters = (filters: MemoFilter[]): MemoFilter[] => {
  const last = filters.findLastIndex((filter) => filter.factor === "tagSearch" || filter.factor === "untagged");
  if (last === -1) return filters;
  if (filters[last].factor === "untagged") {
    return filters.filter((filter, index) => (filter.factor !== "untagged" && filter.factor !== "tagSearch") || index === last);
  }
  return filters.filter((filter) => filter.factor !== "untagged");
};

export const parseFilterQuery = (query: string | null): MemoFilter[] => {
  if (!query) return [];
  try {
    const filters = query.split(",").flatMap((filterStr) => {
      const separatorIndex = filterStr.indexOf(":");
      const factor = separatorIndex === -1 ? filterStr : filterStr.slice(0, separatorIndex);
      if (factor === "property.hasLocation" || factor === "visibility") return [];
      const value = separatorIndex === -1 ? "" : filterStr.slice(separatorIndex + 1);
      return [
        {
          factor: factor as FilterFactor,
          value: factor === "untagged" ? "" : decodeURIComponent(value || ""),
        },
      ];
    });
    return normalizeTagFilters(filters);
  } catch {
    return [];
  }
};

export const stringifyFilters = (filters: MemoFilter[]): string => {
  return filters.map((filter) => `${filter.factor}:${encodeURIComponent(filter.value)}`).join(",");
};

/** The `?filter=` search string that carries these filters in a URL, or "" when there are none. */
export const getFilterSearch = (filters: MemoFilter[]): string => {
  const filterQuery = stringifyFilters(filters);
  return filterQuery ? `?${new URLSearchParams({ filter: filterQuery })}` : "";
};

/** Search filters carry the user's query itself (plain words or a CEL expression), as opposed to facets. */
export const isSearchFilter = (filter: MemoFilter): boolean => filter.factor === "contentSearch" || filter.factor === "celSearch";

export const replaceFiltersByFactor = (filters: MemoFilter[], factor: FilterFactor, replacements: MemoFilter[]): MemoFilter[] =>
  normalizeTagFilters([...filters.filter((filter) => filter.factor !== factor), ...replacements]);

interface MemoFilterContextValue {
  filters: MemoFilter[];
  hasActiveFilters: boolean;
  getFiltersByFactor: (factor: FilterFactor) => MemoFilter[];
  setFilters: (filters: MemoFilter[]) => void;
  addFilter: (filter: MemoFilter) => void;
  removeFilter: (predicate: (f: MemoFilter) => boolean) => void;
  removeFiltersByFactor: (factor: FilterFactor) => void;
  clearAllFilters: () => void;
  hasFilter: (filter: MemoFilter) => boolean;
}

const MemoFilterContext = createContext<MemoFilterContextValue | null>(null);

export function MemoFilterProvider({ children }: { children: ReactNode }) {
  const [searchParams, setSearchParams] = useSearchParams();
  const locationStateRef = useRef(useLocation().state);
  const skipStoreSyncRef = useRef(false);
  const lastSyncedUrlRef = useRef("");
  const lastSyncedStoreRef = useRef("");

  // Initialize from URL
  const [filters, setFiltersState] = useState<MemoFilter[]>(() => {
    return parseFilterQuery(searchParams.get("filter"));
  });

  // Sync URL to state when URL changes externally
  useEffect(() => {
    const filterParam = searchParams.get("filter") || "";
    if (filterParam !== lastSyncedUrlRef.current) {
      // The store still contains the previous page’s filters in this commit.
      // Do not write them back over the destination URL before state catches up.
      skipStoreSyncRef.current = true;
      lastSyncedUrlRef.current = filterParam;
      const newFilters = parseFilterQuery(filterParam);
      setFiltersState(newFilters);
      lastSyncedStoreRef.current = stringifyFilters(newFilters);
    }
  }, [searchParams]);

  // Sync state to URL when state changes
  useEffect(() => {
    if (skipStoreSyncRef.current) {
      skipStoreSyncRef.current = false;
      return;
    }
    const storeString = stringifyFilters(filters);
    if (storeString !== lastSyncedStoreRef.current && storeString !== lastSyncedUrlRef.current) {
      lastSyncedStoreRef.current = storeString;
      const newParams = new URLSearchParams(searchParams);
      if (filters.length > 0) {
        newParams.set("filter", storeString);
      } else {
        newParams.delete("filter");
      }
      setSearchParams(newParams, { replace: true, state: locationStateRef.current });
      lastSyncedUrlRef.current = filters.length > 0 ? storeString : "";
    }
  }, [filters, searchParams, setSearchParams]);

  const getFiltersByFactor = useCallback((factor: FilterFactor) => filters.filter((f) => f.factor === factor), [filters]);

  const setFilters = useCallback((newFilters: MemoFilter[]) => {
    setFiltersState(normalizeTagFilters(newFilters));
  }, []);

  const addFilter = useCallback((filter: MemoFilter) => {
    setFiltersState((prev) => uniqBy(normalizeTagFilters([...prev, filter]), getMemoFilterKey));
  }, []);

  const removeFilter = useCallback((predicate: (f: MemoFilter) => boolean) => {
    setFiltersState((prev) => prev.filter((f) => !predicate(f)));
  }, []);

  const removeFiltersByFactor = useCallback((factor: FilterFactor) => {
    setFiltersState((prev) => prev.filter((f) => f.factor !== factor));
  }, []);

  const clearAllFilters = useCallback(() => {
    setFiltersState([]);
  }, []);

  const hasFilter = useCallback((filter: MemoFilter) => filters.some((f) => getMemoFilterKey(f) === getMemoFilterKey(filter)), [filters]);

  const hasActiveFilters = filters.length > 0;
  const value = useMemo(
    () => ({
      filters,
      hasActiveFilters,
      getFiltersByFactor,
      setFilters,
      addFilter,
      removeFilter,
      removeFiltersByFactor,
      clearAllFilters,
      hasFilter,
    }),
    [filters, hasActiveFilters, getFiltersByFactor, setFilters, addFilter, removeFilter, removeFiltersByFactor, clearAllFilters, hasFilter],
  );

  return <MemoFilterContext.Provider value={value}>{children}</MemoFilterContext.Provider>;
}

export function useMemoFilterContext() {
  const context = useContext(MemoFilterContext);
  if (!context) {
    throw new Error("useMemoFilterContext must be used within MemoFilterProvider");
  }
  return context;
}

// Alias for backwards compatibility during migration
export const useMemoFilter = useMemoFilterContext;

const NO_TERMS: string[] = [];

/**
 * The words of the active plain-text search, in the order the user typed them. Safe outside a
 * provider (share previews, tests), where there is no search and therefore nothing to match.
 */
export function useContentSearchTerms(): string[] {
  const filters = useContext(MemoFilterContext)?.filters;
  return useMemo(() => {
    const terms = (filters ?? []).filter((filter) => filter.factor === "contentSearch").map((filter) => filter.value);
    return terms.length > 0 ? terms : NO_TERMS;
  }, [filters]);
}
