import { useState } from "react";
import { MonthCalendar } from "@/components/ActivityCalendar";
import { useMemoFilterContext } from "@/contexts/MemoFilterContext";
import { useDateFilterNavigation } from "@/hooks";
import { getCurrentMonth } from "@/lib/calendar-utils";
import type { StatisticsData } from "@/types/statistics";
import { MonthNavigator } from "./MonthNavigator";

interface Props {
  statisticsData: StatisticsData;
  onDateSelect?: () => void;
}

const StatisticsView = (props: Props) => {
  const { statisticsData } = props;
  const { activityStats, timeBasis } = statisticsData;
  const { filters } = useMemoFilterContext();
  const navigateToDateFilter = useDateFilterNavigation();
  const [visibleMonthString, setVisibleMonthString] = useState(getCurrentMonth);
  const selectedDate = filters.find((filter) => filter.factor === "displayTime")?.value;

  return (
    <div className="@container w-full animate-fade-in">
      <div className="memos-panel group flex w-full flex-col px-px py-2 text-mu-text-secondary @min-[214px]:px-2">
        <MonthNavigator visibleMonth={visibleMonthString} onMonthChange={setVisibleMonthString} />

        <div className="w-full animate-scale-in">
          <MonthCalendar
            month={visibleMonthString}
            data={activityStats}
            selectedDate={selectedDate}
            onClick={(date) => {
              navigateToDateFilter(date);
              props.onDateSelect?.();
            }}
            timeBasis={timeBasis}
          />
        </div>
      </div>
    </div>
  );
};

export default StatisticsView;
