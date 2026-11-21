import type { LocalFile } from "../types/attachment";

export type PersistedDraftFile = Omit<LocalFile, "mediaMetadata"> & { extractMetadata?: boolean };

interface StagedDraftFiles {
  files: PersistedDraftFile[];
  order: string[];
  baseRevision?: string;
  writeId?: string;
}

const DATABASE = "memos.editor-drafts";
const STORE = "staged-files";
let connection: Promise<IDBDatabase> | undefined;

function openDatabase(): Promise<IDBDatabase> {
  if (connection) return connection;

  const opening = new Promise<IDBDatabase>((resolve, reject) => {
    const request = indexedDB.open(DATABASE, 1);
    request.onupgradeneeded = () => {
      request.result.createObjectStore(STORE);
    };
    request.onerror = () => reject(request.error);
    request.onblocked = () => reject(new Error("Draft file storage is blocked by another tab"));
    request.onsuccess = () => {
      const database = request.result;
      if (connection !== opening) {
        database.close();
        return;
      }
      database.onclose = () => {
        if (connection === opening) connection = undefined;
      };
      database.onversionchange = () => {
        database.close();
        if (connection === opening) connection = undefined;
      };
      resolve(database);
    };
  });
  connection = opening;
  void opening.catch(() => {
    if (connection === opening) connection = undefined;
  });
  return opening;
}

/** One store queues read and write transactions in call order, including while the database opens. */
async function transact<T>(mode: IDBTransactionMode, run: (store: IDBObjectStore, complete: (value: T) => void) => void): Promise<T> {
  const database = await openDatabase();
  return new Promise((resolve, reject) => {
    const transaction = database.transaction(STORE, mode);
    let result: T;
    transaction.oncomplete = () => resolve(result);
    transaction.onabort = () => reject(transaction.error ?? new DOMException("Draft file transaction aborted", "AbortError"));
    run(transaction.objectStore(STORE), (value) => {
      result = value;
    });
  });
}

export function loadStagedDraftFiles(key: string): Promise<StagedDraftFiles | undefined> {
  return transact("readonly", (store, complete) => {
    const request = store.get(key);
    request.onsuccess = () => complete(request.result);
  });
}

/** Checks draft keys in priority order without loading their files into memory. */
export function findFirstStagedDraftKey(keys: readonly string[]): Promise<string | undefined> {
  return transact("readonly", (store, complete) => {
    const check = (index: number) => {
      const key = keys[index];
      if (key === undefined) {
        complete(undefined);
        return;
      }
      const request = store.count(key);
      request.onsuccess = () => {
        if (request.result > 0) complete(key);
        else check(index + 1);
      };
    };
    check(0);
  });
}

export function writeStagedDraftFiles(key: string, expected: string | null, draft?: StagedDraftFiles): Promise<boolean> {
  return transact("readwrite", (store, complete) => {
    const request: IDBRequest<StagedDraftFiles | undefined> = store.get(key);
    request.onsuccess = () => {
      // Check inside the write transaction so an old tab cannot clear a
      // different tab's staged files between the check and the commit.
      const actual = request.result?.writeId ?? null;
      if (actual !== expected) {
        complete(false);
        return;
      }
      try {
        if (draft) store.put(draft, key);
        else store.delete(key);
        complete(true);
      } catch {
        store.transaction.abort();
      }
    };
  });
}
