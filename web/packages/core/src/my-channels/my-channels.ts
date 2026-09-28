import {
  getAddFavouriteChannelUrl,
  getMyChannelsUrl,
  getRecordChannelTuneUrl,
  getRemoveFavouriteChannelUrl,
} from "@loomarr/api/endpoints/channels";
import type { MyChannelsOutputBody } from "@loomarr/api/models/myChannelsOutputBody";

import type {
  MyChannels,
  MyChannelsController,
  MyChannelsPort,
  MyChannelsSnapshot,
} from "./my-channels.type";

const RECENT_LIMIT = 20;

const createMyChannelsPort = (request: typeof globalThis.fetch): MyChannelsPort => {
  const send = async (url: string, method: "DELETE" | "GET" | "PUT", signal?: AbortSignal) => {
    const response = await request(url, { method, signal });
    if (!response.ok) throw new Error(`Couldn't update your channels (${response.status}).`);
    return (await response.json()) as MyChannelsOutputBody;
  };
  return {
    load: (signal) => send(getMyChannelsUrl(), "GET", signal),
    recordTune: (channelId) => send(getRecordChannelTuneUrl(channelId), "PUT"),
    setFavourite: (channelId, starred) =>
      starred
        ? send(getAddFavouriteChannelUrl(channelId), "PUT")
        : send(getRemoveFavouriteChannelUrl(channelId), "DELETE"),
  };
};

const listsFrom = (body: MyChannelsOutputBody): MyChannels => ({
  favouriteIds: body.favourites.map((favourite) => favourite.channelId),
  recentIds: body.recent.map((recent) => recent.channelId),
});

const createMyChannelsController = ({ port }: { port: MyChannelsPort }): MyChannelsController => {
  let disposed = false;
  let request: AbortController | undefined;
  let snapshot: MyChannelsSnapshot = { favouriteIds: [], recentIds: [], status: "loading" };
  const listeners = new Set<() => void>();

  const publish = (next: MyChannelsSnapshot) => {
    if (disposed) return;
    snapshot = next;
    for (const listener of listeners) listener();
  };
  const accept = (body: MyChannelsOutputBody) => publish({ ...listsFrom(body), status: "ready" });

  return {
    dispose: () => {
      if (disposed) return;
      disposed = true;
      request?.abort();
      listeners.clear();
    },
    getSnapshot: () => snapshot,
    recordTune: async (channelId) => {
      const recentIds = [channelId, ...snapshot.recentIds.filter((id) => id !== channelId)];
      publish({ ...snapshot, recentIds: recentIds.slice(0, RECENT_LIMIT) });
      try {
        accept(await port.recordTune(channelId));
      } catch {
        // The tune happened on this device either way; the next read reconciles with the server.
      }
    },
    refresh: async () => {
      if (disposed) return;
      request?.abort();
      const next = new AbortController();
      request = next;
      try {
        const body = await port.load(next.signal);
        if (request === next) accept(body);
      } catch {
        if (request === next && !next.signal.aborted) publish({ ...snapshot, status: "error" });
      }
    },
    setFavourite: async (channelId, starred) => {
      const before = snapshot;
      const others = snapshot.favouriteIds.filter((id) => id !== channelId);
      publish({ ...snapshot, favouriteIds: starred ? [...others, channelId] : others });
      try {
        accept(await port.setFavourite(channelId, starred));
      } catch {
        publish(before);
      }
    },
    subscribe: (listener) => {
      if (disposed) return () => undefined;
      listeners.add(listener);
      return () => listeners.delete(listener);
    },
  };
};

export { createMyChannelsController, createMyChannelsPort };
