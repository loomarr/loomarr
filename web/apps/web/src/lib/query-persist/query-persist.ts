import { listAndDetail } from "@loomarr/core/events";
import type { Query, QueryClient } from "@tanstack/react-query";
import {
  type PersistedClient,
  type Persister,
  persistQueryClientRestore,
  persistQueryClientSubscribe,
} from "@tanstack/react-query-persist-client";
import { del, get, set } from "idb-keyval";
import { GC_MS } from "../query-client";

// The query cache survives a reload (#3399): a cold open paints the last-known page from
// IndexedDB, then every restored read revalidates quietly. What is written is an ALLOWLIST of
// reads, not a denylist, so a new endpoint is never stored until someone decides it may be:
// nothing that carries a credential or a signed URL (the channel play-url, secrets, the session
// and device lists, `/v1/auth/me`) is on it. The stored copy belongs to one person and one build
// of the app; another person, another build, or a sign-out discards it.

// A stored cache older than the longest a read stays in memory is discarded.
const PERSIST_MAX_AGE_MS = GC_MS;
const CACHE_KEY = "loomarr.query-cache";
const LAST_USER_KEY = "loomarr.query-cache.user";
const ME_URL = "/v1/auth/me";

// How long a cold open waits for IndexedDB before painting without it.
const RESTORE_WAIT_MS = 750;
const WRITE_THROTTLE_MS = 1_000;

// The reads that paint a main page. `listAndDetail` is a collection and its one-segment detail
// (so "/v1/channels/{id}/play-url" is not on it), and the rest are exact URLs.
const PERSISTED_READS: readonly RegExp[] = [
  listAndDetail("/v1/channels"),
  /^\/v1\/guide$/,
  listAndDetail("/v1/titles"),
  listAndDetail("/v1/proposals"),
  listAndDetail("/v1/proposal-jobs"),
  listAndDetail("/v1/library/items"),
  /^\/v1\/library\/needs$/,
  /^\/v1\/library\/status$/,
  listAndDetail("/v1/activity"),
  /^\/v1\/household\/viewing$/,
  /^\/v1\/me\/channels$/,
  /^\/v1\/diagnostics\/health$/,
  /^\/v1\/setup\/state$/,
  listAndDetail("/v1/jobs"),
];

/** Is this query one whose data may be written to disk? A successful read of an allowlisted URL. */
const shouldPersistQuery = (query: Pick<Query, "queryKey" | "state">): boolean => {
  const url = query.queryKey[0];
  return (
    query.state.status === "success" &&
    typeof url === "string" &&
    PERSISTED_READS.some((pattern) => pattern.test(url))
  );
};

interface StoredCache {
  userId: string;
  client: PersistedClient;
}

/** Where the cache is kept: IndexedDB in the app, a map in a test. */
interface CacheStore {
  read: () => Promise<StoredCache | undefined>;
  write: (cache: StoredCache) => Promise<void>;
  remove: () => Promise<void>;
}

const idbStore: CacheStore = {
  read: () => get<StoredCache>(CACHE_KEY),
  write: (cache) => set(CACHE_KEY, cache),
  remove: () => del(CACHE_KEY),
};

/** The id of the person the stored cache belongs to; an id is not a secret, so localStorage is fine. */
interface LastUser {
  read: () => string | undefined;
  write: (id: string | undefined) => void;
}

const localStorageLastUser: LastUser = {
  read: () => {
    try {
      return localStorage.getItem(LAST_USER_KEY) ?? undefined;
    } catch {
      return undefined;
    }
  },
  write: (id) => {
    try {
      if (id === undefined) localStorage.removeItem(LAST_USER_KEY);
      else localStorage.setItem(LAST_USER_KEY, id);
    } catch {
      /* storage blocked: the cache simply won't be restored next time */
    }
  },
};

interface PersistenceOptions {
  queryClient: QueryClient;
  /** Identifies this build of the app; a cache stored by another build is discarded. */
  buildId: string;
  store?: CacheStore;
  lastUser?: LastUser;
  restoreWaitMs?: number;
  writeThrottleMs?: number;
}

const idOfMe = (data: unknown): string | undefined => {
  const response = data as { status?: number; data?: { id?: unknown } } | undefined;
  return response?.status === 200 && typeof response.data?.id === "string" ? response.data.id : undefined;
};

/**
 * Restores the stored query cache (if it is this person's, from this build, and under a day old),
 * marks every restored read for a quiet refetch, then keeps writing the allowlisted reads back.
 * Resolves once the cache is restored or `restoreWaitMs` passes, so the first render has it.
 * Returns a function that stops persisting.
 */
const startQueryPersistence = async ({
  queryClient,
  buildId,
  store = idbStore,
  lastUser = localStorageLastUser,
  restoreWaitMs = RESTORE_WAIT_MS,
  writeThrottleMs = WRITE_THROTTLE_MS,
}: PersistenceOptions): Promise<() => void> => {
  // Who the cache on disk is for. Until /v1/auth/me answers, the last person to use this browser.
  let userId = lastUser.read();

  // The cache changes on every fetch state, so writes wait for it to settle: the newest copy is
  // written once per window, never one per event.
  let latest: PersistedClient | undefined;
  let writeTimer: ReturnType<typeof setTimeout> | undefined;
  const dropPendingWrite = () => {
    clearTimeout(writeTimer);
    writeTimer = undefined;
    latest = undefined;
  };

  const persister: Persister = {
    persistClient: (client) => {
      latest = client;
      writeTimer ??= setTimeout(() => {
        const client = latest;
        dropPendingWrite();
        // A full disk or a blocked database costs only the next cold paint.
        if (client !== undefined && userId !== undefined)
          void store.write({ userId, client }).catch(() => undefined);
      }, writeThrottleMs);
    },
    restoreClient: async () => {
      const stored = await store.read();
      return stored !== undefined && stored.userId === userId ? stored.client : undefined;
    },
    removeClient: () => store.remove(),
  };

  const restored = persistQueryClientRestore({
    queryClient,
    persister,
    buster: buildId,
    maxAge: PERSIST_MAX_AGE_MS,
  })
    // A restored read is shown at once and refetched: a frame fed read is trusted forever
    // otherwise, and a frame sent while the app was closed was never heard.
    .then(() => void queryClient.invalidateQueries())
    .catch(() => undefined);
  await Promise.race([restored, new Promise<void>((resolve) => setTimeout(resolve, restoreWaitMs))]);

  const stopWriting = persistQueryClientSubscribe({
    queryClient,
    persister,
    buster: buildId,
    dehydrateOptions: { shouldDehydrateQuery: shouldPersistQuery },
  });

  const cache = queryClient.getQueryCache();
  const stopWatching = cache.subscribe((event) => {
    // Sign-out clears the whole cache: what was stored goes with it, and so does who it was for.
    if (event.type === "removed" && cache.getAll().length === 0) {
      userId = undefined;
      dropPendingWrite();
      lastUser.write(undefined);
      void store.remove().catch(() => undefined);
      return;
    }
    if (event.type !== "updated" || event.action.type !== "success" || event.query.queryKey[0] !== ME_URL)
      return;
    const id = idOfMe(event.query.state.data);
    if (id === undefined || id === userId) return;
    // A different person signed in on this browser: nothing read for the last one is theirs.
    if (userId !== undefined) {
      queryClient.removeQueries({ predicate: (query) => query.queryKey[0] !== ME_URL });
    }
    userId = id;
    lastUser.write(id);
  });

  return () => {
    stopWriting();
    stopWatching();
    dropPendingWrite();
  };
};

export type { CacheStore, LastUser, StoredCache };
export { shouldPersistQuery, startQueryPersistence };
