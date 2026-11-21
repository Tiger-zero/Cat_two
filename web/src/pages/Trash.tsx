import { useInfiniteQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { Button } from "@usememos/ui";
import { RotateCcwIcon, Trash2Icon } from "lucide-react";
import { useState } from "react";
import ConfirmDialog from "@/components/ConfirmDialog";
import { MemoMarkdown } from "@/components/MemoEditor/MemoMarkdown";
import { attachmentKeys } from "@/hooks/useAttachmentQueries";
import useCurrentUser from "@/hooks/useCurrentUser";
import { memoKeys } from "@/hooks/useMemoQueries";
import { userKeys } from "@/hooks/useUserQueries";
import { APP_DATE_TIME_FORMAT, inAppTimeZone } from "@/lib/time-zone";
import { showErrorToast, toast } from "@/lib/toast";
import { changeTrash, listTrash, trashQueryKey } from "@/lib/trash";
import { useTranslate } from "@/utils/i18n";

export default function Trash() {
  const t = useTranslate();
  const user = useCurrentUser();
  const queryClient = useQueryClient();
  const [confirmName, setConfirmName] = useState<string>();
  const query = useInfiniteQuery({
    queryKey: [...trashQueryKey, user?.name],
    initialPageParam: "",
    queryFn: ({ pageParam, signal }) => listTrash(pageParam, signal),
    getNextPageParam: (page) => page.nextPageToken || undefined,
    enabled: Boolean(user),
  });
  const mutation = useMutation({
    mutationFn: ({ name, action }: { name: string; action: "restore" | "delete" }) => changeTrash(name, action),
    onSuccess: async (_, { action }) => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: trashQueryKey }),
        queryClient.invalidateQueries({ queryKey: memoKeys.all }),
        queryClient.invalidateQueries({ queryKey: userKeys.stats() }),
        queryClient.invalidateQueries({ queryKey: attachmentKeys.lists() }),
      ]);
      toast.add({ title: t(action === "restore" ? "trash.restored" : "trash.deleted"), type: "success" });
    },
    onError: () => showErrorToast(t("trash.change-error")),
  });
  const entries = query.data?.pages.flatMap((page) => page.memos) ?? [];

  return (
    <div className="mx-auto flex w-full max-w-3xl flex-col gap-4">
      <header className="flex flex-col gap-1 px-1">
        <h1 className="mu-heading flex items-center gap-2 text-mu-text">
          <Trash2Icon className="mu-icon text-mu-primary" />
          {t("trash.title")}
        </h1>
        <p className="mu-body text-mu-text-secondary">{t("trash.retention")}</p>
      </header>
      {query.isPending && (
        <p role="status" className="px-1 text-mu-text-secondary">
          {t("trash.loading")}
        </p>
      )}
      {query.isError && (
        <div role="alert" className="memos-panel flex items-center justify-between gap-2 p-3">
          <p>{t("trash.load-error")}</p>
          <Button variant="neutral" onClick={() => void query.refetch()}>
            {t("common.retry")}
          </Button>
        </div>
      )}
      {query.isSuccess && !entries.length && <p className="memos-panel p-6 text-center text-mu-text-secondary">{t("trash.empty")}</p>}
      {entries.map(({ memo, deletedTime }) => (
        <article key={memo.name} className="memos-panel flex min-w-0 flex-col gap-3 p-4">
          <time className="mu-caption text-mu-text-secondary" dateTime={deletedTime}>
            {t("trash.deleted-at", { time: inAppTimeZone(new Date(deletedTime)).format(APP_DATE_TIME_FORMAT) })}
          </time>
          <MemoMarkdown value={memo.content} attachments={memo.attachments} className="memo-text min-w-0 wrap-break-word" />
          {memo.attachments.length > 0 && (
            <ul className="mu-caption space-y-0.5 text-mu-text-secondary">
              {memo.attachments.map((attachment) => (
                <li key={attachment.name} className="wrap-break-word">
                  {attachment.filename}
                </li>
              ))}
            </ul>
          )}
          <div className="flex flex-wrap justify-end gap-2">
            <Button
              variant="neutral"
              icon={<RotateCcwIcon />}
              disabled={mutation.isPending}
              onClick={() => mutation.mutate({ name: memo.name, action: "restore" })}
            >
              {t("common.restore")}
            </Button>
            <Button variant="ghost" disabled={mutation.isPending} onClick={() => setConfirmName(memo.name)}>
              {t("trash.permanent-delete")}
            </Button>
          </div>
        </article>
      ))}
      {query.hasNextPage && (
        <Button variant="neutral" className="self-center" disabled={query.isFetchingNextPage} onClick={() => void query.fetchNextPage()}>
          {t("memo.load-more")}
        </Button>
      )}
      <ConfirmDialog
        open={Boolean(confirmName)}
        onOpenChange={(open) => !open && setConfirmName(undefined)}
        title={t("trash.permanent-delete")}
        description={t("trash.permanent-description")}
        confirmLabel={t("trash.permanent-delete")}
        cancelLabel={t("common.cancel")}
        confirmVariant="danger"
        onConfirm={async () => {
          if (confirmName) await mutation.mutateAsync({ name: confirmName, action: "delete" });
        }}
      />
    </div>
  );
}
