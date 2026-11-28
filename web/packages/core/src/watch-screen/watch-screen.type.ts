import type { LineupChannel, LineupGroupKind, LineupLastChannel } from "../channel-lineup";
import type { ChannelListEvent, ChannelListState } from "../channel-list";

/**
 * How the screen is laid out: `full` a desktop at full screen, `window` a desktop window, `tv` a
 * television, `portrait` and `landscape` a phone upright and on its side.
 */
type WatchLayout = "full" | "window" | "tv" | "portrait" | "landscape";

/** The quiet row's controls, and the lower third's Go live (`live`). */
type WatchControl =
  | "airplay"
  | "audio"
  | "channels"
  | "fullscreen"
  | "guide"
  | "live"
  | "more"
  | "pause"
  | "subtitles"
  | "volume";

/** The menus the screen opens; `shortcuts` is the sheet of keys. */
type WatchMenuName = "audio" | "more" | "shortcuts" | "subtitles";

interface WatchMenu {
  name: WatchMenuName;
  /** An inner page the menu has opened, named by the menu; Back returns to the menu from it. */
  page?: string | undefined;
}

/** Where the screen asks focus to go: the list's cursor, or the Channels control that opened it. */
interface WatchHandOff {
  to: "list" | "channels";
  /** Counts the hand-offs, so a part knows when to move focus again. */
  seq: number;
}

interface WatchScreenState {
  layout: WatchLayout;
  /** When something last happened; the bare picture rests `WATCH_REST_MS` after it. */
  activeAt: number;
  watchedId: string | undefined;
  /**
   * The channels this screen has left, newest first. The caller builds the lineup with these as its
   * `leftIds`, so the lineup's `lastChannel` is the one this screen left.
   */
  leftIds: readonly string[];
  /** The channel list's own state; it is open while `list.open`. */
  list: ChannelListState;
  /** True when Channels opened the list, so closing it gives focus back to Channels. */
  listFromControl: boolean;
  menu: WatchMenu | undefined;
  /** The control holding focus. */
  focus: WatchControl | undefined;
  /** The pointer rests on the controls. */
  pointerHeld: boolean;
  /** Digits typed and not yet committed, and when they commit by themselves. */
  digits: { typed: string; commitAt: number } | undefined;
  paused: boolean;
  /** When the picture paused; set only while paused. */
  pausedAt: number | undefined;
  /** How far behind live the picture plays, from earlier pauses. */
  behindMs: number;
  /** The first-run hint is gone for this session. */
  hintSeen: boolean;
  handOff: WatchHandOff | undefined;
}

interface WatchScreenStateArgs {
  layout: WatchLayout;
  at: number;
  watchedId?: string | undefined;
  group?: LineupGroupKind | undefined;
  /** The hint was seen in an earlier session. */
  hintSeen?: boolean | undefined;
}

/** What happened. Every event says when, in milliseconds from any fixed point. */
type WatchScreenEvent =
  /** A key no control, field or open list took: `watchKeyOf` says which are. */
  | { type: "key"; key: string; at: number }
  | { type: "pointerMove"; at: number }
  /** The pointer came to rest on the controls, or left them. */
  | { type: "pointerHold"; held: boolean; at: number }
  /** A control took focus, or focus left the controls. */
  | { type: "focus"; control: WatchControl | undefined; at: number }
  /** A quiet-row control or Go live was pressed. */
  | { type: "press"; control: WatchControl; at: number }
  | { type: "pictureClick"; at: number }
  | { type: "pictureDoubleClick"; at: number }
  /** What the channel list reported. */
  | { type: "list"; event: ChannelListEvent; at: number }
  /** An open menu moved to one of its inner pages, or back to its first (`undefined`). */
  | { type: "menuPage"; page: string | undefined; at: number }
  | { type: "closeMenu"; at: number }
  /** Back or Esc, from a control that has no key of its own: one step. */
  | { type: "back"; at: number }
  /** The watched channel changed from outside the screen. */
  | { type: "tuned"; channelId: string; at: number }
  | { type: "layout"; layout: WatchLayout; at: number }
  /** The time `nextAt` named has come. It is not activity: it wakes nothing. */
  | { type: "tick"; at: number };

/** What the screen asks the caller to do. It does none of it itself. */
type WatchRequest =
  | { type: "tune"; channelId: string }
  | { type: "pause" }
  | { type: "resume" }
  | { type: "goLive" }
  | { type: "guide" }
  | { type: "fullScreen"; on: boolean }
  | { type: "mute" }
  | { type: "airplay" }
  | { type: "menu"; name: WatchMenuName }
  | { type: "message"; text: string };

interface WatchScreenStep {
  state: WatchScreenState;
  requests: readonly WatchRequest[];
}

interface WatchScreenView<C extends LineupChannel> {
  /** The lower third, quiet row and hint show; false while the bare picture rests. */
  awake: boolean;
  /** The first-run hint shows. */
  hint: boolean;
  listOpen: boolean;
  menu: WatchMenu | undefined;
  /** The digits typed and the channel they name, if any. */
  digits: { typed: string; target: C | undefined } | undefined;
  paused: boolean;
  /** Whole seconds behind live while playing; 0 at live and while paused. */
  behindSeconds: number;
  /** Where `L` goes; `playable` false means it does nothing. */
  lastChannel: LineupLastChannel<C> | undefined;
  /** When the view next changes by itself: send a `tick` then. Null while it waits for an event. */
  nextAt: number | null;
}

/** A key as the platform reports it, before the screen knows whether it is its own. */
interface WatchKeyInput {
  key: string;
  altKey?: boolean;
  ctrlKey?: boolean;
  metaKey?: boolean;
  /** A control or the list already took it. */
  defaultPrevented?: boolean;
  /** The key is being typed into a text field. */
  inField?: boolean;
  /** An input method is building a character. */
  composing?: boolean;
}

export type {
  WatchControl,
  WatchHandOff,
  WatchKeyInput,
  WatchLayout,
  WatchMenu,
  WatchMenuName,
  WatchRequest,
  WatchScreenEvent,
  WatchScreenState,
  WatchScreenStateArgs,
  WatchScreenStep,
  WatchScreenView,
};
