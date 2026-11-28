import type { LineupChannel } from "@loomarr/core/channel-lineup";

/** A channel as the web's watching screen lists it. */
interface WatchLineupChannel extends LineupChannel {
  /** Minutes left of what is on now, when the guide has read it. */
  minutesLeft?: number | undefined;
}

export type { WatchLineupChannel };
