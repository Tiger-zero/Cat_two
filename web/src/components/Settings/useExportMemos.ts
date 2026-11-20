import { useState } from "react";
import { getErrorMessage } from "@/lib/error";
import { exportMemos, MemoExportError } from "@/lib/memo-export";
import { showErrorToast, toast } from "@/lib/toast";
import { useTranslate } from "@/utils/i18n";

// useExportMemos starts an export download for the user's memos and reports the outcome.
export const useExportMemos = () => {
  const t = useTranslate();
  const [exporting, setExporting] = useState(false);

  const run = async (username: string, memoUIDs?: readonly string[]) => {
    setExporting(true);
    try {
      await exportMemos(username, memoUIDs);
      toast.add({ title: t("setting.memo-export.export-success"), type: "success" });
    } catch (error) {
      if (error instanceof MemoExportError && error.status === 429) {
        showErrorToast(t("setting.memo-export.export-busy"));
      } else {
        showErrorToast(getErrorMessage(error, t("setting.memo-export.export-failed")));
      }
    } finally {
      setExporting(false);
    }
  };

  return { exportMemos: run, exporting };
};
