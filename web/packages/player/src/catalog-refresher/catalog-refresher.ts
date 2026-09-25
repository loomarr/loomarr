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
  /** Resolves when this call's data is reconciled; concurrent calls share one request. */
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
  let trailing = false;
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

  const drain = async () => {
    do {
      trailing = false;
      await once();
    } while (trailing);
  };

  return {
    abort: () => {
      trailing = false;
      request?.abort();
    },
    getState: () => state,
    refresh: () => {
      if (inflight) {
        trailing = true;
        return inflight;
      }
      const run = drain().finally(() => {
        if (inflight === run) inflight = undefined;
      });
      inflight = run;
      return run;
    },
    subscribe: (listener) => {
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
  };
};

export type { CatalogRefresher, CatalogRefresherOptions, CatalogRefreshState };
export { createCatalogRefresher };
