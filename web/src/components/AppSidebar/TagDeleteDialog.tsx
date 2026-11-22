import { AlertDialog, Button, Field, Fieldset, Radio, RadioGroup } from "@usememos/ui";
import { type FormEvent, useState } from "react";
import { type TagDeleteMode, useEditTags } from "@/hooks/useEditTags";
import { showErrorToast, toast } from "@/lib/toast";
import { useTranslate } from "@/utils/i18n";

interface Props {
  tagName: string;
  onClose: () => void;
  onDeleted: () => void;
}

export default function TagDeleteDialog({ tagName, onClose, onDeleted }: Props) {
  const t = useTranslate();
  const editTags = useEditTags();
  const [mode, setMode] = useState<TagDeleteMode>("remove");
  const { isPending } = editTags;
  const submitLabel = t(mode === "remove" ? "setting.tags.delete-remove" : "setting.tags.delete-trash");

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (isPending) return;
    try {
      const count = await editTags.mutateAsync({ kind: "delete", tagName, mode });
      onDeleted();
      toast.add({
        title: t(mode === "remove" ? "setting.tags.updated-notes" : "setting.tags.trashed-notes", { count }),
        type: "success",
      });
      onClose();
    } catch {
      showErrorToast(t("setting.tags.delete-failed"));
    }
  };

  return (
    <AlertDialog.Root open onOpenChange={(open) => !open && !isPending && onClose()}>
      <AlertDialog.Popup>
        <AlertDialog.Title>{t("setting.tags.delete-title", { tag: `#${tagName}` })}</AlertDialog.Title>
        <AlertDialog.Description>{t("setting.tags.delete-description")}</AlertDialog.Description>
        <form onSubmit={submit} className="mt-3">
          <Field.Root>
            <Fieldset.Root
              render={
                <RadioGroup
                  value={mode}
                  disabled={isPending}
                  onValueChange={(value) => {
                    if (value === "remove" || value === "trash") setMode(value);
                  }}
                />
              }
            >
              <Fieldset.Legend>{t("setting.tags.delete-how")}</Fieldset.Legend>
              <Field.Item>
                <Field.Label>
                  <Radio value="remove" />
                  {t("setting.tags.delete-remove")}
                </Field.Label>
                <Field.Description>{t("setting.tags.delete-remove-description")}</Field.Description>
              </Field.Item>
              <Field.Item>
                <Field.Label>
                  <Radio value="trash" />
                  {t("setting.tags.delete-trash")}
                </Field.Label>
                <Field.Description>{t("setting.tags.delete-trash-description")}</Field.Description>
              </Field.Item>
            </Fieldset.Root>
          </Field.Root>
          <AlertDialog.Footer>
            <Button type="button" variant="ghost" disabled={isPending} onClick={onClose}>
              {t("common.cancel")}
            </Button>
            <Button type="submit" variant={mode === "trash" ? "danger" : "primary"} loading={isPending} loadingLabel={submitLabel}>
              {submitLabel}
            </Button>
          </AlertDialog.Footer>
        </form>
      </AlertDialog.Popup>
    </AlertDialog.Root>
  );
}
