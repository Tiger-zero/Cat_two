import { Button, Tooltip } from "@usememos/ui";
import copy from "copy-to-clipboard";
import { CopyIcon } from "lucide-react";
import { showErrorToast, toast } from "@/lib/toast";
import { useTranslate } from "@/utils/i18n";

export default function CopyableSettingValue({ label, value }: { label: string; value: string }) {
  const t = useTranslate();
  const copyLabel = t("setting.mcp-connection.copy-value", { label });

  return (
    <div className="min-w-0">
      <div className="mu-label-sm text-mu-text-secondary">{label}</div>
      <div className="relative mt-1 rounded-mu-lg border border-mu-line/60 bg-mu-well/30">
        <pre className="overflow-x-auto p-3 pe-12 font-mono mu-caption text-mu-text/85" dir="ltr">
          <code>{value}</code>
        </pre>
        <Tooltip.Root>
          <Tooltip.Trigger
            render={
              <Button
                variant="ghost"
                className="absolute top-1.5 end-1.5"
                aria-label={copyLabel}
                onClick={async () => {
                  if (await copy(value).catch(() => false)) {
                    toast.add({ title: t("message.copied"), type: "success" });
                  } else {
                    showErrorToast(t("setting.mcp-connection.copy-failed"));
                  }
                }}
                icon={<CopyIcon />}
              />
            }
          />
          <Tooltip.Popup>{copyLabel}</Tooltip.Popup>
        </Tooltip.Root>
      </div>
    </div>
  );
}
