import { Button, List } from "@usememos/ui";
import { useState } from "react";
import ConfirmDialog from "@/components/ConfirmDialog";
import useCurrentUser from "@/hooks/useCurrentUser";
import { useLoginBlocklist } from "@/hooks/useLoginBlocklist";
import { APP_DATE_TIME_FORMAT, inAppTimeZone } from "@/lib/time-zone";
import { showErrorToast, toast } from "@/lib/toast";
import { useTranslate } from "@/utils/i18n";
import SettingGroup from "./SettingGroup";
import { SettingList } from "./SettingList";
import SettingSection from "./SettingSection";

export default function SecuritySection() {
  const t = useTranslate();
  const account = useCurrentUser()?.name;
  const { query, unblock } = useLoginBlocklist(account);
  const [unblockIP, setUnblockIP] = useState<string>();
  const entries = query.data?.pages.flatMap((page) => page.entries) ?? [];

  const handleUnblock = async () => {
    if (!unblockIP) return;
    try {
      await unblock.mutateAsync(unblockIP);
      toast.add({ title: t("setting.security.unblocked", { ip: unblockIP }), type: "success" });
    } catch (error) {
      showErrorToast(t("setting.security.unblock-error"));
      throw error;
    }
  };

  return (
    <SettingSection title={t("setting.security.label")}>
      <SettingGroup title={t("setting.security.blocklist")} description={t("setting.security.description")}>
        {query.isPending && (
          <p role="status" className="mu-body text-mu-text-secondary">
            {t("setting.security.loading")}
          </p>
        )}
        {query.isError && (
          <div className="flex items-center justify-between gap-3">
            <p role="alert" className="mu-body text-mu-text-secondary">
              {t("setting.security.load-error")}
            </p>
            <Button
              variant="neutral"
              size="sm"
              disabled={query.isFetching}
              onClick={() => void (query.isFetchNextPageError ? query.fetchNextPage() : query.refetch())}
            >
              {t("common.retry")}
            </Button>
          </div>
        )}
        {query.isSuccess && entries.length === 0 && (
          <p role="status" className="mu-body text-mu-text-secondary">
            {t("setting.security.empty")}
          </p>
        )}
        {entries.length > 0 && (
          <SettingList aria-busy={query.isFetching}>
            {entries.map(({ ip, blockedAt }) => (
              <List.Item key={ip} className="flex items-center justify-between gap-3">
                <div className="flex min-w-0 flex-1 flex-col gap-0.5">
                  <span className="mu-label break-all font-mono text-mu-text">{ip}</span>
                  <time dateTime={blockedAt} className="mu-caption text-mu-text-secondary">
                    {t("setting.security.blocked-at", { time: inAppTimeZone(new Date(blockedAt)).format(APP_DATE_TIME_FORMAT) })}
                  </time>
                </div>
                <Button
                  variant="ghost"
                  size="sm"
                  className="shrink-0"
                  disabled={unblock.isPending}
                  aria-label={t("setting.security.unblock-ip", { ip })}
                  onClick={() => setUnblockIP(ip)}
                >
                  {t("setting.security.unblock")}
                </Button>
              </List.Item>
            ))}
          </SettingList>
        )}
        {query.hasNextPage && !query.isFetchNextPageError && (
          <Button
            variant="neutral"
            size="sm"
            className="self-start"
            disabled={query.isFetching || unblock.isPending}
            onClick={() => void query.fetchNextPage()}
          >
            {t("memo.load-more")}
          </Button>
        )}
      </SettingGroup>
      <ConfirmDialog
        open={unblockIP !== undefined}
        onOpenChange={(open) => !open && setUnblockIP(undefined)}
        title={t("setting.security.unblock-title", { ip: unblockIP })}
        description={t("setting.security.unblock-description")}
        cancelLabel={t("common.cancel")}
        confirmLabel={t("setting.security.unblock")}
        onConfirm={handleUnblock}
      />
    </SettingSection>
  );
}
