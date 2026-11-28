/** What the lineup needs to know about a channel. Callers keep any other fields they pass in. */
interface LineupChannel {
  id: string;
  number: number;
  name: string;
  /** False for a channel this viewer cannot open: it is listed but never flipped or landed to. */
  playable: boolean;
  /** The decade the channel's titles share; absent for a channel with no single decade. */
  decade?: number | undefined;
}

type LineupGroupKind = "favourites" | "all" | "recent" | "decade";

interface LineupDecade<C extends LineupChannel> {
  decade: number;
  channels: readonly C[];
}

interface LineupGroup<C extends LineupChannel> {
  kind: LineupGroupKind;
  /** In the order the group shows them. For By decade this is every section's channels in turn. */
  channels: readonly C[];
  /** A client says so and offers "Show all channels". */
  empty: boolean;
  /** The decade headings, ascending; only By decade has any. */
  decades: readonly LineupDecade<C>[];
}

interface ChannelLineupArgs<C extends LineupChannel> {
  channels: readonly C[];
  /** In the order starred, as `GET /v1/me/channels` returns them. */
  favouriteIds?: readonly string[] | undefined;
  /** Newest first, as `GET /v1/me/channels` returns them. */
  recentIds?: readonly string[] | undefined;
  /** The channel this person last tuned, from any device. */
  lastTunedId?: string | undefined;
}

interface ChannelLineup<C extends LineupChannel> {
  /** Every channel in number order. */
  channels: readonly C[];
  /** The four groups, in the order their tabs head a list. */
  groups: readonly LineupGroup<C>[];
  group: (kind: LineupGroupKind) => LineupGroup<C>;
  /** Where a channel sits in a group, so a list can scroll to it; absent when it is not there. */
  positionOf: (channelId: string, kind?: LineupGroupKind) => number | undefined;
  /** A group with the last tuned channel moved to the front, the order Home's wall uses. */
  lastTunedFirst: (kind?: LineupGroupKind) => readonly C[];
  /** Channels whose name contains the query, or whose number is the query, in number order. */
  find: (query: string) => readonly C[];
  /** The channel that typed digits tune to. */
  channelByNumber: (digits: string) => C | undefined;
  /** The playable channel a flip goes to, wrapping at both ends of the group. */
  neighbour: (currentId: string | undefined, direction: -1 | 1, kind?: LineupGroupKind) => C | undefined;
  /** The last tuned channel, else the lowest-numbered playable one. */
  landing: C | undefined;
  /** The channel the Last channel action returns to. */
  lastChannel: (currentId: string | undefined) => C | undefined;
}

export type { ChannelLineup, ChannelLineupArgs, LineupChannel, LineupDecade, LineupGroup, LineupGroupKind };
