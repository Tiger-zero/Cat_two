import { timestampDate } from "@bufbuild/protobuf/wkt";
import { Button, List } from "@usememos/ui";
import { RefreshCwIcon } from "lucide-react";
import { useInstanceStats } from "@/hooks/useInstanceQueries";
import { APP_DATE_TIME_FORMAT, inAppTimeZone } from "@/lib/time-zone";
import { useTranslate } from "@/utils/i18n";
import SettingGroup from "./SettingGroup";
import { SettingList, StatRow } from "./SettingList";
import SettingSection from "./SettingSection";

const renderBytes = (value: bigint | undefined, unknown: string): string => {
  if (value === undefined || value < 0n) return unknown;
  if (value === 0n) return "0 B";
  const bytes = Number(value);
  const units = ["B", "KB", "MB", "GB", "TB"];
  const unit = Math.min(units.length - 1, Math.floor(Math.log(bytes) / Math.log(1024)));
  return `${(bytes / 1024 ** unit).toFixed(unit === 0 ? 0 : 1)} ${units[unit]}`;
};

export default function StorageSection() {
  const t = useTranslate();
  const { data, isLoading, isError, isFetching, refetch } = useInstanceStats();
  const unknown = t("setting.storage.unknown");
  const lastUpdated = data?.generatedTime ? timestampDate(data.generatedTime) : undefined;

  return (
    <SettingSection title={t("setting.storage.label")}>
      {data && (
        <SettingGroup title={t("setting.storage.locations")} description={t("setting.storage.locations-description")}>
          <SettingList>
            <StatRow label={t("setting.storage.data-directory")} value={data.dataDirectory || unknown} />
            <StatRow label={t("setting.storage.notes-path")} value={data.notesPath || unknown} />
            <StatRow label={t("setting.storage.attachments-directory")} value={data.attachmentsDirectory || unknown} />
          </SettingList>
        </SettingGroup>
      )}
      <SettingGroup
        title={t("setting.storage.usage")}
        showSeparator={Boolean(data)}
        actions={
          <>
            {lastUpdated && (
              <time dateTime={lastUpdated.toISOString()} className="mu-caption text-mu-text-secondary">
                {t("setting.storage.last-updated", { time: inAppTimeZone(lastUpdated).format(APP_DATE_TIME_FORMAT) })}
              </time>
            )}
            <Button variant="neutral" size="sm" disabled={isFetching} onClick={() => void refetch()} icon={<RefreshCwIcon />}>
              {t("setting.storage.refresh")}
            </Button>
          </>
        }
      >
        {isError && (
          <p role="alert" className="mu-body text-mu-danger">
            {t("setting.storage.load-error")}
          </p>
        )}
        {isLoading && (
          <SettingList aria-busy>
            <List.Item>
              <span role="status" className="mu-body text-mu-text-secondary">
                {t("setting.storage.loading")}
              </span>
            </List.Item>
          </SettingList>
        )}
        {data && (
          <SettingList aria-busy={isFetching}>
            <StatRow label={t("setting.storage.database-driver")} value={data.database?.driver || unknown} />
            <StatRow label={t("setting.storage.database-size")} value={renderBytes(data.database?.sizeBytes, unknown)} />
            <StatRow label={t("setting.storage.data-directory-size")} value={renderBytes(data.localStorageBytes, unknown)} />
          </SettingList>
        )}
      </SettingGroup>
    </SettingSection>
  );
}
