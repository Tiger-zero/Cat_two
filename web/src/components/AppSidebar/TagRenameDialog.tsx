import { Button, Dialog, Field, Input } from "@usememos/ui";
import { type FormEvent, useState } from "react";
import { useEditTags } from "@/hooks/useEditTags";
import { getErrorMessage } from "@/lib/error";
import { showErrorToast, toast } from "@/lib/toast";
import { useTranslate } from "@/utils/i18n";
import { isCompleteTagValue } from "@/utils/tag-grammar";

interface Props {
  oldName: string;
  onClose: () => void;
  onRenamed: (name: string) => void;
}

const TagRenameDialog = ({ oldName, onClose, onRenamed }: Props) => {
  const t = useTranslate();
  const editTags = useEditTags();
  const [input, setInput] = useState(oldName);
  const trimmedInput = input.trim();
  const newName = isCompleteTagValue(trimmedInput) ? trimmedInput : trimmedInput.replace(/^#/, "");
  const invalid = newName !== "" && (!isCompleteTagValue(newName) || newName.startsWith(`${oldName}/`));

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (newName === "" || newName === oldName || invalid || editTags.isPending) return;
    try {
      const count = await editTags.mutateAsync({ kind: "rename", oldName, newName });
      if (count > 0) onRenamed(newName);
      toast.add({ title: t("setting.tags.updated-notes", { count }), type: "success" });
      onClose();
    } catch (error) {
      const fallback = t("setting.tags.rename-failed");
      showErrorToast(getErrorMessage(error, fallback) || fallback);
    }
  };

  return (
    <Dialog.Root open onOpenChange={(open) => !open && !editTags.isPending && onClose()}>
      <Dialog.Popup closeLabel={t("common.close")}>
        <Dialog.Title>{t("setting.tags.rename-title", { tag: `#${oldName}` })}</Dialog.Title>
        <Dialog.Description>{t("setting.tags.rename-description")}</Dialog.Description>
        <form onSubmit={submit} className="mt-3">
          <Field.Root invalid={invalid}>
            <Field.Label>{t("setting.tags.new-name")}</Field.Label>
            <Input value={input} onChange={(event) => setInput(event.target.value)} disabled={editTags.isPending} autoComplete="off" />
            {invalid && <Field.Error match>{t("setting.tags.invalid-name")}</Field.Error>}
          </Field.Root>
          <Dialog.Footer>
            <Button type="button" variant="ghost" disabled={editTags.isPending} onClick={onClose}>
              {t("common.cancel")}
            </Button>
            <Button
              type="submit"
              disabled={!newName || newName === oldName || invalid}
              loading={editTags.isPending}
              loadingLabel={t("common.save")}
            >
              {t("common.save")}
            </Button>
          </Dialog.Footer>
        </form>
      </Dialog.Popup>
    </Dialog.Root>
  );
};

export default TagRenameDialog;
