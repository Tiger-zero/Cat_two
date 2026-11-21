import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Button, Dialog, Field, Fieldset, Input, List, Radio, RadioGroup } from "@usememos/ui";
import { PlusIcon } from "lucide-react";
import { type FormEvent, useId, useState } from "react";
import ConfirmDialog from "@/components/ConfirmDialog";
import { authenticatedFetch } from "@/connect";
import { APP_DATE_TIME_FORMAT, inAppTimeZone } from "@/lib/time-zone";
import { showErrorToast } from "@/lib/toast";
import { useTranslate } from "@/utils/i18n";
import CopyableSettingValue from "./CopyableSettingValue";

type MCPConnectionStatus = "awaiting" | "pending" | "approved" | "denied" | "revoked" | "expired";
type MCPConnectionDuration = "day" | "permanent";

const statusLabels = {
  awaiting: "setting.mcp-connection.status.legacy",
  pending: "setting.mcp-connection.status.legacy",
  approved: "setting.mcp-connection.status.approved",
  denied: "setting.mcp-connection.status.denied",
  revoked: "setting.mcp-connection.status.revoked",
  expired: "setting.mcp-connection.status.expired",
} as const;

interface MCPConnection {
  id: string;
  label: string;
  status: MCPConnectionStatus;
  createdAt: number;
  requestedAt: number;
  expiresAt: number;
  remoteIp: string;
}

const connectionsPath = "/api/mcpConnections";

class MCPConnectionRequestError extends Error {
  constructor(readonly status: number) {
    super(`MCP connection request returned HTTP ${status}`);
  }
}

async function requestConnection(
  path: string,
  body?: { label: string; duration: MCPConnectionDuration } | { action: "revoke" },
  signal?: AbortSignal,
) {
  const response = await authenticatedFetch(path, {
    cache: "no-store",
    signal,
    ...(body && { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) }),
  });
  if (!response.ok) throw new MCPConnectionRequestError(response.status);
  return response;
}

function CreateMCPConnectionDialog({ serverURL, onClose, onCreated }: { serverURL: string; onClose: () => void; onCreated: () => void }) {
  const t = useTranslate();
  const [label, setLabel] = useState("");
  const [duration, setDuration] = useState<MCPConnectionDuration>("day");
  // Keep the one-time token in this dialog only. The list and its query cache never store it.
  const [token, setToken] = useState("");
  const [pending, setPending] = useState(false);
  const [failed, setFailed] = useState(false);
  const trimmed = label.trim();
  const tooLong = Array.from(trimmed).length > 80;

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!trimmed || tooLong || pending) return;
    setPending(true);
    setFailed(false);
    try {
      const response = await requestConnection(connectionsPath, { label: trimmed, duration });
      const created = (await response.json()) as { token?: string };
      if (!created.token) throw new Error("Missing one-time MCP connection token");
      setToken(created.token);
      onCreated();
    } catch {
      setFailed(true);
    } finally {
      setPending(false);
    }
  };

  return (
    <Dialog.Root open onOpenChange={(open) => !open && !pending && onClose()}>
      <Dialog.Popup closeLabel={t("common.close")}>
        <Dialog.Title>{t("setting.mcp-connection.create")}</Dialog.Title>
        {token ? (
          <>
            <Dialog.Description>{t("setting.mcp-connection.token-shown-once")}</Dialog.Description>
            <div className="flex min-w-0 flex-col gap-3">
              <CopyableSettingValue label={t("setting.access-token.mcp-url")} value={serverURL} />
              <CopyableSettingValue label={t("setting.mcp-connection.bearer-value")} value={`Bearer ${token}`} />
            </div>
            <Dialog.Footer>
              <Button variant="primary" onClick={onClose}>
                {t("common.close")}
              </Button>
            </Dialog.Footer>
          </>
        ) : (
          <form className="flex min-w-0 flex-col gap-4" onSubmit={submit}>
            <Field.Root invalid={tooLong || failed}>
              <Field.Label>{t("setting.mcp-connection.name")}</Field.Label>
              <Input
                required
                readOnly={pending}
                value={label}
                onChange={(event) => {
                  setLabel(event.target.value);
                  setFailed(false);
                }}
              />
              {tooLong && <Field.Error match>{t("setting.mcp-connection.name-too-long")}</Field.Error>}
              {failed && <Field.Error match>{t("setting.mcp-connection.create-error")}</Field.Error>}
            </Field.Root>
            <Field.Root>
              <Fieldset.Root
                render={
                  <RadioGroup
                    value={duration}
                    disabled={pending}
                    onValueChange={(value) => {
                      if (value === "day" || value === "permanent") setDuration(value);
                    }}
                  />
                }
              >
                <Fieldset.Legend>{t("setting.mcp-connection.duration")}</Fieldset.Legend>
                <Field.Item>
                  <Field.Label>
                    <Radio value="day" />
                    {t("setting.mcp-connection.duration-day")}
                  </Field.Label>
                </Field.Item>
                <Field.Item>
                  <Field.Label>
                    <Radio value="permanent" />
                    {t("setting.mcp-connection.duration-permanent")}
                  </Field.Label>
                </Field.Item>
              </Fieldset.Root>
            </Field.Root>
            <Dialog.Footer>
              <Button type="button" variant="ghost" disabled={pending} onClick={onClose}>
                {t("common.cancel")}
              </Button>
              <Button type="submit" variant="primary" loading={pending} disabled={!trimmed || tooLong}>
                {t("common.create")}
              </Button>
            </Dialog.Footer>
          </form>
        )}
      </Dialog.Popup>
    </Dialog.Root>
  );
}

const dateTime = (seconds: number) => inAppTimeZone(new Date(seconds * 1000)).format(APP_DATE_TIME_FORMAT);

function MCPConnectionRow({
  connection,
  disabled,
  onRevoke,
}: {
  connection: MCPConnection;
  disabled: boolean;
  onRevoke: (connection: MCPConnection) => void;
}) {
  const t = useTranslate();
  const { label, status, createdAt, requestedAt, expiresAt, remoteIp } = connection;
  const legacy = status === "awaiting" || status === "pending";
  const active = status === "approved";
  return (
    <List.Item className="flex flex-col items-start gap-2 sm:flex-row sm:items-center sm:justify-between">
      <div className="min-w-0 flex-1">
        <div className="flex min-w-0 flex-wrap items-baseline gap-x-2 gap-y-0.5">
          <span className="mu-label-sm break-words text-mu-text">{label}</span>
          <span className={legacy ? "mu-caption text-mu-warning" : "mu-caption text-mu-text-secondary"}>{t(statusLabels[status])}</span>
        </div>
        <div className="mt-0.5 flex flex-wrap gap-x-3 mu-caption text-mu-text-secondary">
          {requestedAt > 0 ? (
            <span>{t("setting.mcp-connection.requested-at", { time: dateTime(requestedAt) })}</span>
          ) : (
            createdAt > 0 && <span>{t("setting.mcp-connection.created-at", { time: dateTime(createdAt) })}</span>
          )}
          {active && !requestedAt && <span>{t("setting.mcp-connection.not-used")}</span>}
          {remoteIp && <span className="break-all">{t("setting.mcp-connection.remote-ip", { ip: remoteIp })}</span>}
          {expiresAt > 0 && <span>{t("setting.mcp-connection.expires-at", { time: dateTime(expiresAt) })}</span>}
          {active && !expiresAt && <span>{t("setting.mcp-connection.no-expiration")}</span>}
        </div>
      </div>
      {(legacy || active) && (
        <Button size="sm" variant="ghost" disabled={disabled} onClick={() => onRevoke(connection)}>
          {t("setting.mcp-connection.revoke")}
        </Button>
      )}
    </List.Item>
  );
}

export default function MCPConnectionSection({ account, serverURL }: { account: string; serverURL: string }) {
  const t = useTranslate();
  const titleId = useId();
  const queryClient = useQueryClient();
  const [open, setOpen] = useState(false);
  const [revokeTarget, setRevokeTarget] = useState<MCPConnection>();
  const queryKey = ["mcp-connections", account];
  const { data, error, isError, refetch } = useQuery({
    queryKey,
    queryFn: async ({ signal }) => {
      const response = await requestConnection(connectionsPath, undefined, signal);
      return response.json() as Promise<{ connections: MCPConnection[] }>;
    },
    staleTime: 0,
    retry: false,
    refetchInterval: ({ state }) =>
      !state.error &&
      state.data?.connections.some(({ status, expiresAt, requestedAt }) => status === "approved" && (expiresAt > 0 || !requestedAt))
        ? 15_000
        : false,
  });
  const refresh = () => queryClient.invalidateQueries({ queryKey });
  const revoke = useMutation({
    mutationFn: async (id: string) => {
      await requestConnection(`${connectionsPath}/${encodeURIComponent(id)}/decision`, { action: "revoke" });
    },
    retry: false,
    onSettled: refresh,
    onError: (error) => {
      showErrorToast(
        t(
          error instanceof MCPConnectionRequestError && error.status === 409
            ? "setting.mcp-connection.conflict"
            : "setting.mcp-connection.revoke-error",
        ),
      );
    },
  });
  const unsupported = error instanceof MCPConnectionRequestError && error.status === 404;
  const connections = data?.connections ?? [];
  const title = t("setting.mcp-connection.title");
  const config = JSON.stringify(
    { mcpServers: { "cat-two": { type: "http", url: serverURL, headers: { Authorization: "Bearer <YOUR_MCP_TOKEN>" } } } },
    null,
    2,
  );

  return (
    <section aria-labelledby={titleId} className="flex min-w-0 flex-col gap-3">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0 sm:flex-1">
          <h2 id={titleId} className="mu-label text-mu-text">
            {title}
          </h2>
          <p className="mu-body text-mu-text-secondary">{t("setting.mcp-connection.description")}</p>
        </div>
        {!unsupported && (
          <Button variant="primary" disabled={!data || isError} icon={<PlusIcon />} onClick={() => setOpen(true)}>
            {t("setting.mcp-connection.create")}
          </Button>
        )}
      </div>
      {isError ? (
        <div className="flex flex-wrap items-center gap-2">
          <p className="mu-caption text-mu-text-secondary" role="status">
            {t(unsupported ? "setting.mcp-connection.unsupported" : "setting.mcp-connection.load-error")}
          </p>
          {!unsupported && (
            <Button variant="neutral" size="sm" onClick={() => void refetch()}>
              {t("common.retry")}
            </Button>
          )}
        </div>
      ) : !data || connections.length === 0 ? (
        <p className="mu-caption text-mu-text-secondary" role="status">
          {t(data ? "setting.mcp-connection.empty" : "setting.mcp-connection.loading")}
        </p>
      ) : (
        <List.Root aria-label={title}>
          {connections.map((connection) => (
            <MCPConnectionRow key={connection.id} connection={connection} disabled={revoke.isPending} onRevoke={setRevokeTarget} />
          ))}
        </List.Root>
      )}
      {!unsupported && (
        <div className="grid gap-3 lg:grid-cols-2">
          <div className="flex min-w-0 flex-col gap-3">
            <p className="mu-body text-mu-text-secondary">{t("setting.access-token.mcp-description")}</p>
            <CopyableSettingValue label={t("setting.access-token.mcp-url")} value={serverURL} />
          </div>
          <CopyableSettingValue label={t("setting.access-token.mcp-config")} value={config} />
        </div>
      )}
      {open && <CreateMCPConnectionDialog serverURL={serverURL} onClose={() => setOpen(false)} onCreated={() => void refresh()} />}
      <ConfirmDialog
        open={Boolean(revokeTarget)}
        onOpenChange={(open) => !open && setRevokeTarget(undefined)}
        title={t("setting.mcp-connection.revoke-title", { label: revokeTarget?.label })}
        description={t("setting.mcp-connection.revoke-description")}
        cancelLabel={t("common.cancel")}
        confirmLabel={t("setting.mcp-connection.revoke")}
        confirmVariant="danger"
        onConfirm={async () => {
          if (revokeTarget) await revoke.mutateAsync(revokeTarget.id);
        }}
      />
    </section>
  );
}
