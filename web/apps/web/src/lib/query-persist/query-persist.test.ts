import { dehydrate, QueryClient } from "@tanstack/react-query";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  type CacheStore,
  type LastUser,
  type StoredCache,
  shouldPersistQuery,
  startQueryPersistence,
} from "./query-persist";

const BUILD = "/assets/index-aaaa.js";

const memoryStore = (initial?: StoredCache): CacheStore & { value: StoredCache | undefined } => {
  const store = {
    value: initial,
    read: async () => store.value,
    write: async (cache: StoredCache) => {
      store.value = cache;
    },
    remove: async () => {
      store.value = undefined;
    },
  };
  return store;
};

const memoryUser = (initial?: string): LastUser & { id: string | undefined } => {
  const user = {
    id: initial,
    read: () => user.id,
    write: (id: string | undefined) => {
      user.id = id;
    },
  };
  return user;
};

const successOf = (key: readonly unknown[], data: unknown) => {
  const qc = new QueryClient();
  qc.setQueryData(key, data);
  return qc.getQueryCache().find({ queryKey: key });
};

describe("which reads are persisted", () => {
  it.each([
    ["/v1/channels"],
    ["/v1/channels/ch_1"],
    ["/v1/channels/now-next"],
    ["/v1/guide"],
    ["/v1/titles"],
    ["/v1/library/items"],
    ["/v1/library/needs"],
    ["/v1/activity"],
    ["/v1/proposals"],
    ["/v1/jobs"],
  ])("persists %s", (url) => {
    const query = successOf([url, { page: 1 }], { ok: true });
    expect(query && shouldPersistQuery(query)).toBe(true);
  });

  it.each([
    ["/v1/channels/ch_1/play-url"],
    ["/v1/auth/me"],
    ["/v1/settings/secrets/token"],
    ["/v1/settings"],
    ["/v1/sessions"],
    ["/v1/auth/devices"],
    ["/v1/system/llm"],
    ["/v1/never-heard-of-it"],
  ])("never persists %s", (url) => {
    const query = successOf([url], { ok: true });
    expect(query && shouldPersistQuery(query)).toBe(false);
  });

  it("does not persist a read that has not succeeded", () => {
    const qc = new QueryClient();
    const query = qc.getQueryCache().build(qc, { queryKey: ["/v1/channels"] });
    expect(shouldPersistQuery(query)).toBe(false);
  });
});

describe("the persisted cache", () => {
  let stop: (() => void) | undefined;
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => {
    stop?.();
    stop = undefined;
    vi.useRealTimers();
  });

  const storedFor = (userId: string, buster = BUILD): StoredCache => {
    const qc = new QueryClient();
    qc.setQueryData(["/v1/channels"], { status: 200, data: [{ id: "ch_1" }] });
    return {
      userId,
      client: { timestamp: Date.now(), buster, clientState: dehydrate(qc) },
    };
  };

  const start = async (store: CacheStore, lastUser: LastUser, qc = new QueryClient()) => {
    stop = await startQueryPersistence({ queryClient: qc, buildId: BUILD, store, lastUser });
    return qc;
  };

  it("paints the last person's cache on open, marked for a quiet refetch", async () => {
    const qc = await start(memoryStore(storedFor("u1")), memoryUser("u1"));
    const state = qc.getQueryState(["/v1/channels"]);
    expect(state?.data).toEqual({ status: 200, data: [{ id: "ch_1" }] });
    expect(state?.status).toBe("success");
    expect(state?.isInvalidated).toBe(true);
  });

  it("restores nothing when the browser's last person is someone else", async () => {
    const qc = await start(memoryStore(storedFor("u1")), memoryUser("u2"));
    expect(qc.getQueryState(["/v1/channels"])).toBeUndefined();
  });

  it("restores nothing when nobody is known to have used this browser", async () => {
    const qc = await start(memoryStore(storedFor("u1")), memoryUser(undefined));
    expect(qc.getQueryState(["/v1/channels"])).toBeUndefined();
  });

  it("discards a cache stored by another build", async () => {
    const store = memoryStore(storedFor("u1", "/assets/index-old.js"));
    const qc = await start(store, memoryUser("u1"));
    expect(qc.getQueryState(["/v1/channels"])).toBeUndefined();
    expect(store.value).toBeUndefined();
  });

  it("discards a cache older than a day", async () => {
    const stored = storedFor("u1");
    stored.client.timestamp = Date.now() - 25 * 60 * 60 * 1000;
    const qc = await start(memoryStore(stored), memoryUser("u1"));
    expect(qc.getQueryState(["/v1/channels"])).toBeUndefined();
  });

  it("writes allowlisted reads, once per window, and never the identity read", async () => {
    const store = memoryStore();
    const qc = await start(store, memoryUser("u1"));
    qc.setQueryData(["/v1/titles"], { status: 200, data: [] });
    qc.setQueryData(["/v1/titles"], { status: 200, data: [1] });
    qc.setQueryData(["/v1/auth/me"], { status: 200, data: { id: "u1" } });
    qc.setQueryData(["/v1/channels/ch_1/play-url"], { status: 200, data: { hls: "https://x/?sig=1" } });
    expect(store.value).toBeUndefined();
    await vi.advanceTimersByTimeAsync(1_100);
    const keys = store.value?.client.clientState.queries.map((q) => q.queryKey[0]);
    expect(store.value?.userId).toBe("u1");
    expect(keys).toEqual(["/v1/titles"]);
  });

  it("drops the previous person's reads when someone else signs in", async () => {
    const lastUser = memoryUser("u1");
    const qc = await start(memoryStore(storedFor("u1")), lastUser);
    qc.setQueryData(["/v1/auth/me"], { status: 200, data: { id: "u2" } });
    expect(qc.getQueryState(["/v1/channels"])).toBeUndefined();
    expect(qc.getQueryState(["/v1/auth/me"])).toBeDefined();
    expect(lastUser.id).toBe("u2");
  });

  it("learns who the first person is without dropping the page already painted", async () => {
    const lastUser = memoryUser(undefined);
    const qc = await start(memoryStore(), lastUser);
    qc.setQueryData(["/v1/channels"], { status: 200, data: [] });
    qc.setQueryData(["/v1/auth/me"], { status: 200, data: { id: "u1" } });
    expect(qc.getQueryState(["/v1/channels"])).toBeDefined();
    expect(lastUser.id).toBe("u1");
  });

  it("forgets everything on sign-out, which clears the cache", async () => {
    const store = memoryStore(storedFor("u1"));
    const lastUser = memoryUser("u1");
    const qc = await start(store, lastUser);
    qc.setQueryData(["/v1/titles"], { status: 200, data: [] });
    qc.clear();
    await vi.advanceTimersByTimeAsync(1_100);
    expect(store.value).toBeUndefined();
    expect(lastUser.id).toBeUndefined();
  });
});
