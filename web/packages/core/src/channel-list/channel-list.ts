import type { ChannelLineup, LineupChannel, LineupGroupKind } from "../channel-lineup";
import type {
  ChannelListEvent,
  ChannelListNotice,
  ChannelListPress,
  ChannelListState,
  ChannelListStateArgs,
  ChannelListStep,
  ChannelListView,
} from "./channel-list.type";

/** How far a pointer must move before a second press on a re-centred list counts. */
const PRESS_MOVE_PX = 14;
/** How long after a tap a second tap counts wherever it lands. */
const TOUCH_GAP_MS = 500;

const createChannelListState = ({
  watchedId,
  group = "all",
  open = false,
  television = false,
}: ChannelListStateArgs = {}): ChannelListState => ({
  cursor: { kind: "none" },
  group,
  open,
  query: null,
  television,
  watchedId,
});

const finding = (state: ChannelListState) => !!state.query?.trim();

/**
 * What the list shows: the group's channels, or find's results. Every group is in number order but
 * Recent (newest first) and By decade (decade, then number). The watched channel is always listed:
 * Favourites takes it in its place by number, and Recent and By decade lead with it.
 */
const channelListView = <C extends LineupChannel>(
  state: ChannelListState,
  lineup: ChannelLineup<C>,
): ChannelListView<C> => {
  if (finding(state)) {
    const rows = lineup.find(state.query ?? "");
    return { finding: true, headings: new Map(), notice: rows.length ? undefined : "no-match", rows };
  }
  const members = lineup.group(state.group).channels;
  const watched = lineup.channels.find((channel) => channel.id === state.watchedId);
  let rows = members;
  if (state.group === "favourites") {
    // The lineup keeps favourites in the order starred; a list of channels is in number order.
    rows = lineup.channels.filter((channel) => channel === watched || members.includes(channel));
  } else if (watched && !members.includes(watched)) {
    rows = [watched, ...members];
  }

  const headings = new Map<string, number>();
  if (state.group === "decade") {
    for (const section of lineup.group("decade").decades) {
      const first = section.channels[0];
      if (first) headings.set(first.id, section.decade);
    }
  }

  const empties: Partial<Record<LineupGroupKind, ChannelListNotice>> = {
    decade: "empty-decade",
    favourites: "empty-favourites",
    recent: "empty-recent",
  };
  // Recent always holds the channel on screen, so it is empty when that is all it holds.
  const nothing =
    state.group === "recent" ? members.every((channel) => channel === watched) : !members.length;
  return { finding: false, headings, notice: nothing ? empties[state.group] : undefined, rows };
};

/** The row the cursor goes to: the one asked for, else the watched channel, else the first. */
const landingRow = <C extends LineupChannel>(
  state: ChannelListState,
  lineup: ChannelLineup<C>,
  wanted?: string,
): ChannelListState["cursor"] => {
  const { rows } = channelListView(state, lineup);
  const row =
    rows.find((channel) => channel.id === wanted) ??
    rows.find((channel) => channel.id === state.watchedId) ??
    rows[0];
  return row ? { channelId: row.id, kind: "row" } : { kind: "none" };
};

/** A new group, with find closed and the cursor back on the watched channel. */
const inGroup = <C extends LineupChannel>(
  state: ChannelListState,
  lineup: ChannelLineup<C>,
  group: LineupGroupKind,
): ChannelListState => {
  const next = { ...state, group, query: null };
  return { ...next, cursor: landingRow(next, lineup) };
};

const stepped = <C extends LineupChannel>(
  lineup: ChannelLineup<C>,
  group: LineupGroupKind,
  by: -1 | 1,
): LineupGroupKind => {
  const kinds = lineup.groups.map((each) => each.kind);
  const at = kinds.indexOf(group);
  return kinds[Math.max(0, Math.min(kinds.length - 1, at + by))] ?? group;
};

/**
 * The channel list's interaction model, the same on every platform. Moving the cursor never tunes;
 * only `choose` asks for a tune. Nothing here depends on time: the list closes when asked to.
 */
const channelListReduce = <C extends LineupChannel>(
  state: ChannelListState,
  event: ChannelListEvent,
  lineup: ChannelLineup<C>,
): ChannelListStep => {
  const stay: ChannelListStep = { state };
  const to = (next: Partial<ChannelListState>): ChannelListStep => ({ state: { ...state, ...next } });
  const { cursor } = state;
  const toTabs = (): ChannelListStep =>
    // Find stands where the tabs are.
    state.query === null ? to({ cursor: { group: state.group, kind: "tab" } }) : stay;
  const toList = () => to({ cursor: landingRow(state, lineup) });
  // A tab chosen from the tabs keeps the cursor, so Left and Right go on stepping.
  const chooseGroup = (group: LineupGroupKind): ChannelListStep => {
    const next = inGroup(state, lineup, group);
    return { state: cursor.kind === "tab" ? { ...next, cursor: { group, kind: "tab" } } : next };
  };

  switch (event.type) {
    case "open": {
      const next = { ...state, open: true, query: null };
      return { state: { ...next, cursor: landingRow(next, lineup) } };
    }
    case "close":
      return to({ cursor: { kind: "none" }, open: false, query: null });
    case "back":
      if (state.query !== null) return { state: inGroup(state, lineup, state.group) };
      if (cursor.kind === "tab") return toList();
      return to({ cursor: { kind: "none" }, open: false });
    case "move": {
      if (!state.open) return stay;
      if (cursor.kind === "tab") return event.by > 0 ? toList() : stay;
      const { rows } = channelListView(state, lineup);
      // A list with nothing in it still has its tabs.
      if (!rows.length) return event.by < 0 ? toTabs() : stay;
      const at = cursor.kind === "row" ? rows.findIndex((channel) => channel.id === cursor.channelId) : -1;
      if (at < 0) return toList();
      if (at === 0 && event.by < 0) return toTabs();
      const row = rows[at + event.by];
      return row ? to({ cursor: { channelId: row.id, kind: "row" } }) : stay;
    }
    case "edge": {
      const row = state.open ? channelListView(state, lineup).rows.at(event.to < 0 ? 0 : -1) : undefined;
      return row ? to({ cursor: { channelId: row.id, kind: "row" } }) : stay;
    }
    case "tabs":
      return state.open ? toTabs() : stay;
    case "across": {
      if (!state.open || state.query !== null) return stay;
      if (cursor.kind === "tab") {
        return to({ cursor: { ...cursor, group: stepped(lineup, cursor.group, event.by) } });
      }
      const group = stepped(lineup, state.group, event.by);
      return group === state.group ? stay : { state: inGroup(state, lineup, group) };
    }
    case "chooseTab":
      return chooseGroup(event.group);
    case "showAll":
      return { state: inGroup(state, lineup, "all") };
    case "find": {
      if (state.television) return stay;
      const next = { ...state, open: true, query: event.query };
      const first = finding(next) ? channelListView(next, lineup).rows[0] : undefined;
      if (first) return { state: { ...next, cursor: { channelId: first.id, kind: "row" } } };
      return { state: { ...next, cursor: finding(next) ? { kind: "none" } : landingRow(next, lineup) } };
    }
    case "clearFind":
      return state.query === null ? stay : { state: inGroup(state, lineup, state.group) };
    case "choose": {
      if (event.channelId === undefined && cursor.kind === "tab") return chooseGroup(cursor.group);
      const channelId = event.channelId ?? (cursor.kind === "row" ? cursor.channelId : undefined);
      const channel = lineup.channels.find((each) => each.id === channelId);
      if (!channel) return stay;
      if (channel.id === state.watchedId) {
        // From find's results it only closes find; from the list it closes the list.
        return state.query === null
          ? to({ cursor: { kind: "none" }, open: false })
          : { state: inGroup(state, lineup, state.group) };
      }
      if (!channel.playable) return stay;
      // Choosing a result closes find and leaves the group as it was.
      return {
        state: { ...state, cursor: { channelId: channel.id, kind: "row" }, query: null },
        tune: channel.id,
      };
    }
    case "focusRow":
      return to({ cursor: { channelId: event.channelId, kind: "row" } });
    case "focusTab":
      return to({ cursor: { group: event.group, kind: "tab" } });
    case "tuned": {
      if (event.channelId === state.watchedId) return stay;
      const next = { ...state, watchedId: event.channelId };
      if (cursor.kind !== "row" || finding(next)) return { state: next };
      return { state: { ...next, cursor: landingRow(next, lineup) } };
    }
  }
};

/**
 * Whether a press counts. A tune re-centres the list, so another row slides under a pointer that has
 * not moved: the next press counts once the pointer has moved, or, on touch, after a short gap.
 */
const channelListAllowsPress = (last: ChannelListPress | undefined, next: ChannelListPress): boolean =>
  !last ||
  Math.hypot(next.x - last.x, next.y - last.y) >= PRESS_MOVE_PX ||
  (next.touch && next.at - last.at >= TOUCH_GAP_MS);

export {
  channelListAllowsPress,
  channelListReduce,
  channelListView,
  createChannelListState,
  PRESS_MOVE_PX,
  TOUCH_GAP_MS,
};
