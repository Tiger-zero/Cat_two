import { Field, Input } from "@usememos/ui";
import { useTranslate } from "@/utils/i18n";

interface CodeFieldProps {
  value: string;
  onChange: (value: string) => void;
  error?: string;
  readOnly?: boolean;
  autoFocus?: boolean;
}

export function TwoFactorCodeField({ value, onChange, error, readOnly, autoFocus }: CodeFieldProps) {
  const t = useTranslate();
  return (
    <Field.Root invalid={Boolean(error)}>
      <Field.Label>{t("two-factor.code")}</Field.Label>
      <Input
        type="text"
        inputMode="numeric"
        autoComplete="one-time-code"
        autoFocus={autoFocus}
        pattern="[0-9]{6}"
        maxLength={6}
        required
        readOnly={readOnly}
        value={value}
        onChange={(event) => onChange(event.target.value.replace(/[^0-9]/g, "").slice(0, 6))}
      />
      {error && <Field.Error match>{error}</Field.Error>}
    </Field.Root>
  );
}

export function TwoFactorRecovery() {
  const t = useTranslate();
  return (
    <details className="mu-caption text-mu-text-secondary">
      <summary className="w-fit cursor-pointer rounded-mu-sm border border-transparent text-mu-accent mu-focus">
        {t("two-factor.recovery-title")}
      </summary>
      <p className="mt-2">{t("two-factor.recovery-description")}</p>
      <code className="mt-2 block select-all break-words rounded-mu-md bg-mu-well px-2 py-1 font-mono text-mu-text">
        docker compose exec cat-two /usr/local/memos/memos two-factor reset --username YOUR_USERNAME
      </code>
    </details>
  );
}
