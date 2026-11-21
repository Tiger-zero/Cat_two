import { authenticatedFetch } from "@/connect";
import { downloadFileFromUrl } from "@/lib/browser";

export class AttachmentDownloadError extends Error {
  constructor(readonly status: number) {
    super(`Attachment download failed with HTTP ${status}`);
  }
}

/** Use the active web session when retrieving a private document. */
export async function downloadAttachmentFile(url: string, filename: string): Promise<void> {
  const response = await authenticatedFetch(url);
  if (!response.ok) throw new AttachmentDownloadError(response.status);

  const objectUrl = URL.createObjectURL(await response.blob());
  try {
    downloadFileFromUrl(objectUrl, filename);
  } finally {
    // The browser takes ownership of the download after the anchor click.
    window.setTimeout(() => URL.revokeObjectURL(objectUrl), 1_000);
  }
}
