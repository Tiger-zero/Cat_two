import { Button, Field, Input, Switch } from "@usememos/ui";
import { type FormEvent, useState } from "react";
import { useFieldErrors } from "@/hooks/useFieldErrors";
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
  wecom: ChannelStatus & { corpId: string; agentId: string; callbackUrl: string; hasCallbackToken: boolean; hasEncodingAesKey: boolean };
  wecomCallbackPath: string;
  wecomDefaultCallbackUrl?: string;
}

type SettingsUpdate =
  | { telegram: { enabled: boolean; userId: string; botToken?: string } }
  | {
      wecom: {
        enabled: boolean;
        userId: string;
        corpId: string;
        agentId: string;
        callbackToken?: string;
        encodingAesKey?: string;
        callbackUrl?: string;
      };
    };

type FieldName = "userId" | "botToken" | "corpId" | "agentId" | "callbackToken" | "encodingAesKey" | "callbackUrl";

interface ChannelField {
  name: FieldName;
  label: Translations;
  description?: Translations;
  placeholder?: string;
  value?: string;
  numeric?: boolean;
  hasSecret?: boolean;
}

function isWeComCallbackAddress(value: string, callbackPath: string) {
  if (/[\s?#\\]/.test(value)) return false;
  try {
    const url = new URL(value.includes("://") ? value : `https://${value}`);
    const hostname = url.hostname.replace(/\.$/, "");
    return (
      (url.protocol === "https:" || url.protocol === "http:") &&
      hostname.includes(".") &&
      !/^[0-9.]+$/.test(hostname) &&
      !url.username &&
      !url.password &&
      (url.pathname === "/" || url.pathname.endsWith(callbackPath))
    );
  } catch {
    return false;
  }
}

function ChannelForm({
  channel,
  status,
  fields,
  callbackPath,
  saving,
  save,
}: {
  channel: string;
  status: ChannelStatus;
  fields: ChannelField[];
  callbackPath?: string;
  saving: boolean;
  save: (enabled: boolean, values: FormData) => Promise<void>;
}) {
  const t = useTranslate();
  const [enabledEdit, setEnabledEdit] = useState<boolean>();
  const [edits, setEdits] = useState<Partial<Record<FieldName, string>>>({});
  const [failed, setFailed] = useState(false);
  const fieldErrors = useFieldErrors<FieldName>();
  const enabled = enabledEdit ?? status.enabled;

  const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setFailed(false);
    const values = new FormData(event.currentTarget);
    if (edits.callbackUrl === undefined) {
      values.delete("callbackUrl");
    } else if (callbackPath) {
      const address = readField(values, "callbackUrl");
      if (address && !isWeComCallbackAddress(address, callbackPath)) {
        fieldErrors.set("callbackUrl", t("setting.notification.wecom-callback-invalid"));
        return;
      }
    }
    try {
      await save(enabled, values);
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
        {fields.map(({ name, label, description, placeholder, value, numeric, hasSecret }) => (
          <SettingListItem
            key={name}
            label={t(label)}
            description={description && t(description)}
            orientation="vertical"
            invalid={Boolean(fieldErrors.errors[name])}
          >
            <Input
              name={name}
              dir="ltr"
              type={hasSecret === undefined ? "text" : "password"}
              inputMode={numeric ? "numeric" : undefined}
              autoComplete={hasSecret === undefined ? "off" : "new-password"}
              spellCheck={false}
              placeholder={hasSecret ? t("setting.notification.secret-stored") : placeholder}
              value={edits[name] ?? value ?? ""}
              readOnly={saving}
              onChange={(event) => {
                setEdits({ ...edits, [name]: event.target.value });
                setFailed(false);
                fieldErrors.clear(name);
              }}
            />
            {fieldErrors.errors[name] && <Field.Error match>{fieldErrors.errors[name]}</Field.Error>}
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
  const { query, saving, save } = useAdminSettingsRequest<NotificationSettings, SettingsUpdate>("/internal/notificationSettings");
  const callbackUrl = query.data?.wecom.callbackUrl || query.data?.wecomDefaultCallbackUrl;

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
            {callbackUrl && <CopyableSettingValue label={t("setting.notification.wecom-callback")} value={callbackUrl} />}
            <ChannelForm
              channel={t("setting.notification.wecom")}
              status={query.data.wecom}
              callbackPath={query.data.wecomCallbackPath}
              fields={[
                {
                  name: "callbackUrl",
                  label: "setting.notification.wecom-callback-override",
                  description: "setting.notification.wecom-callback-description",
                  placeholder: "https://notes.example.com:8443",
                  value: query.data.wecom.callbackUrl,
                },
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
                    callbackUrl: values.has("callbackUrl") ? readField(values, "callbackUrl") : undefined,
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
