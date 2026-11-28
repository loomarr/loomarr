import type {
  ChannelLineup,
  ChannelLineupArgs,
  LineupChannel,
  LineupDecade,
  LineupGroup,
  LineupGroupKind,
} from "./channel-lineup.type";

/** How many recently tuned channels the server keeps for a person. */
const RECENT_LIMIT = 20;

const byNumberThenId = (left: LineupChannel, right: LineupChannel) =>
  left.number - right.number || left.id.localeCompare(right.id);

/**
 * The newest of the server's recents that is not on screen and can be played: the shipped phone
 * panel's Previous. The lineup's `lastChannel` is the channel this screen left.
 */
const newestOtherPlayableRecent = (
  currentId: string | undefined,
  recentIds: readonly string[],
  playableIds: readonly string[],
): string | undefined => {
  const playable = new Set(playableIds);
  return recentIds.find((id) => id !== currentId && playable.has(id));
};

const createChannelLineup = <C extends LineupChannel>({
  channels,
  favouriteIds = [],
  lastTunedId,
  leftIds = [],
  recentIds = [],
}: ChannelLineupArgs<C>): ChannelLineup<C> => {
  const ordered = [...channels].sort(byNumberThenId);
  const byId = new Map(ordered.map((channel) => [channel.id, channel]));
  const [leftId] = leftIds;

  // The named channels that exist, in the order named, each once.
  const named = (ids: readonly string[]): C[] =>
    [...new Set(ids)].flatMap((id) => {
      const channel = byId.get(id);
      return channel ? [channel] : [];
    });

  const decades: LineupDecade<C>[] = [...new Set(ordered.flatMap((channel) => channel.decade ?? []))]
    .sort((left, right) => left - right)
    .map((decade) => ({ decade, channels: ordered.filter((channel) => channel.decade === decade) }));

  const makeGroup = (
    kind: LineupGroupKind,
    members: readonly C[],
    headings: readonly LineupDecade<C>[] = [],
  ): LineupGroup<C> => ({ channels: members, decades: headings, empty: members.length === 0, kind });

  const byKind: Record<LineupGroupKind, LineupGroup<C>> = {
    favourites: makeGroup("favourites", named(favouriteIds)),
    all: makeGroup("all", ordered),
    recent: makeGroup("recent", named(recentIds).slice(0, RECENT_LIMIT)),
    decade: makeGroup(
      "decade",
      decades.flatMap((section) => section.channels),
      decades,
    ),
  };

  const playable = (id: string | undefined): C | undefined => {
    const channel = id === undefined ? undefined : byId.get(id);
    return channel?.playable ? channel : undefined;
  };

  return {
    channels: ordered,
    groups: [byKind.favourites, byKind.all, byKind.recent, byKind.decade],
    group: (kind) => byKind[kind],
    positionOf: (channelId, kind = "all") => {
      const index = byKind[kind].channels.findIndex((channel) => channel.id === channelId);
      return index < 0 ? undefined : index;
    },
    lastTunedFirst: (kind = "all") => {
      const members = byKind[kind].channels;
      const last = members.find((channel) => channel.id === lastTunedId);
      return last ? [last, ...members.filter((channel) => channel !== last)] : members;
    },
    find: (query) => {
      const needle = query.trim().toLocaleLowerCase();
      if (!needle) return [];
      const matches = (text: string | undefined) => text?.toLocaleLowerCase().includes(needle) ?? false;
      return ordered.filter(
        (channel) => String(channel.number) === needle || matches(channel.name) || matches(channel.onNow),
      );
    },
    channelByNumber: (digits) =>
      /^\d+$/.test(digits) ? ordered.find((channel) => channel.number === Number(digits)) : undefined,
    neighbour: (currentId, direction, kind = "all") => {
      const members = byKind[kind].channels;
      const at = members.findIndex((channel) => channel.id === currentId);
      // From outside the group, Up and Down enter it at its first or last channel.
      const start = at >= 0 ? at : direction > 0 ? -1 : members.length;
      for (let step = 1; step <= members.length; step += 1) {
        const index = (((start + direction * step) % members.length) + members.length) % members.length;
        const candidate = members[index];
        if (candidate?.playable && candidate.id !== currentId) return candidate;
      }
      return undefined;
    },
    landing: playable(lastTunedId) ?? ordered.find((channel) => channel.playable),
    lastChannel:
      leftId === undefined
        ? undefined
        : { channel: byId.get(leftId), id: leftId, playable: byId.get(leftId)?.playable ?? false },
  };
};

export { createChannelLineup, newestOtherPlayableRecent, RECENT_LIMIT };
