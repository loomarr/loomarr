import type { LineupChannel, LineupGroupKind, LineupLastChannel } from "../channel-lineup";
import type { ChannelListEvent, ChannelListState } from "../channel-list";

/**
 * How the screen is laid out: `full` a desktop at full screen, `window` a desktop window, `tv` a
 * television, `portrait` and `landscape` a phone upright and on its side.
 */
type WatchLayout = "full" | "window" | "tv" | "portrait" | "landscape";

/**
 * The quiet row's controls, and the lower third's Go live (`goLive`). A television's quiet row
 * leads with a Go live of its own (`live`); the two are told apart because only the lower third's
 * goes when the lower third does. `back` is the named back control at the top leading corner, which
 * leaves the screen: the caller acts on its press, and the model only hears its focus.
 */
type WatchControl =
  | "airplay"
  | "back"
  | "audio"
  | "channels"
  | "fullscreen"
  | "goLive"
  | "guide"
  | "live"
  | "more"
  | "pause"
  | "subtitles"
  | "volume";

/** The menus the screen opens; `shortcuts` is the sheet of keys. */
type WatchMenuName = "audio" | "more" | "shortcuts" | "subtitles";

/** What the picture is doing: `playing` a picture, `tuning` in with nothing yet, `covered` by a cover. */
type WatchPicture = "playing" | "tuning" | "covered";

/**
 * The parts of the screen the pointer can rest on: the open list takes the lower third's place, and
 * `back` is the named back control.
 */
type WatchPointerRegion = "back" | "lowerThird" | "quietRow";

/** How far the picture is from live, as it was before a click on it. */
interface WatchPlayback {
  paused: boolean;
  pausedAt: number | undefined;
  behindMs: number;
}

interface WatchMenu {
  name: WatchMenuName;
  /** An inner page the menu has opened, named by the menu; Back returns to the menu from it. */
  page?: string | undefined;
}

/**
 * Where the screen asks focus to go: the list's cursor, the Channels control that opened the list,
 * or the control whose menu a key closed.
 */
interface WatchHandOff {
  to: "list" | "channels" | "audio" | "more" | "subtitles";
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
  /** Where the pointer rests on the controls; anywhere here holds the screen awake. */
  pointerOn: readonly WatchPointerRegion[];
  /** Digits typed and not yet committed, and when they commit by themselves. */
  digits: { typed: string; commitAt: number } | undefined;
  paused: boolean;
  /** When the picture paused; set only while paused. */
  pausedAt: number | undefined;
  /** How far behind live the picture plays, from earlier pauses or the player's word. */
  behindMs: number;
  /** The first-run hint is gone for this session. */
  hintSeen: boolean;
  handOff: WatchHandOff | undefined;
  /** Only a playing picture pauses. */
  picture: WatchPicture;
  /**
   * The playback before each of the last two clicks on the picture. A double click always follows
   * exactly two clicks, so it goes back to the playback before the first of them, whatever the gap.
   */
  clicked:
    | { at: number; before: WatchPlayback; previous: { at: number; before: WatchPlayback } | undefined }
    | undefined;
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
  | { type: "pointerHold"; held: boolean; region: WatchPointerRegion; at: number }
  /**
   * A control took focus from the keyboard or a remote, or that focus left the controls. Focus a
   * pointer gave is not sent: it holds nothing awake.
   */
  | { type: "focus"; control: WatchControl | undefined; at: number }
  /** A quiet-row control or Go live was pressed. */
  | { type: "press"; control: WatchControl; at: number }
  | { type: "pictureClick"; at: number }
  | { type: "pictureDoubleClick"; at: number }
  /**
   * What the channel list reported. `fromKey` says a key caused it, so a list that a key closes
   * gives focus back to the Channels control that opened it; a pointer's close leaves focus alone.
   */
  | { type: "list"; event: ChannelListEvent; fromKey?: boolean; at: number }
  /** An open menu moved to one of its inner pages, or back to its first (`undefined`). */
  | { type: "menuPage"; page: string | undefined; at: number }
  /** The menu closed itself; `fromKey` says a key did it, so focus goes back to its control. */
  | { type: "closeMenu"; fromKey?: boolean; at: number }
  /** Back or Esc, from a control that has no key of its own: one step. */
  | { type: "back"; at: number }
  /** The watched channel changed from outside the screen. */
  | { type: "tuned"; channelId: string; at: number }
  | { type: "layout"; layout: WatchLayout; at: number }
  /** The time `nextAt` named has come. It is not activity: it wakes nothing. */
  | { type: "tick"; at: number }
  /** What the picture is doing changed. Like the clock, it wakes nothing. */
  | { type: "picture"; picture: WatchPicture; at: number }
  /**
   * What the player really does: paused or not, and how far behind live, however it got there (a
   * seek, Start over, the media keys, a stall, a Retry), and whether it plays at the live edge.
   * The model takes it as the truth; its own pause, resume and go-live stay requests. Like the
   * clock, it wakes nothing: only the viewer's own action, sent as the input it is, wakes the screen.
   */
  | {
      type: "playback";
      /** The channel the report is about: one the screen has left is dropped. */
      channelId: string;
      behindMs: number;
      paused: boolean;
      live: boolean;
      at: number;
    };

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
  /** Paused or behind live, as the lower third says: Go live is offered and End answers. */
  offLive: boolean;
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
  WatchPicture,
  WatchPlayback,
  WatchPointerRegion,
  WatchRequest,
  WatchScreenEvent,
  WatchScreenState,
  WatchScreenStateArgs,
  WatchScreenStep,
  WatchScreenView,
};
