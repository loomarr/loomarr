// The watching screen of The Cross (frontend-design §9.1, target), the same on every client. Pure:
// a client feeds it what happened, with the time, draws `watchScreenView`, does what it asks, and
// sends a `tick` at the view's `nextAt`. It starts no timer and touches no player.

import type { ChannelLineup, LineupChannel } from "../channel-lineup";
import {
  type ChannelListEvent,
  channelListReduce,
  channelListView,
  createChannelListState,
} from "../channel-list";
import type {
  WatchControl,
  WatchHandOff,
  WatchKeyInput,
  WatchLayout,
  WatchMenuName,
  WatchPlayback,
  WatchRequest,
  WatchScreenEvent,
  WatchScreenState,
  WatchScreenStateArgs,
  WatchScreenStep,
  WatchScreenView,
} from "./watch-screen.type";

/** How long the bare picture waits before the lower third, the quiet row and the hint rest. */
const WATCH_REST_MS = 5000;
/** How long typed digits wait for another before they tune. */
const WATCH_DIGITS_MS = 1600;
/** How many channels left the screen remembers. */
const LEFT_LIMIT = 20;

/** Only where the picture fills the screen does anything rest: nothing covers it elsewhere. */
const RESTS: ReadonlySet<WatchLayout> = new Set(["full", "landscape", "tv"]);

const createWatchScreenState = ({
  at,
  group,
  hintSeen = false,
  layout,
  watchedId,
}: WatchScreenStateArgs): WatchScreenState => ({
  activeAt: at,
  behindMs: 0,
  clicked: undefined,
  digits: undefined,
  focus: undefined,
  handOff: undefined,
  hintSeen,
  layout,
  leftIds: [],
  list: createChannelListState({ group, television: layout === "tv", watchedId }),
  listFromControl: false,
  menu: undefined,
  paused: false,
  pausedAt: undefined,
  picture: "playing",
  pointerOn: [],
  watchedId,
});

/** Only the bare picture rests: anything open, focused, pointed at, paused or typed holds it. */
/**
 * Paused, or a whole second or more behind live: what the lower third says as behind, and the one
 * test Go live and End answer to, so the two never disagree.
 */
const offLive = (state: WatchScreenState) => state.paused || Math.floor(state.behindMs / 1000) > 0;

const held = (state: WatchScreenState) =>
  state.list.open ||
  state.menu !== undefined ||
  state.focus !== undefined ||
  state.pointerOn.length > 0 ||
  state.paused ||
  state.digits !== undefined;

/**
 * The key the screen acts on, or undefined when it is not the screen's: a key with a modifier, one
 * a control or the open list already took, one typed into a field, or one an input method is
 * still building.
 */
const watchKeyOf = (input: WatchKeyInput): string | undefined =>
  input.altKey || input.ctrlKey || input.metaKey || input.defaultPrevented || input.inField || input.composing
    ? undefined
    : input.key;

/** The longest channel number, so typed digits stop where no channel could be named. */
const digitsWidth = <C extends LineupChannel>(lineup: ChannelLineup<C>) =>
  Math.max(1, ...lineup.channels.map((channel) => String(channel.number).length));

const unplayable = (channel: LineupChannel) => `${channel.number} ${channel.name} cannot be played right now`;

// The arrow keys the open list takes when focus is not already in it.
const LIST_KEYS: Record<string, ChannelListEvent> = {
  ArrowDown: { by: 1, type: "move" },
  ArrowLeft: { by: -1, type: "across" },
  ArrowRight: { by: 1, type: "across" },
  ArrowUp: { by: -1, type: "move" },
  End: { to: 1, type: "edge" },
  Enter: { type: "choose" },
  Home: { to: -1, type: "edge" },
};

const MENU_KEYS: Record<string, WatchMenuName> = { "?": "shortcuts", a: "audio", c: "subtitles" };

/**
 * The lower third is drawn while the list is closed and the watched channel is in the lineup. When
 * it is not drawn it went with no pointer-leave or blur (the list took its place, or the channel
 * left the lineup), so what the pointer and focus held on it is let go. The quiet row stays, and
 * so does a pointer resting on it.
 */
const withoutHiddenThird = <C extends LineupChannel>(
  state: WatchScreenState,
  lineup: ChannelLineup<C>,
): WatchScreenState => {
  const drawn = !state.list.open && lineup.channels.some((channel) => channel.id === state.watchedId);
  const held = state.pointerOn.includes("lowerThird") || state.focus === "goLive";
  if (drawn || !held) return state;
  return {
    ...state,
    focus: state.focus === "goLive" ? undefined : state.focus,
    pointerOn: state.pointerOn.filter((region) => region !== "lowerThird"),
  };
};

/**
 * The watching screen's interaction model. Flipping tunes at once; the list only looks until OK or
 * a click. It asks for every effect in `requests` and performs none.
 */
const watchScreenReduce = <C extends LineupChannel>(
  state: WatchScreenState,
  event: WatchScreenEvent,
  lineup: ChannelLineup<C>,
): WatchScreenStep => {
  const step = reduceStep(state, event, lineup);
  return { ...step, state: withoutHiddenThird(step.state, lineup) };
};

const reduceStep = <C extends LineupChannel>(
  state: WatchScreenState,
  event: WatchScreenEvent,
  lineup: ChannelLineup<C>,
): WatchScreenStep => {
  const { at } = event;
  // Digits already due commit first, when an event comes before the tick that was due for them.
  if (event.type !== "tick" && state.digits && at >= state.digits.commitAt) {
    const due = watchScreenReduce(state, { at: state.digits.commitAt, type: "tick" }, lineup);
    const after = watchScreenReduce(due.state, event, lineup);
    return { requests: [...due.requests, ...after.requests], state: after.state };
  }
  // Everything but the clock and the picture is activity, and wakes the screen.
  const woken: WatchScreenState =
    event.type === "tick" || event.type === "picture" || event.type === "playback"
      ? state
      : { ...state, activeAt: at };
  const stay = (next: WatchScreenState = woken): WatchScreenStep => ({ requests: [], state: next });
  const ask = (next: WatchScreenState, ...requests: WatchRequest[]): WatchScreenStep => ({
    requests,
    state: next,
  });
  const handTo = (s: WatchScreenState, to: WatchHandOff["to"]) => ({ seq: (s.handOff?.seq ?? 0) + 1, to });
  // A menu a key closes gives focus back to the control that opened it; the shortcuts sheet has
  // none.
  const closeMenuByKey = (s: WatchScreenState): WatchScreenState => {
    const name = s.menu?.name;
    const control = name && name !== "shortcuts" ? name : undefined;
    return { ...s, handOff: control ? handTo(s, control) : s.handOff, menu: undefined };
  };

  // A change of channel: the one left is remembered, and pause, digits and the hint are over.
  const changed = (s: WatchScreenState, channelId: string): WatchScreenState => ({
    ...s,
    behindMs: 0,
    digits: undefined,
    hintSeen: true,
    leftIds:
      s.watchedId === undefined
        ? s.leftIds
        : [s.watchedId, ...s.leftIds.filter((id) => id !== s.watchedId && id !== channelId)].slice(
            0,
            LEFT_LIMIT,
          ),
    list: channelListReduce(s.list, { channelId, type: "tuned" }, lineup).state,
    paused: false,
    pausedAt: undefined,
    watchedId: channelId,
  });
  const tuneTo = (s: WatchScreenState, channelId: string): WatchScreenStep =>
    channelId === s.watchedId ? stay(s) : ask(changed(s, channelId), { channelId, type: "tune" });

  // Nothing pauses or resumes while tuning in or under a cover: there is no picture to hold.
  const togglePause = (s: WatchScreenState): WatchScreenStep =>
    s.picture !== "playing"
      ? stay(s)
      : s.paused
        ? ask(
            {
              ...s,
              behindMs: s.behindMs + Math.max(0, at - (s.pausedAt ?? at)),
              paused: false,
              pausedAt: undefined,
            },
            { type: "resume" },
          )
        : ask({ ...s, paused: true, pausedAt: at }, { type: "pause" });

  const goLive = (s: WatchScreenState): WatchScreenStep =>
    offLive(s) ? ask({ ...s, behindMs: 0, paused: false, pausedAt: undefined }, { type: "goLive" }) : stay(s);

  const fullScreen = (s: WatchScreenState) => ask(s, { on: s.layout !== "full", type: "fullScreen" });

  const toggleMenu = (s: WatchScreenState, name: WatchMenuName): WatchScreenStep =>
    s.menu?.name === name
      ? stay({ ...s, menu: undefined })
      : ask({ ...s, menu: { name } }, { name, type: "menu" });

  // The list reported something, or a key was sent to it. A key that closes it hands focus back to
  // Channels when Channels opened it; a pointer that closes it leaves focus where it is.
  const listStep = (s: WatchScreenState, listEvent: ChannelListEvent, fromKey = false): WatchScreenStep => {
    const step = channelListReduce(s.list, listEvent, lineup);
    const closed = s.list.open && !step.state.open;
    const next: WatchScreenState = {
      ...s,
      handOff: closed && fromKey && s.listFromControl ? handTo(s, "channels") : s.handOff,
      list: step.state,
      listFromControl: closed ? false : s.listFromControl,
    };
    return step.tune ? tuneTo(next, step.tune) : stay(next);
  };

  const openList = (s: WatchScreenState, fromControl: boolean): WatchScreenStep => ({
    requests: [],
    state: {
      ...s,
      list: channelListReduce(s.list, { type: "open" }, lineup).state,
      listFromControl: fromControl,
    },
  });

  const openFind = (s: WatchScreenState): WatchScreenStep =>
    s.layout === "tv"
      ? stay(s)
      : stay({
          ...s,
          list: channelListReduce(s.list, { query: "", type: "find" }, lineup).state,
          listFromControl: s.list.open ? s.listFromControl : false,
        });

  const commitDigits = (s: WatchScreenState): WatchScreenStep => {
    const typed = s.digits?.typed ?? "";
    const cleared = { ...s, digits: undefined };
    const channel = lineup.channelByNumber(typed);
    if (!channel) return ask(cleared, { text: `There is no channel ${typed}`, type: "message" });
    if (!channel.playable) return ask(cleared, { text: unplayable(channel), type: "message" });
    return tuneTo(cleared, channel.id);
  };

  const lastChannel = (s: WatchScreenState): WatchScreenStep => {
    const last = lineup.lastChannel;
    if (!last) return ask(s, { text: "There is no last channel yet", type: "message" });
    if (!last.playable) {
      const text = last.channel ? unplayable(last.channel) : "The last channel is no longer listed";
      return ask(s, { text, type: "message" });
    }
    return tuneTo(s, last.id);
  };

  // A flip steps through the rows the list shows for the group in play, so Down is the row below:
  // number order, with the watched channel a stop in its place. Recent is newest first, which is
  // no order to flip in, so it flips through every channel in number order.
  const flip = (s: WatchScreenState, direction: -1 | 1): WatchScreenStep => {
    const group = s.list.group === "recent" ? "all" : s.list.group;
    const { rows } = channelListView({ ...s.list, group, query: null }, lineup);
    const at = rows.findIndex((channel) => channel.id === s.watchedId);
    const start = at >= 0 ? at : direction > 0 ? -1 : rows.length;
    for (let step = 1; step <= rows.length; step += 1) {
      const candidate = rows[(((start + direction * step) % rows.length) + rows.length) % rows.length];
      if (candidate?.playable && candidate.id !== s.watchedId) return tuneTo(s, candidate.id);
    }
    return stay(s);
  };

  // One step back: a menu's page, the menu, typed digits, then the list's own steps, then full screen.
  const back = (s: WatchScreenState): WatchScreenStep => {
    if (s.menu?.page !== undefined) return stay({ ...s, menu: { name: s.menu.name } });
    if (s.menu) return stay(closeMenuByKey(s));
    if (s.digits) return stay({ ...s, digits: undefined });
    // Back is a key or a remote's button.
    if (s.list.open) return listStep(s, { type: "back" }, true);
    return s.layout === "full" ? ask(s, { on: false, type: "fullScreen" }) : stay(s);
  };

  const press = (s: WatchScreenState, control: WatchControl): WatchScreenStep => {
    // A press away from an open menu closes it and does nothing else, unless the control opens a
    // menu itself: then it switches to its own, or closes its own.
    const opensMenu = control === "audio" || control === "more" || control === "subtitles";
    if (s.menu && !opensMenu) return stay({ ...s, menu: undefined });
    switch (control) {
      case "channels":
        return s.list.open ? listStep(s, { type: "close" }) : openList(s, true);
      case "guide":
        return ask(
          { ...s, list: channelListReduce(s.list, { type: "close" }, lineup).state, menu: undefined },
          { type: "guide" },
        );
      case "pause":
        return togglePause(s);
      case "audio":
      case "more":
      case "subtitles":
        return toggleMenu(s, control);
      case "volume":
        return ask(s, { type: "mute" });
      case "fullscreen":
        return fullScreen(s);
      case "goLive":
      case "live":
        return goLive(s);
      case "airplay":
        return ask(s, { type: "airplay" });
      // Back leaves the screen, which the caller does; the press asks nothing of the model.
      case "back":
        return stay(s);
    }
  };

  const key = (s: WatchScreenState, pressed: string): WatchScreenStep => {
    const letter = pressed.length === 1 ? pressed.toLowerCase() : pressed;
    const isBack = pressed === "Escape" || (s.layout === "tv" && pressed === "Backspace");
    // Full screen answers from anywhere, a menu and the open list included.
    if (letter === "f" && s.layout !== "tv") return fullScreen(s);
    if (isBack) return back(s);
    // Its own key closes an open menu, as Esc does; the menu keeps every other key.
    if (s.menu && MENU_KEYS[letter] === s.menu.name) return stay(closeMenuByKey(s));
    if (s.menu) return stay(s);

    if (/^[0-9]$/.test(pressed) && s.layout !== "tv") {
      const typed = `${s.digits?.typed ?? ""}${pressed}`.slice(0, digitsWidth(lineup));
      return stay({ ...s, digits: { commitAt: at + WATCH_DIGITS_MS, typed } });
    }
    if (s.digits && pressed === "Enter") return commitDigits(s);

    // The open list owns its keys; it took them already when focus was in it.
    const listEvent = s.list.open ? LIST_KEYS[pressed] : undefined;
    if (listEvent) {
      const step = listStep(s, listEvent, true);
      return step.state.list.open
        ? { ...step, state: { ...step.state, handOff: handTo(step.state, "list") } }
        : step;
    }

    switch (letter) {
      case "ArrowDown":
        return flip(s, 1);
      case "ArrowUp":
        return flip(s, -1);
      // A focused control presses itself with Space.
      case " ":
        return s.focus ? stay(s) : togglePause(s);
      case "/":
        return openFind(s);
      case "g":
        return ask(
          { ...s, list: channelListReduce(s.list, { type: "close" }, lineup).state },
          { type: "guide" },
        );
      case "l":
        return lastChannel(s);
      case "m":
        return ask(s, { type: "mute" });
      case "End":
        return goLive(s);
    }
    const menu = MENU_KEYS[letter];
    return menu ? toggleMenu(s, menu) : stay(s);
  };

  switch (event.type) {
    case "tick":
      return state.digits && at >= state.digits.commitAt ? commitDigits(state) : stay(state);
    case "pointerMove":
      return stay();
    case "pointerHold": {
      const others = woken.pointerOn.filter((region) => region !== event.region);
      return stay({ ...woken, pointerOn: event.held ? [...others, event.region] : others });
    }
    case "focus":
      return stay({ ...woken, focus: event.control });
    case "press":
      return press(woken, event.control);
    case "pictureClick": {
      // The playback before this click and before the one ahead of it: a double click that
      // follows goes back to the earlier, however far apart the two clicks were.
      const before: WatchPlayback = {
        behindMs: woken.behindMs,
        paused: woken.paused,
        pausedAt: woken.pausedAt,
      };
      const previous = woken.clicked && { at: woken.clicked.at, before: woken.clicked.before };
      const s = { ...woken, clicked: { at, before, previous } };
      // A click away from an open layer closes it and does nothing else.
      if (s.menu) return stay({ ...s, menu: undefined });
      if (s.list.open) return listStep(s, { type: "close" });
      return togglePause(s);
    }
    case "pictureDoubleClick": {
      // A double click is full screen alone: whatever its clicks did, the picture plays on, and
      // the moment they held it counts for nothing behind live.
      const first = woken.clicked?.previous ?? woken.clicked;
      const before = first?.before;
      const firstAt = first?.at ?? at;
      const behindMs = before
        ? before.behindMs + (before.paused ? Math.max(0, firstAt - (before.pausedAt ?? firstAt)) : 0)
        : woken.behindMs;
      const next = { ...woken, behindMs, clicked: undefined, paused: false, pausedAt: undefined };
      const fill = fullScreen(next);
      return woken.paused ? { ...fill, requests: [...fill.requests, { type: "resume" }] } : fill;
    }
    case "list":
      return listStep(woken, event.event, event.fromKey ?? false);
    case "menuPage":
      return stay(woken.menu ? { ...woken, menu: { name: woken.menu.name, page: event.page } } : woken);
    case "closeMenu":
      return stay(event.fromKey ? closeMenuByKey(woken) : { ...woken, menu: undefined });
    case "back":
      return back(woken);
    case "tuned":
      // An echo of the channel on screen is no change, and wakes nothing.
      return stay(event.channelId === state.watchedId ? state : changed(woken, event.channelId));
    case "layout": {
      // A new arrangement starts bare: the list and any menu close, as the prototype's do.
      const list = channelListReduce(woken.list, { type: "close" }, lineup).state;
      return stay({
        ...woken,
        layout: event.layout,
        list: { ...list, television: event.layout === "tv" },
        listFromControl: false,
        menu: undefined,
      });
    }
    case "picture":
      return stay({ ...state, picture: event.picture });
    // The player's own word on pause and live, whoever moved them: the truth the lower third and
    // Go live tell, taken silently. It is not input, so it wakes nothing; the viewer's own action
    // reaches the screen as the input it is.
    // A report from a channel the screen has left is not about the picture on screen. While the
    // picture stays paused the screen's own pause clock counts the pause, once, from where the
    // pause began: the player's growing lag is not added to it.
    case "playback": {
      if (event.channelId !== state.watchedId) return stay(state);
      const stillPaused = event.paused && state.paused;
      return stay({
        ...state,
        behindMs: stillPaused ? state.behindMs : event.live ? 0 : event.behindMs,
        paused: event.paused,
        pausedAt: event.paused ? (state.paused ? state.pausedAt : at) : undefined,
      });
    }
    case "key":
      return key(woken, event.key);
  }
};

/** What to draw at `now`. After a tick, pass `Math.max(now, nextAt)`. */
const watchScreenView = <C extends LineupChannel>(
  state: WatchScreenState,
  lineup: ChannelLineup<C>,
  now: number,
): WatchScreenView<C> => {
  const holding = held(state) || !RESTS.has(state.layout);
  const restAt = state.activeAt + WATCH_REST_MS;
  const awake = holding || now < restAt;
  const times = [state.digits?.commitAt, !holding && awake ? restAt : undefined].filter(
    (time): time is number => time !== undefined,
  );
  const typed = state.digits?.typed;
  return {
    awake,
    behindSeconds: state.paused ? 0 : Math.floor(state.behindMs / 1000),
    offLive: offLive(state),
    digits: typed === undefined ? undefined : { target: lineup.channelByNumber(typed), typed },
    hint: awake && !state.hintSeen && !state.list.open && state.menu === undefined,
    lastChannel: lineup.lastChannel,
    listOpen: state.list.open,
    menu: state.menu,
    nextAt: times.length ? Math.min(...times) : null,
    paused: state.paused,
  };
};

export {
  createWatchScreenState,
  WATCH_DIGITS_MS,
  WATCH_REST_MS,
  watchKeyOf,
  watchScreenReduce,
  watchScreenView,
};
