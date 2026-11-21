import { useMutation, useQueryClient } from "@tanstack/react-query";
import { authenticatedFetch } from "@/connect";
import { memoKeys } from "./useMemoQueries";
import { userKeys } from "./useUserQueries";

type TagEdit = { kind: "rename"; oldName: string; newName: string } | { kind: "normalize" };

export function useEditTags() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (edit: TagEdit): Promise<number> => {
      const response = await authenticatedFetch(`/api/tags/${edit.kind}`, {
        method: "POST",
        ...(edit.kind === "rename" && {
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ oldName: edit.oldName, newName: edit.newName }),
        }),
      });
      const result = (await response.json().catch(() => undefined)) as { message?: string; updatedCount?: number } | undefined;
      if (!response.ok || typeof result?.updatedCount !== "number") throw new Error(result?.message);
      return result.updatedCount;
    },
    onSettled: async () => {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: memoKeys.all }),
        queryClient.invalidateQueries({ queryKey: userKeys.stats() }),
      ]);
    },
  });
}
