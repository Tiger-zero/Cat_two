import { timestampDate } from "@bufbuild/protobuf/wkt";
import { Tooltip } from "@usememos/ui";
import { Link } from "react-router-dom";
import { APP_DATE_TIME_FORMAT, inAppTimeZone } from "@/lib/time-zone";
import { cn } from "@/lib/utils";
import { useTranslate } from "@/utils/i18n";
import { MEMO_TIME_CONTROL_CLASSES } from "../constants";
import { useMemoViewContext } from "../MemoViewContext";
import { createMemoNavigationState } from "../navigation";

const MemoTimestamp = ({ isCurrentPage }: { isCurrentPage: boolean }) => {
  const t = useTranslate();
  const { memo, parentPage } = useMemoViewContext();
  if (!memo.createTime) return null;

  const created = timestampDate(memo.createTime);
  const shanghaiTime = inAppTimeZone(created);
  const time = <time dateTime={created.toISOString()}>{shanghaiTime.format("YYYY-MM-DD HH:mm")}</time>;

  return (
    <footer className="flex w-full justify-end mu-caption tabular-nums text-mu-text/60">
      <Tooltip.Root>
        <Tooltip.Trigger
          render={
            isCurrentPage ? (
              <span className="whitespace-nowrap">{time}</span>
            ) : (
              <Link
                to={`/${memo.name}`}
                state={createMemoNavigationState(parentPage)}
                viewTransition
                className={cn(MEMO_TIME_CONTROL_CLASSES, "text-inherit underline-offset-2 hover:underline")}
              >
                {time}
              </Link>
            )
          }
        />
        <Tooltip.Popup align="end" className="flex flex-col items-start gap-0.5 whitespace-nowrap">
          {t("common.created-at")}: {shanghaiTime.format(APP_DATE_TIME_FORMAT)}
        </Tooltip.Popup>
      </Tooltip.Root>
    </footer>
  );
};

export default MemoTimestamp;
