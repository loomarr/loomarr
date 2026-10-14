import type { MyChannelsOutputBody } from "@loomarr/api/models/myChannelsOutputBody";

type MyChannelsStatus = "error" | "loading" | "ready";

/** One person's two channel lists (#1666). A paired TV reads and writes the lists of whoever paired it. */
interface MyChannels {
  /** Starred channels, oldest first. */
  favouriteIds: readonly string[];
  /** Tuned channels, newest first, at most 20, from any of the person's devices. */
  recentIds: readonly string[];
}

interface MyChannelsSnapshot extends MyChannels {
  status: MyChannelsStatus;
}

/** Every write answers with both lists, so a write replaces the snapshot rather than patching it. */
interface MyChannelsPort {
  load: (signal: AbortSignal) => Promise<MyChannelsOutputBody>;
  recordTune: (channelId: string) => Promise<MyChannelsOutputBody>;
  setFavourite: (channelId: string, starred: boolean) => Promise<MyChannelsOutputBody>;
}

interface MyChannelsController {
  dispose: () => void;
  getSnapshot: () => MyChannelsSnapshot;
  /**
   * Record a tune once it has settled (the first decoded frame), never while warming or prefetching a
   * neighbour. The channel moves to the front at once; the server's lists replace it when they arrive.
   */
  recordTune: (channelId: string) => Promise<void>;
  refresh: () => Promise<void>;
  /** Star or unstar at once; a refused write puts the lists back. */
  setFavourite: (channelId: string, starred: boolean) => Promise<void>;
  subscribe: (listener: () => void) => () => void;
}

export type { MyChannels, MyChannelsController, MyChannelsPort, MyChannelsSnapshot, MyChannelsStatus };
