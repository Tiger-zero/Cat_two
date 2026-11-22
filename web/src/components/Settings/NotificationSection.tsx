import { Button, Input, Switch } from "@usememos/ui";
import { type FormEvent, useState } from "react";
import { useInstance } from "@/contexts/InstanceContext";
import { toast } from "@/lib/toast";
import { type Translations, useTranslate } from "@/utils/i18n";
import CopyableSettingValue from "./CopyableSettingValue";
import SettingGroup from "./SettingGroup";
import { SettingList, SettingListItem } from "./SettingList";
import SettingSection from "./SettingSection";
import useAdminSettingsRequest from "./useAdminSettingsRequest";

interface ChannelStatus {
  enabled: boolean;
  configured: boolean;
  source: "environment" | "settings" | "";
  userId: string;
}

interface NotificationSettings {
  telegram: ChannelStatus & { hasBotToken: boolean };
  wecom: ChannelStatus & { corpId: string; agentId: string; hasCallbackToken: boolean; hasEncodingAesKey: boolean };
  wecomCallbackPath: string;
}

type SettingsUpdate =
  | { telegram: { enabled: boolean; userId: string; botToken?: string } }
  | { wecom: { enabled: boolean; userId: string; corpId: string; agentId: string; callbackToken?: string; encodingAesKey?: string } };

type FieldName = "userId" | "botToken" | "corpId" | "agentId" | "callbackToken" | "encodingAesKey";

interface ChannelField {
  name: FieldName;
  label: Translations;
  value?: string;
  numeric?: boolean;
  hasSecret?: boolean;
}

function ChannelForm({
  channel,
  status,
  fields,
  saving,
  save,
}: {
  channel: string;
  status: ChannelStatus;
  fields: ChannelField[];
  saving: boolean;
  save: (enabled: boolean, values: FormData) => Promise<void>;
}) {
  const t = useTranslate();
  const [enabledEdit, setEnabledEdit] = useState<boolean>();
  const [edits, setEdits] = useState<Partial<Record<FieldName, string>>>({});
  const [failed, setFailed] = useState(false);
  const enabled = enabledEdit ?? status.enabled;

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setFailed(false);
    try {
      await save(enabled, new FormData(event.currentTarget));
      setEnabledEdit(undefined);
      setEdits({});
      toast.add({ type: "success", title: t("setting.notification.saved", { channel }) });
    } catch {
      setFailed(true);
    }
  };

  const statusLabel = status.enabled
    ? t("setting.notification.enabled")
    : status.configured
      ? t("setting.notification.disabled")
      : t("setting.notification.not-configured");

  return (
    <form className="flex min-w-0 flex-col gap-3" onSubmit={handleSubmit}>
      <SettingList>
        <SettingListItem
          label={t("setting.notification.enable", { channel })}
          description={status.source === "environment" ? `${statusLabel} · ${t("setting.notification.environment")}` : statusLabel}
        >
          <Switch checked={enabled} disabled={saving} onCheckedChange={setEnabledEdit} />
        </SettingListItem>
        {fields.map(({ name, label, value, numeric, hasSecret }) => (
          <SettingListItem key={name} label={t(label)} orientation="vertical">
            <Input
              name={name}
              dir="ltr"
              type={hasSecret === undefined ? "text" : "password"}
              inputMode={numeric ? "numeric" : undefined}
              autoComplete={hasSecret === undefined ? "off" : "new-password"}
              spellCheck={false}
              placeholder={hasSecret ? t("setting.notification.secret-stored") : undefined}
              value={edits[name] ?? value ?? ""}
              readOnly={saving}
              onChange={(event) => {
                setEdits({ ...edits, [name]: event.target.value });
                setFailed(false);
              }}
            />
          </SettingListItem>
        ))}
      </SettingList>
      <div className="flex items-center justify-end gap-3">
        {failed && (
          <p role="alert" className="me-auto mu-caption text-mu-text-secondary">
            {t("setting.notification.save-error")}
          </p>
        )}
        <Button type="submit" variant="neutral" disabled={saving} aria-label={t("setting.notification.save-channel", { channel })}>
          {t("common.save")}
        </Button>
      </div>
    </form>
  );
}

const readField = (fields: FormData, name: FieldName) => String(fields.get(name) ?? "").trim();

export default function NotificationSection() {
  const t = useTranslate();
  const { profile } = useInstance();
  const { query, saving, save } = useAdminSettingsRequest<NotificationSettings, SettingsUpdate>("/api/notificationSettings");
  const baseUrl = (profile.instanceUrl || window.location.origin).replace(/\/+$/, "");

  return (
    <SettingSection title={t("setting.notification.label")} description={t("setting.notification.description")}>
      {query.isPending && (
        <p role="status" className="mu-body text-mu-text-secondary">
          {t("setting.notification.loading")}
        </p>
      )}
      {query.isError && (
        <div className="flex items-center justify-between gap-3">
          <p role="alert" className="mu-body text-mu-text-secondary">
            {t("setting.notification.load-error")}
          </p>
          <Button variant="neutral" size="sm" disabled={query.isFetching} onClick={() => void query.refetch()}>
            {t("common.retry")}
          </Button>
        </div>
      )}
      {query.data && (
        <>
          <SettingGroup title={t("setting.notification.telegram")} description={t("setting.notification.telegram-description")}>
            <ChannelForm
              channel={t("setting.notification.telegram")}
              status={query.data.telegram}
              fields={[
                { name: "userId", label: "setting.notification.telegram-user-id", value: query.data.telegram.userId, numeric: true },
                { name: "botToken", label: "setting.notification.bot-token", hasSecret: query.data.telegram.hasBotToken },
              ]}
              saving={saving}
              save={(enabled, values) =>
                save({
                  telegram: {
                    enabled,
                    userId: readField(values, "userId"),
                    botToken: readField(values, "botToken") || undefined,
                  },
                })
              }
            />
          </SettingGroup>
          <SettingGroup title={t("setting.notification.wecom")} description={t("setting.notification.wecom-description")} showSeparator>
            <CopyableSettingValue label={t("setting.notification.wecom-callback")} value={baseUrl + query.data.wecomCallbackPath} />
            <ChannelForm
              channel={t("setting.notification.wecom")}
              status={query.data.wecom}
              fields={[
                { name: "corpId", label: "setting.notification.corp-id", value: query.data.wecom.corpId },
                { name: "agentId", label: "setting.notification.agent-id", value: query.data.wecom.agentId, numeric: true },
                { name: "userId", label: "setting.notification.wecom-user-id", value: query.data.wecom.userId },
                { name: "callbackToken", label: "setting.notification.callback-token", hasSecret: query.data.wecom.hasCallbackToken },
                { name: "encodingAesKey", label: "setting.notification.encoding-aes-key", hasSecret: query.data.wecom.hasEncodingAesKey },
              ]}
              saving={saving}
              save={(enabled, values) =>
                save({
                  wecom: {
                    enabled,
                    corpId: readField(values, "corpId"),
                    agentId: readField(values, "agentId"),
                    userId: readField(values, "userId"),
                    callbackToken: readField(values, "callbackToken") || undefined,
                    encodingAesKey: readField(values, "encodingAesKey") || undefined,
                  },
                })
              }
            />
          </SettingGroup>
        </>
      )}
    </SettingSection>
  );
}
