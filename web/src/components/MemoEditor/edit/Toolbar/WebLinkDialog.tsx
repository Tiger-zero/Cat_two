import { Button, Dialog, Field, Input } from "@usememos/ui";
import { type FormEvent, type RefObject, useRef, useState } from "react";
import { isAllowedLinkUrl } from "@/lib/url-policy";
import { useTranslate } from "@/utils/i18n";

interface Props {
  triggerRef: RefObject<HTMLButtonElement | null>;
  onClose: (submitted: boolean) => void;
  onApply: (href: string) => void;
}

function isValidLinkAddress(href: string): boolean {
  if (!href || !isAllowedLinkUrl(href)) return false;
  try {
    new URL(href, window.location.origin);
    return true;
  } catch {
    return false;
  }
}

export default function WebLinkDialog({ triggerRef, onClose, onApply }: Props) {
  const t = useTranslate();
  const [open, setOpen] = useState(true);
  const [address, setAddress] = useState("");
  const [invalid, setInvalid] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);
  const submitted = useRef(false);

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (submitted.current) return;
    const href = address.trim();
    if (!isValidLinkAddress(href)) {
      setInvalid(true);
      return;
    }
    submitted.current = true;
    onApply(href);
    setOpen(false);
  };

  const handleClosed = (open: boolean) => {
    if (open) return;
    onClose(submitted.current);
  };

  return (
    <Dialog.Root open={open} onOpenChange={setOpen} onOpenChangeComplete={handleClosed}>
      <Dialog.Popup
        closeLabel={t("common.close")}
        initialFocus={inputRef}
        finalFocus={() => (submitted.current ? false : triggerRef.current)}
      >
        <Dialog.Title>{t("editor.format.add-web-link")}</Dialog.Title>
        <form className="flex min-w-0 flex-col gap-4" onSubmit={submit}>
          <Field.Root invalid={invalid}>
            <Field.Label>{t("editor.format.link-address")}</Field.Label>
            <Input
              ref={inputRef}
              required
              inputMode="url"
              value={address}
              onChange={(event) => {
                setAddress(event.target.value);
                setInvalid(false);
              }}
            />
            {invalid && <Field.Error match>{t("editor.format.invalid-link-address")}</Field.Error>}
          </Field.Root>
          <Dialog.Footer>
            <Dialog.Close
              render={
                <Button type="button" variant="ghost">
                  {t("common.cancel")}
                </Button>
              }
            />
            <Button type="submit" variant="primary">
              {t("editor.format.add-link")}
            </Button>
          </Dialog.Footer>
        </form>
      </Dialog.Popup>
    </Dialog.Root>
  );
}
