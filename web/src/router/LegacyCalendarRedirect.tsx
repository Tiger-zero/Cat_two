import { Navigate, useLocation, useParams } from "react-router-dom";
import { parseFilterQuery, replaceFiltersByFactor, stringifyFilters } from "@/contexts/MemoFilterContext";
import { parseLocalDate } from "@/lib/calendar-utils";
import { personalFeedSearch, ROUTES } from "./routes";

export const LegacyCalendarRedirect = () => {
  const { spaceUid, year, month, day } = useParams();
  const location = useLocation();
  if (spaceUid?.includes("/")) return <Navigate to="/404" replace />;

  const { hash } = location;
  const pathname = ROUTES.HOME;
  const search = personalFeedSearch(location.search);

  // A selected day on an old link becomes the same filter as a pick in the sidebar calendar.
  const date = year && month && day && `${year}-${month.padStart(2, "0")}-${day.padStart(2, "0")}`;
  if (!date || !parseLocalDate(date)) return <Navigate to={{ pathname, search, hash }} replace />;

  const params = new URLSearchParams(search);
  const filters = parseFilterQuery(params.get("filter"));
  params.set("filter", stringifyFilters(replaceFiltersByFactor(filters, "displayTime", [{ factor: "displayTime", value: date }])));
  return <Navigate to={{ pathname, search: `?${params}`, hash }} replace />;
};
