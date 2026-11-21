import { create } from "@bufbuild/protobuf";
import { useQuery } from "@tanstack/react-query";
import { attachmentServiceClient } from "@/connect";
import { ListAttachmentsRequestSchema } from "@/types/proto/api/attachment_service_pb";

// Query keys factory
export const attachmentKeys = {
  all: ["attachments"] as const,
  lists: () => [...attachmentKeys.all, "list"] as const,
};

// Hook to fetch attachments
export function useAttachments() {
  return useQuery({
    queryKey: attachmentKeys.lists(),
    queryFn: async () => {
      const { attachments } = await attachmentServiceClient.listAttachments(create(ListAttachmentsRequestSchema, {}));
      return attachments;
    },
  });
}
