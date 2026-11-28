import type { ChannelLineup, LineupChannel, LineupGroupKind } from "../channel-lineup";

/** One block of a channel's row: a programme, or a run of one series. Breaks are not programmes. */
interface GuideViewProgramme {
  id: string;
  startMs: number;
  stopMs: number;
  title: string;
}

/** A lineup channel with the programmes its row draws, in time order. */
interface GuideViewChannel extends LineupChannel {
  programmes: readonly GuideViewProgramme[];
}

/** What the caller knows and the model does not hold: the schedule, the clock and the device. */
interface GuideViewContext<C extends GuideViewChannel> {
  lineup: ChannelLineup<C>;
  nowMs: number;
  /** How much of the day the screen shows at once. */
  widthMs: number;
  /** How far the schedule the caller holds goes. */
  untilMs: number;
  /** A clock time, as the screen words it. */
  formatTime: (ms: number) => string;
}

/** Where the keyboard or remote cursor is. */
type GuideViewCursor =
  /** `atMs` is the time of day Up and Down keep; `programmeId` is absent on a row with nothing drawn. */
  | { kind: "programme"; channelId: string; programmeId: string | undefined; atMs: number }
  | { kind: "tab"; group: LineupGroupKind };

interface GuideViewState {
  watchedId: string;
  group: LineupGroupKind;
  /** The start of the span shown. */
  startMs: number;
  cursor: GuideViewCursor;
  /** A television says OK where a keyboard says Enter. */
  television: boolean;
}

type GuideViewEvent =
  /** Up (-1) or Down (1). */
  | { type: "move"; by: -1 | 1 }
  /** Left (-1) or Right (1): along the row, or between tabs. */
  | { type: "across"; by: -1 | 1 }
  /** Earlier (-1) or Later (1). */
  | { type: "page"; by: -1 | 1 }
  /** OK or Enter, on the cursor. */
  | { type: "choose" }
  /** A programme's block was pressed: by a pointer, or `keyed` by OK or Enter on it. */
  | { type: "press"; channelId: string; programmeId: string; keyed?: boolean }
  /** A click or tap on a row's channel number and name. */
  | { type: "pressChannel"; channelId: string }
  /** A remote or keyboard focus engine landed on a block or a tab. */
  | { type: "focusProgramme"; channelId: string; programmeId: string }
  | { type: "focusTab"; group: LineupGroupKind }
  | { type: "chooseTab"; group: LineupGroupKind }
  | { type: "showAll" }
  /** Back or Esc: one step. */
  | { type: "back" }
  /** The watched channel changed, by this view or from outside. */
  | { type: "tuned"; channelId: string }
  /** The clock or the schedule moved on. */
  | { type: "refresh" };

interface GuideViewStep {
  state: GuideViewState;
  /** Set when the caller should tune this channel. */
  tune?: string | undefined;
  /** Set when the caller should return to the picture. */
  close?: boolean | undefined;
  /** Words for the caller to raise over the Guide. */
  message?: string | undefined;
}

/** What the header describes. */
interface GuideViewLook<C extends GuideViewChannel> {
  channel: C | undefined;
  programme: GuideViewProgramme | undefined;
  /** True when it is what is on now on the watched channel: looking is playing. */
  playing: boolean;
}

export type {
  GuideViewChannel,
  GuideViewContext,
  GuideViewCursor,
  GuideViewEvent,
  GuideViewLook,
  GuideViewProgramme,
  GuideViewState,
  GuideViewStep,
};
