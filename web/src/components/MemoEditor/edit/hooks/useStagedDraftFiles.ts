import { useCallback, useEffect, useRef } from "react";
import { v4 as uuidv4 } from "uuid";
import { cacheService } from "../services/cacheService";
import { mediaMetadataService } from "../services/mediaMetadataService";
import { loadStagedDraftFiles, type StagedDraftFiles, snapshotDraftFiles, writeStagedDraftFiles } from "../services/stagedDraftFiles";
import { useEditorStore } from "../state";
import type { LocalFile } from "../types/attachment";

interface Options {
  key: string;
  enabled: boolean;
  getBaseRevision: () => string | undefined;
  createBlobUrl: (file: File) => string;
  onStorageError: () => void;
  onStaleDraft: (baseRevision?: string) => void;
  preserveDraft: () => void;
}

export function useStagedDraftFiles({
  key,
  enabled,
  getBaseRevision,
  createBlobUrl,
  onStorageError,
  onStaleDraft,
  preserveDraft,
}: Options) {
  const store = useEditorStore();
  const onDiscard = useRef<(() => Promise<void>) | undefined>(undefined);

  useEffect(() => {
    if (!enabled) return;
    const { getState, actions, dispatch } = store;
    let mounted = true;
    let discarded = false;
    let ready = false;
    let pending = false;
    let warned = false;
    let operation = 0;
    let tail = Promise.resolve();
    let expected: string | null = null;
    let hasDraft = false;
    let lastFiles: LocalFile[] = [];
    let lastOrder: string[] = [];
    let lastRevision = getBaseRevision();
    let navigationSnapshot: StagedDraftFiles | undefined;

    const preventLoss = (event: BeforeUnloadEvent) => {
      event.preventDefault();
      event.returnValue = true;
    };
    const syncWarning = () => {
      const state = getState();
      const unsafe =
        !discarded &&
        (pending || (!ready && state.localFiles.length > 0) || state.recorderBusy || state.ui.pendingInlineImageInsertions > 0);
      if (unsafe) window.addEventListener("beforeunload", preventLoss);
      else window.removeEventListener("beforeunload", preventLoss);
    };

    const write = (files: LocalFile[], order: string[], baseRevision?: string) => {
      if (cacheService.recoveredFiles(key)) {
        if (!discarded) preserveDraft();
        return tail;
      }
      const id = ++operation;
      const draft = files.length ? { ...snapshotDraftFiles(files, order, baseRevision), writeId: uuidv4() } : undefined;
      pending = true;
      syncWarning();
      tail = tail.then(async () => {
        const recovery = cacheService.recoveredFiles(key);
        if (recovery && recovery !== navigationSnapshot) {
          if (id === operation && mounted) {
            pending = false;
            syncWarning();
          }
          return;
        }
        let committed = false;
        try {
          if (draft || hasDraft) committed = await writeStagedDraftFiles(key, expected, draft);
          else committed = true;
        } catch {
          // Unavailable storage or an interrupted transaction leaves the draft in memory.
        }
        if (committed) {
          hasDraft = Boolean(draft);
          expected = draft?.writeId ?? null;
          if (id === operation && navigationSnapshot) cacheService.discardCommittedRecovery(key, navigationSnapshot);
        }
        if (id !== operation || !mounted) return;
        pending = !committed;
        if (pending && !discarded) {
          preserveDraft();
          if (!warned) onStorageError();
        }
        warned = pending;
        syncWarning();
      });
      return tail;
    };

    const persist = () => {
      if (ready) {
        const { localFiles, attachmentOrder } = getState();
        const baseRevision = getBaseRevision();
        if (lastFiles !== localFiles || lastOrder !== attachmentOrder || lastRevision !== baseRevision) {
          const hadFiles = lastFiles.length > 0;
          lastFiles = localFiles;
          lastOrder = attachmentOrder;
          lastRevision = baseRevision;
          if (localFiles.length > 0) {
            discarded = false;
            void write(localFiles, attachmentOrder, baseRevision);
          } else if (hadFiles) {
            void write([], []);
          }
        }
      }
      syncWarning();
    };
    const unsubscribe = store.subscribe(persist);

    const restore = (saved: StagedDraftFiles) => {
      if (saved.baseRevision !== getBaseRevision()) {
        onStaleDraft(saved.baseRevision);
        return;
      }
      if (discarded) return;
      const state = getState();
      const rebuiltURLs = new Map<string, string>();
      const restored = saved.files.map(({ previewUrl, extractMetadata, ...row }) => {
        const rebuilt = createBlobUrl(row.file);
        rebuiltURLs.set(previewUrl, rebuilt);
        return {
          ...row,
          previewUrl: rebuilt,
          mediaMetadata: extractMetadata ? mediaMetadataService.extract(row.file) : undefined,
        };
      });
      lastFiles = restored;
      lastOrder = saved.order.map((entry) => rebuiltURLs.get(entry) ?? entry);
      dispatch(actions.setLocalFiles(state.localFiles.length > 0 ? [...restored, ...state.localFiles] : restored));
      dispatch(actions.setAttachmentOrder(lastOrder));
    };
    let loading = Promise.resolve();
    const recovered = cacheService.recoveredFiles(key);
    if (recovered) {
      restore(recovered);
      ready = true;
    } else if (typeof indexedDB === "undefined") {
      ready = true;
    } else {
      dispatch(actions.setLoading("loading", true));
      const openedRevision = getBaseRevision();
      loading = loadStagedDraftFiles(key)
        .then((saved) => {
          if (!mounted || !saved) return;
          expected = saved.writeId ?? null;
          hasDraft = saved.baseRevision === openedRevision;
          restore(saved);
        })
        .catch(() => {
          if (mounted && getState().localFiles.length > 0) onStorageError();
        })
        .finally(() => {
          if (!mounted) return;
          ready = true;
          dispatch(actions.setLoading("loading", false));
        });
    }
    onDiscard.current = async () => {
      discarded = true;
      syncWarning();
      await loading;
      if (pending || hasDraft) await write([], []);
    };

    return () => {
      mounted = false;
      unsubscribe();
      onDiscard.current = undefined;
      if (!discarded && (pending || (!ready && getState().localFiles.length > 0)) && !cacheService.recoveredFiles(key)) {
        preserveDraft();
        navigationSnapshot = cacheService.recoveredFiles(key);
      }
      window.removeEventListener("beforeunload", preventLoss);
    };
  }, [store, key, enabled, getBaseRevision, createBlobUrl, onStorageError, onStaleDraft, preserveDraft]);

  return useCallback(() => onDiscard.current?.(), []);
}
