import { lazyWithReload } from "@/utils/lazy";

// Share the lazy import between the editor and hosts that can load it in advance.
type EditSessionComponent = typeof import("./EditSession").EditSession;
let loadedEditSession: EditSessionComponent | undefined;

const loadEditSession = () =>
  import("./EditSession").then((module) => {
    loadedEditSession = module.EditSession;
    return { default: module.EditSession };
  });
const LazyEditSession = lazyWithReload(loadEditSession);

export const getEditSession = () => loadedEditSession ?? LazyEditSession;

/** Loads the editing session for a host that is about to open an editor. */
export const preloadMemoEditing = (): Promise<void> =>
  loadEditSession().then(
    () => undefined,
    () => undefined,
  );
