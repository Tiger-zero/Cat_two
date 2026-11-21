import { useCallback } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { replaceFiltersByFactor, stringifyFilters, useMemoFilterContext } from "@/contexts/MemoFilterContext";

export const useDateFilterNavigation = (destination?: string) => {
  const navigate = useNavigate();
  const location = useLocation();
  const { filters, setFilters } = useMemoFilterContext();

  const navigateToDateFilter = useCallback(
    (date: string) => {
      const nextFilters = replaceFiltersByFactor(filters, "displayTime", [{ factor: "displayTime", value: date }]);
      const nextSearchParams = new URLSearchParams(destination ? undefined : location.search);
      nextSearchParams.set("filter", stringifyFilters(nextFilters));
      if (!destination) setFilters(nextFilters);
      navigate({ pathname: destination ?? location.pathname, search: nextSearchParams.toString() });
    },
    [destination, filters, location.pathname, location.search, navigate, setFilters],
  );

  return navigateToDateFilter;
};
