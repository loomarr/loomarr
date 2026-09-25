import type { DevicePlaybackProfile, PlayerChannel, PlayerSource, PlayerSourcePort } from "../player-source";

interface NeighbourWarmerOptions {
  /** Starts fetching a warmed neighbour's still image so the switch overlay can paint it from cache. */
  prefetchStill?: (uri: string) => void;
  profile: DevicePlaybackProfile;
  /** How many channels each side of the current one to warm. The server still bounds admission. */
  radius?: number;
  source: Pick<PlayerSourcePort, "warm">;
}

interface NeighbourWarmer {
  /** Aborts every in-flight warm and starts nothing more. Already-warmed sources stay takeable. */
  cancel: () => void;
  /** Warms the neighbours of `currentId`, superseding whatever the previous call started. */
  retarget: (catalog: readonly PlayerChannel[], currentId: string) => void;
  /** The still address of a warmed neighbour, readable before `take` so the overlay needs no request. */
  stillFor: (channelId: string) => string | undefined;
  /**
   * The exact signed source a neighbour was warmed under, for the real tune. Handed out once, only
   * while comfortably unexpired, with `serverTimeMs` advanced by its age so live chrome stays true.
   * A fresh signature would change the asset URLs, so the warmed bytes would no longer match.
   */
  take: (channelId: string) => PlayerSource | undefined;
}

export type { NeighbourWarmer, NeighbourWarmerOptions };
