import { Button, Input, Switch } from "@usememos/ui";
import { type FormEvent, useState } from "react";
import { toast } from "@/lib/toast";
import { type Translations, useTranslate } from "@/utils/i18n";
import SettingGroup from "./SettingGroup";
import { SettingList, SettingListItem } from "./SettingList";
import SettingSection from "./SettingSection";
import useAdminSettingsRequest from "./useAdminSettingsRequest";

interface NetworkSettings {
  enabled: boolean;
  hasProxyUrl: boolean;
  source: "settings" | "";
}

interface NetworkUpdate {
  enabled: boolean;
  proxyUrl?: string;
}

const proxyProtocols = new Set(["http:", "https:", "socks5:", "socks5h:"]);

function isProxyUrl(value: string) {
  try {
    const url = new URL(value);
    return url.hostname !== "" && proxyProtocols.has(url.protocol);
  } catch {
    return false;
  }
}

export default function NetworkSection() {
  const t = useTranslate();
  const { query, saving, save } = useAdminSettingsRequest<NetworkSettings, NetworkUpdate>("/internal/networkSettings");
  const [enabledEdit, setEnabledEdit] = useState<boolean>();
  const [proxyUrl, setProxyUrl] = useState("");
  const [error, setError] = useState<Translations>();
  const settings = query.data;
  const enabled = enabledEdit ?? settings?.enabled ?? false;

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const address = proxyUrl.trim();
    if (address ? !isProxyUrl(address) : enabled && !settings?.hasProxyUrl) {
      setError("setting.network.invalid-url");
      return;
    }
    setError(undefined);
    try {
      await save({ enabled, proxyUrl: address || undefined });
      setEnabledEdit(undefined);
      setProxyUrl("");
      toast.add({ type: "success", title: t("setting.network.saved") });
    } catch {
      setError("setting.network.save-error");
    }
  };

  return (
    <SettingSection title={t("setting.network.label")}>
      <SettingGroup title={t("setting.network.proxy")} description={t("setting.network.description")}>
        {query.isPending && (
          <p role="status" className="mu-body text-mu-text-secondary">
            {t("setting.network.loading")}
          </p>
        )}
        {query.isError && (
          <div className="flex items-center justify-between gap-3">
            <p role="alert" className="mu-body text-mu-text-secondary">
              {t("setting.network.load-error")}
            </p>
            <Button variant="neutral" size="sm" disabled={query.isFetching} onClick={() => void query.refetch()}>
              {t("common.retry")}
            </Button>
          </div>
        )}
        {settings && (
          <form className="flex min-w-0 flex-col gap-3" onSubmit={handleSubmit}>
            <SettingList>
              <SettingListItem
                label={t("setting.network.enable")}
                description={t(
                  settings.source !== "settings"
                    ? "setting.network.proxy-inherited"
                    : settings.enabled
                      ? "setting.network.proxy-on"
                      : "setting.network.proxy-off",
                )}
              >
                <Switch
                  checked={enabled}
                  disabled={saving}
                  onCheckedChange={(checked) => {
                    setEnabledEdit(checked);
                    setError(undefined);
                  }}
                />
              </SettingListItem>
              <SettingListItem label={t("setting.network.url")} description={t("setting.network.url-description")} orientation="vertical">
                <Input
                  type="password"
                  autoComplete="new-password"
                  dir="ltr"
                  spellCheck={false}
                  aria-invalid={error === "setting.network.invalid-url"}
                  placeholder={settings.hasProxyUrl ? t("setting.notification.secret-stored") : "http://192.168.1.10:7890"}
                  value={proxyUrl}
                  readOnly={saving}
                  onChange={(event) => {
                    setProxyUrl(event.target.value);
                    setError(undefined);
                  }}
                />
              </SettingListItem>
            </SettingList>
            <div className="flex items-center justify-end gap-3">
              {error && (
                <p role="alert" className="me-auto mu-caption text-mu-text-secondary">
                  {t(error)}
                </p>
              )}
              <Button type="submit" variant="neutral" disabled={saving}>
                {t("common.save")}
              </Button>
            </div>
          </form>
        )}
      </SettingGroup>
    </SettingSection>
  );
}
