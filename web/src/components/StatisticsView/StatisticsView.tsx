import { useState } from "react";
import { MonthCalendar } from "@/components/ActivityCalendar";
import { useMemoFilterContext } from "@/contexts/MemoFilterContext";
import { useDateFilterNavigation } from "@/hooks";
import { getCurrentMonth, parseLocalDate } from "@/lib/calendar-utils";
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
  const dateFilter = filters.find((filter) => filter.factor === "displayTime")?.value;
  const selectedDate = dateFilter && parseLocalDate(dateFilter) ? dateFilter : undefined;
  const selectedMonth = selectedDate?.slice(0, 7);
  const [calendar, setCalendar] = useState(() => ({ selectedDate, visibleMonth: selectedMonth ?? getCurrentMonth() }));

  // A restored day reveals its month. The arrows still browse independently of the active filter.
  if (calendar.selectedDate !== selectedDate) {
    setCalendar({ selectedDate, visibleMonth: selectedMonth ?? calendar.visibleMonth });
  }

  return (
    <div className="@container w-full animate-fade-in">
      <div className="memos-panel group flex w-full flex-col px-px py-2 text-mu-text-secondary @min-[214px]:px-2">
        <MonthNavigator
          visibleMonth={calendar.visibleMonth}
          onMonthChange={(visibleMonth) => setCalendar({ selectedDate, visibleMonth })}
        />

        <div className="w-full animate-scale-in">
          <MonthCalendar
            month={calendar.visibleMonth}
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
