import { Button, Dialog, Field, Input } from "@usememos/ui";
import copy from "copy-to-clipboard";
import { useState } from "react";
import { TwoFactorCodeField, TwoFactorRecovery } from "@/components/TwoFactorFields";
import { useAuth } from "@/contexts/AuthContext";
import useCurrentUser from "@/hooks/useCurrentUser";
import useNavigateTo from "@/hooks/useNavigateTo";
import { requestTwoFactor, useTwoFactorStatus } from "@/hooks/useTwoFactor";
import { showErrorToast, toast } from "@/lib/toast";
import { ROUTES } from "@/router/routes";
import { useTranslate } from "@/utils/i18n";
import SettingGroup from "./SettingGroup";

interface TwoFactorStatus {
  enabled: boolean;
}

interface TwoFactorDialogProps {
  enabled: boolean;
  onClose: () => void;
  onChanged: () => Promise<void>;
}

function TwoFactorDialog({ enabled, onClose, onChanged }: TwoFactorDialogProps) {
  const t = useTranslate();
  const [password, setPassword] = useState("");
  const [code, setCode] = useState("");
  const [secret, setSecret] = useState("");
  const [error, setError] = useState(false);
  const [pending, setPending] = useState(false);
  const isSetup = !enabled && !secret;
  const title = t(enabled ? "two-factor.disable" : "two-factor.enable");

  const submit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (pending || !password || (!isSetup && code.length !== 6)) return;
    setError(false);
    setPending(true);
    try {
      if (isSetup) {
        const configuration = await requestTwoFactor<{ secret: string }>("/setup", { password });
        if (!configuration.secret) throw new Error("Missing authenticator setup key");
        setSecret(configuration.secret);
        return;
      }
      const status = await requestTwoFactor<TwoFactorStatus>(enabled ? "/disable" : "/enable", { password, code });
      if (status.enabled !== !enabled) throw new Error("Two-factor status did not change");
      setPassword("");
      setCode("");
      setSecret("");
    } catch {
      setCode("");
      setError(true);
      return;
    } finally {
      setPending(false);
    }
    await onChanged();
  };

  return (
    <Dialog.Root open onOpenChange={(open) => !open && !pending && onClose()}>
      <Dialog.Popup closeLabel={t("common.close")}>
        <Dialog.Title>{title}</Dialog.Title>
        <Dialog.Description>
          {t(enabled ? "two-factor.disable-description" : secret ? "two-factor.enroll-description" : "two-factor.confirm-password")}
        </Dialog.Description>
        <form className="flex min-w-0 flex-col gap-4" onSubmit={submit}>
          {(enabled || isSetup) && (
            <Field.Root invalid={isSetup && error}>
              <Field.Label>{t("two-factor.current-password")}</Field.Label>
              <Input
                type="password"
                autoComplete="current-password"
                required
                readOnly={pending}
                value={password}
                onChange={(event) => {
                  setPassword(event.target.value);
                  setError(false);
                }}
              />
              {isSetup && error && <Field.Error match>{t("two-factor.setup-error")}</Field.Error>}
            </Field.Root>
          )}
          {secret && (
            <div className="flex min-w-0 flex-col gap-1">
              <p className="mu-label">{t("two-factor.setup-key")}</p>
              <div className="flex min-w-0 flex-wrap items-center gap-2">
                <code className="min-w-0 select-all break-all rounded-mu-md bg-mu-well px-2 py-1 font-mono text-mu-text">{secret}</code>
                <Button
                  type="button"
                  variant="neutral"
                  onClick={async () => {
                    if (await copy(secret).catch(() => false)) {
                      toast.add({ title: t("message.copied"), type: "success" });
                    } else {
                      showErrorToast(t("two-factor.copy-error"));
                    }
                  }}
                >
                  {t("common.copy")}
                </Button>
              </div>
            </div>
          )}
          {!isSetup && (
            <TwoFactorCodeField
              autoFocus={!enabled}
              value={code}
              readOnly={pending}
              error={error ? t("two-factor.change-error") : undefined}
              onChange={(value) => {
                setCode(value);
                setError(false);
              }}
            />
          )}
          <Dialog.Footer>
            <Button type="button" variant="ghost" disabled={pending} onClick={onClose}>
              {t("common.cancel")}
            </Button>
            <Button
              type="submit"
              variant={enabled ? "danger" : "primary"}
              loading={pending}
              disabled={!password || (!isSetup && code.length !== 6)}
            >
              {isSetup ? t("two-factor.get-setup-key") : title}
            </Button>
          </Dialog.Footer>
        </form>
      </Dialog.Popup>
    </Dialog.Root>
  );
}

export default function TwoFactorSection() {
  const t = useTranslate();
  const user = useCurrentUser();
  const { logout } = useAuth();
  const navigateTo = useNavigateTo();
  const [open, setOpen] = useState(false);
  const { data, isError, refetch } = useTwoFactorStatus(user?.name);
  const unavailableStatus = t(isError ? "two-factor.status-error" : "two-factor.loading");

  const onChanged = async () => {
    setOpen(false);
    toast.add({ title: t(data?.enabled ? "two-factor.disabled-sign-in" : "two-factor.enabled-sign-in"), type: "success" });
    await logout();
    navigateTo(ROUTES.AUTH, { replace: true });
  };

  return (
    <SettingGroup showSeparator title={t("two-factor.title")} description={t("two-factor.description")}>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <p className="mu-caption text-mu-text-secondary" role="status">
          {data ? t(data.enabled ? "two-factor.enabled" : "two-factor.disabled") : unavailableStatus}
        </p>
        {data ? (
          <Button variant="neutral" onClick={() => setOpen(true)}>
            {t(data.enabled ? "two-factor.disable" : "two-factor.enable")}
          </Button>
        ) : (
          isError && (
            <Button variant="neutral" onClick={() => void refetch()}>
              {t("common.retry")}
            </Button>
          )
        )}
      </div>
      {data?.enabled && <TwoFactorRecovery />}
      {data && open && <TwoFactorDialog enabled={data.enabled} onClose={() => setOpen(false)} onChanged={onChanged} />}
    </SettingGroup>
  );
}
