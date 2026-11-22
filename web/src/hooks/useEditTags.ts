import { useMutation, useQueryClient } from "@tanstack/react-query";
import { authenticatedFetch } from "@/connect";
import { trashQueryKey } from "@/lib/trash";
import { attachmentKeys } from "./useAttachmentQueries";
import { memoKeys } from "./useMemoQueries";
import { userKeys } from "./useUserQueries";

export type TagDeleteMode = "remove" | "trash";

type TagEdit =
  | { kind: "rename"; oldName: string; newName: string }
  | { kind: "delete"; tagName: string; mode: TagDeleteMode }
  | { kind: "normalize" };

export function useEditTags() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ kind, ...payload }: TagEdit): Promise<number> => {
      const response = await authenticatedFetch(`/api/tags/${kind}`, {
        method: "POST",
        ...(kind !== "normalize" && {
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify(payload),
        }),
      });
      const result = (await response.json().catch(() => undefined)) as { message?: string; updatedCount?: number } | undefined;
      if (!response.ok || typeof result?.updatedCount !== "number") throw new Error(result?.message);
      return result.updatedCount;
    },
    onSettled: async (_, _error, edit) => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: memoKeys.all }),
        queryClient.invalidateQueries({ queryKey: userKeys.stats() }),
        ...(edit.kind === "delete" && edit.mode === "trash"
          ? [
              queryClient.invalidateQueries({ queryKey: trashQueryKey }),
              queryClient.invalidateQueries({ queryKey: attachmentKeys.lists() }),
            ]
          : []),
      ]);
    },
  });
}
