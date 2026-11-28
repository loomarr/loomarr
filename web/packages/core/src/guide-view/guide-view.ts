import type { LineupGroupKind } from "../channel-lineup";
import { channelListView, createChannelListState } from "../channel-list";
import type {
  GuideViewChannel,
  GuideViewContext,
  GuideViewEvent,
  GuideViewLook,
  GuideViewProgramme,
  GuideViewState,
  GuideViewStep,
} from "./guide-view.type";

/** How far Earlier and Later move the span. */
const PAGE_MS = 60 * 60_000;

const clamp = (value: number, min: number, max: number) => Math.max(min, Math.min(max, value));

/** The span starts no earlier than the present hour and ends no later than the schedule held. */
const bounds = <C extends GuideViewChannel>({ nowMs, widthMs, untilMs }: GuideViewContext<C>) => {
  const min = Math.floor(nowMs / PAGE_MS) * PAGE_MS;
  return { max: Math.max(min, untilMs - widthMs), min };
};

/** The rows, in the channel list's order, with its headings and its word on an empty group. */
const guideViewRows = <C extends GuideViewChannel>(state: GuideViewState, context: GuideViewContext<C>) =>
  channelListView(createChannelListState({ group: state.group, watchedId: state.watchedId }), context.lineup);

/** The programmes the cursor can be on: drawn in the span, and not ended. */
const reachable = <C extends GuideViewChannel>(
  channel: C | undefined,
  startMs: number,
  context: GuideViewContext<C>,
): readonly GuideViewProgramme[] =>
  channel?.programmes.filter(
    (p) => p.stopMs > Math.max(startMs, context.nowMs) && p.startMs < startMs + context.widthMs,
  ) ?? [];

const channelOf = <C extends GuideViewChannel>(context: GuideViewContext<C>, id: string) =>
  context.lineup.channels.find((channel) => channel.id === id);

/** The programme a channel shows at a time: on at that moment, else the next one drawn, else the last. */
const programmeAt = <C extends GuideViewChannel>(
  channel: C | undefined,
  atMs: number,
  startMs: number,
  context: GuideViewContext<C>,
) => {
  const drawn = reachable(channel, startMs, context);
  return drawn.find((p) => p.stopMs > atMs) ?? drawn.at(-1);
};

/** The time Up and Down keep once the cursor is on a programme: the middle of what is drawn of it. */
const timeOf = <C extends GuideViewChannel>(
  programme: GuideViewProgramme,
  startMs: number,
  context: GuideViewContext<C>,
) => {
  const from = Math.max(programme.startMs, startMs, context.nowMs);
  const to = Math.min(programme.stopMs, startMs + context.widthMs);
  return (from + Math.max(from, to)) / 2;
};

const onto = (
  state: GuideViewState,
  channelId: string,
  programme: GuideViewProgramme | undefined,
  atMs: number,
): GuideViewState => ({
  ...state,
  cursor: { atMs, channelId, kind: "programme", programmeId: programme?.id },
});

/** The cursor on what is on now on the watched channel, in a span from the present hour. */
const onWatched = <C extends GuideViewChannel>(
  state: GuideViewState,
  context: GuideViewContext<C>,
): GuideViewState => {
  const startMs = bounds(context).min;
  const programme = programmeAt(channelOf(context, state.watchedId), context.nowMs, startMs, context);
  return onto({ ...state, startMs }, state.watchedId, programme, context.nowMs);
};

const createGuideViewState = <C extends GuideViewChannel>(
  args: { watchedId: string; group?: LineupGroupKind | undefined; television?: boolean | undefined },
  context: GuideViewContext<C>,
): GuideViewState =>
  onWatched(
    {
      cursor: { group: args.group ?? "all", kind: "tab" },
      group: args.group ?? "all",
      startMs: 0,
      television: args.television ?? false,
      watchedId: args.watchedId,
    },
    context,
  );

/**
 * The programme the cursor is on. A refreshed schedule may name a block differently, so a block that
 * is gone is looked up again at the cursor's time.
 */
const cursorProgramme = <C extends GuideViewChannel>(
  state: GuideViewState,
  context: GuideViewContext<C>,
): GuideViewProgramme | undefined => {
  const { cursor } = state;
  if (cursor.kind !== "programme") return undefined;
  const channel = channelOf(context, cursor.channelId);
  const held = channel?.programmes.find((p) => p.id === cursor.programmeId);
  const drawn = held && held.stopMs > state.startMs && held.startMs < state.startMs + context.widthMs;
  return drawn ? held : programmeAt(channel, cursor.atMs, state.startMs, context);
};

/** A new span, with the cursor kept on something drawn in its row. */
const paged = <C extends GuideViewChannel>(
  state: GuideViewState,
  context: GuideViewContext<C>,
  startMs: number,
): GuideViewState => {
  const next = { ...state, startMs };
  const { cursor } = state;
  if (cursor.kind !== "programme") return next;
  const channel = channelOf(context, cursor.channelId);
  const drawn = reachable(channel, startMs, context);
  const kept = drawn.find((p) => p.id === cursor.programmeId);
  if (kept) return onto(next, cursor.channelId, kept, timeOf(kept, startMs, context));
  const programme = drawn[0];
  return onto(
    next,
    cursor.channelId,
    programme,
    programme ? timeOf(programme, startMs, context) : cursor.atMs,
  );
};

const stepSpan = <C extends GuideViewChannel>(
  state: GuideViewState,
  context: GuideViewContext<C>,
  by: -1 | 1,
) => {
  const { min, max } = bounds(context);
  return clamp(state.startMs + by * PAGE_MS, min, max);
};

const across = <C extends GuideViewChannel>(
  state: GuideViewState,
  context: GuideViewContext<C>,
  by: -1 | 1,
): GuideViewState => {
  const { cursor } = state;
  if (cursor.kind === "tab") {
    const kinds = context.lineup.groups.map((group) => group.kind);
    const group = kinds[clamp(kinds.indexOf(cursor.group) + by, 0, kinds.length - 1)] ?? cursor.group;
    return { ...state, cursor: { group, kind: "tab" } };
  }
  const channel = channelOf(context, cursor.channelId);
  const current = cursorProgramme(state, context);
  // The neighbour on the row, whether or not it is drawn yet.
  const beside = (from: GuideViewProgramme | undefined) =>
    from
      ? channel?.programmes.filter((p) =>
          by > 0 ? p.startMs > from.startMs : p.startMs < from.startMs && p.stopMs > context.nowMs,
        )
      : undefined;
  const neighbour = by > 0 ? beside(current)?.[0] : beside(current)?.at(-1);
  const drawnIn = (startMs: number) =>
    reachable(channel, startMs, context).find((p) => p.id === neighbour?.id);
  const here = drawnIn(state.startMs);
  if (here) return onto(state, cursor.channelId, here, timeOf(here, state.startMs, context));

  // Past the span's edge: move the span, and take the neighbour once it is drawn.
  const startMs = stepSpan(state, context, by);
  if (startMs === state.startMs) return state;
  const there = drawnIn(startMs);
  if (there) return onto({ ...state, startMs }, cursor.channelId, there, timeOf(there, startMs, context));
  return paged(state, context, startMs);
};

/** OK on a programme: tune what is on now elsewhere, return from it here, or say when a later one starts. */
const choose = <C extends GuideViewChannel>(
  state: GuideViewState,
  context: GuideViewContext<C>,
  channelId: string,
  programme: GuideViewProgramme | undefined,
): GuideViewStep => {
  const channel = channelOf(context, channelId);
  if (!channel) return { state };
  if (programme && programme.startMs > context.nowMs) {
    const ok = state.television ? "OK" : "Enter";
    return {
      message: `${programme.title} starts at ${context.formatTime(programme.startMs)}. ${ok} on what is on now watches ${channel.name}`,
      state,
    };
  }
  if (programme && programme.stopMs <= context.nowMs) return { state };
  if (channelId === state.watchedId) return { close: true, state };
  return channel.playable ? { state, tune: channelId } : { state };
};

/**
 * The Guide's interaction model, the same on every platform. Moving the cursor never tunes: only
 * choosing what is on now, or a row's channel, asks for a tune.
 */
const guideViewReduce = <C extends GuideViewChannel>(
  state: GuideViewState,
  event: GuideViewEvent,
  context: GuideViewContext<C>,
): GuideViewStep => {
  const { cursor } = state;
  const { rows } = guideViewRows(state, context);
  const toRows = (): GuideViewStep => ({ state: onWatched(state, context) });
  const chooseGroup = (group: LineupGroupKind): GuideViewStep => {
    const next = { ...state, group };
    return {
      state: cursor.kind === "tab" ? { ...next, cursor: { group, kind: "tab" } } : onWatched(next, context),
    };
  };

  switch (event.type) {
    case "move": {
      if (cursor.kind === "tab") return event.by > 0 ? toRows() : { state };
      // A row with nothing drawn takes no cursor, so Up and Down pass over it.
      const at = rows.findIndex((channel) => channel.id === cursor.channelId);
      for (let index = at + event.by; at >= 0 && index >= 0 && index < rows.length; index += event.by) {
        const row = rows[index];
        const programme = row && programmeAt(row, cursor.atMs, state.startMs, context);
        if (row && programme) return { state: onto(state, row.id, programme, cursor.atMs) };
      }
      if (at < 0) return toRows();
      return event.by < 0 ? { state: { ...state, cursor: { group: state.group, kind: "tab" } } } : { state };
    }
    case "across":
      return { state: across(state, context, event.by) };
    case "page": {
      const startMs = stepSpan(state, context, event.by);
      return { state: startMs === state.startMs ? state : paged(state, context, startMs) };
    }
    case "choose":
      return cursor.kind === "tab"
        ? chooseGroup(cursor.group)
        : choose(state, context, cursor.channelId, cursorProgramme(state, context));
    case "press":
    case "focusProgramme": {
      const programme = channelOf(context, event.channelId)?.programmes.find(
        (p) => p.id === event.programmeId,
      );
      if (!programme || programme.stopMs <= context.nowMs) return { state };
      const next = onto(state, event.channelId, programme, timeOf(programme, state.startMs, context));
      if (event.type === "focusProgramme") return { state: next };
      // A click on a later programme only shows its facts; OK or Enter on it says when it starts.
      if (programme.startMs > context.nowMs && !event.keyed) return { state: next };
      return choose(next, context, event.channelId, programme);
    }
    case "pressChannel": {
      const channel = channelOf(context, event.channelId);
      if (!channel) return { state };
      if (channel.id === state.watchedId) return { close: true, state };
      return channel.playable ? { state, tune: channel.id } : { state };
    }
    case "focusTab":
      return { state: { ...state, cursor: { group: event.group, kind: "tab" } } };
    case "chooseTab":
      return chooseGroup(event.group);
    case "showAll":
      return { state: onWatched({ ...state, group: "all" }, context) };
    case "back":
      return cursor.kind === "tab" ? toRows() : { close: true, state };
    case "tuned":
      // The cursor stays: another screen changing the channel does not move what is being read.
      return { state: { ...state, watchedId: event.channelId } };
    case "refresh": {
      const { min, max } = bounds(context);
      const startMs = clamp(state.startMs, min, max);
      return { state: startMs === state.startMs ? state : paged(state, context, startMs) };
    }
  }
};

const guideViewLook = <C extends GuideViewChannel>(
  state: GuideViewState,
  context: GuideViewContext<C>,
): GuideViewLook<C> => {
  const watched = channelOf(context, state.watchedId);
  const onNow = (p: GuideViewProgramme) => p.startMs <= context.nowMs && context.nowMs < p.stopMs;
  // From the tabs there is no programme under the cursor, so the header says what is playing.
  if (state.cursor.kind === "tab") {
    return { channel: watched, playing: true, programme: watched?.programmes.find(onNow) };
  }
  const channel = channelOf(context, state.cursor.channelId);
  const programme = cursorProgramme(state, context);
  const playing = channel?.id === state.watchedId && (!programme || onNow(programme));
  return { channel, playing, programme };
};

/** Whether Earlier and Later can move, and where the schedule ends if this page reaches it. */
const guideViewSpan = <C extends GuideViewChannel>(state: GuideViewState, context: GuideViewContext<C>) => {
  const { min, max } = bounds(context);
  const stopMs = state.startMs + context.widthMs;
  return {
    canEarlier: state.startMs > min,
    canLater: state.startMs < max,
    endsAtMs: stopMs >= context.untilMs ? context.untilMs : undefined,
    startMs: state.startMs,
    stopMs,
  };
};

export {
  createGuideViewState,
  cursorProgramme,
  guideViewLook,
  guideViewReduce,
  guideViewRows,
  guideViewSpan,
  PAGE_MS,
};
