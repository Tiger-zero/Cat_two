interface ManagedAttachmentOrigin {
  readonly origin?: string;
  readonly host?: string;
}

let managedAttachmentOrigin: ManagedAttachmentOrigin = {};

export const getManagedAttachmentOrigin = (): ManagedAttachmentOrigin => managedAttachmentOrigin;

// Match absolute managed image URLs against the configured instance URL.
export const setManagedAttachmentInstanceUrl = (instanceUrl: string | undefined): void => {
  let next: ManagedAttachmentOrigin = {};
  if (instanceUrl?.trim()) {
    try {
      const { origin, host } = new URL(instanceUrl.trim());
      next = { origin, host };
    } catch {
      // Treat a malformed instance URL as unset.
    }
  }
  if (next.origin === managedAttachmentOrigin.origin && next.host === managedAttachmentOrigin.host) return;
  managedAttachmentOrigin = next;
};
