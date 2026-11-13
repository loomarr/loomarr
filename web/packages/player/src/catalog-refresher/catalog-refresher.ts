import type { PlayerController } from "../player-controller";
import type { PlayerChannel } from "../player-source";

interface CatalogRefreshState {
  /** True only while there is no catalog to show yet; background refreshes never set it. */
  loading: boolean;
  /** Set only when the initial load failed. A failed background refresh leaves playback alone. */
  error?: string;
}

interface CatalogRefresherOptions {
  controller: Pick<PlayerController, "getSnapshot" | "reconcile">;
  list: (signal: AbortSignal) => Promise<readonly PlayerChannel[]>;
}

interface CatalogRefresher {
  /** Cancels the in-flight request and any queued trailing refresh. */
  abort: () => void;
  getState: () => CatalogRefreshState;
  /**
   * Resolves when a request that started after this call is reconciled. Calls made while one is in
   * flight share a single follow-up request, which runs whether the in-flight one succeeds or fails.
   */
  refresh: () => Promise<void>;
  subscribe: (listener: () => void) => () => void;
}

/**
 * Owns catalog refetch policy so channel-event bursts cannot disturb playback: requests coalesce
 * (never abort each other), and the loading/error state is reserved for the initial load.
 */
const createCatalogRefresher = ({ controller, list }: CatalogRefresherOptions): CatalogRefresher => {
  let state: CatalogRefreshState = { loading: false };
  let inflight: Promise<void> | undefined;
  let request: AbortController | undefined;
  // The one refresh waiting for the in-flight request; every caller that arrives meanwhile shares it.
  let queued: { cancel: () => void; promise: Promise<void>; start: () => void } | undefined;
  const listeners = new Set<() => void>();

  const setState = (next: CatalogRefreshState) => {
    if (next.loading === state.loading && next.error === state.error) return;
    state = next;
    for (const listener of listeners) listener();
  };

  const once = async () => {
    const current = new AbortController();
    request = current;
    const initial = controller.getSnapshot().catalog.length === 0;
    if (initial) setState({ loading: true });
    try {
      await controller.reconcile(await list(current.signal));
      setState({ loading: false });
    } catch (error) {
      if (current.signal.aborted) setState({ loading: false });
      else
        setState(
          initial
            ? { error: error instanceof Error ? error.message : "Couldn't load channels.", loading: false }
            : { loading: false },
        );
      throw error;
    } finally {
      if (request === current) request = undefined;
    }
  };

  // Runs one request; once it settles, success or failure, the queued trailing refresh starts.
  const start = (): Promise<void> => {
    const run = once().finally(() => {
      inflight = undefined;
      const next = queued;
      queued = undefined;
      next?.start();
    });
    inflight = run;
    return run;
  };

  return {
    abort: () => {
      const dropped = queued;
      queued = undefined;
      request?.abort();
      dropped?.cancel();
    },
    getState: () => state,
    refresh: () => {
      if (!inflight) return start();
      if (!queued) {
        let begin!: () => void;
        let cancel!: () => void;
        const promise = new Promise<void>((resolve, reject) => {
          begin = () => start().then(resolve, reject);
          cancel = () => reject(new DOMException("The catalog refresh was aborted.", "AbortError"));
        });
        queued = { cancel, promise, start: begin };
      }
      return queued.promise;
    },
    subscribe: (listener) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
  };
};

export type { CatalogRefresher, CatalogRefresherOptions, CatalogRefreshState };
export { createCatalogRefresher };
