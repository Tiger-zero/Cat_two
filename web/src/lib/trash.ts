import { fromJson, type JsonValue } from "@bufbuild/protobuf";
import { authenticatedFetch } from "@/connect";
import { MemoSchema } from "@/types/proto/api/memo_service_pb";

export const trashQueryKey = ["trash"] as const;

export async function listTrash(pageToken: string, signal: AbortSignal) {
  const params = new URLSearchParams({ pageSize: "30" });
  if (pageToken) params.set("pageToken", pageToken);
  const response = await authenticatedFetch(`/internal/trash?${params}`, { signal });
  if (!response.ok) throw new Error(`Could not load trash (${response.status})`);
  const data: { memos: { memo: JsonValue; deletedTime: string }[]; nextPageToken: string } = await response.json();
  return {
    memos: data.memos.map(({ memo, deletedTime }) => ({ memo: fromJson(MemoSchema, memo), deletedTime })),
    nextPageToken: data.nextPageToken,
  };
}

export async function changeTrash(memoName: string, action: "restore" | "delete") {
  const uid = encodeURIComponent(memoName.slice("memos/".length));
  const restore = action === "restore";
  const response = await authenticatedFetch(`/internal/trash/${uid}${restore ? "/restore" : ""}`, { method: restore ? "POST" : "DELETE" });
  if (!response.ok) throw new Error(`Could not update trash (${response.status})`);
}
