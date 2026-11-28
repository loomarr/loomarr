import type { LineupChannel, LineupGroupKind } from "../channel-lineup";

/** Where the keyboard or remote cursor is: a row, a group tab, or nowhere. */
type ChannelListCursor =
  | { kind: "row"; channelId: string }
  /** `returnTo` is the row Down goes back to. */
  | { kind: "tab"; group: LineupGroupKind; returnTo: string | undefined }
  | { kind: "none" };

interface ChannelListState {
  open: boolean;
  watchedId: string | undefined;
  cursor: ChannelListCursor;
  group: LineupGroupKind;
  /** The text typed into find; null while find is closed. */
  query: string | null;
  /** A television has no typed find. */
  television: boolean;
}

interface ChannelListStateArgs {
  watchedId?: string | undefined;
  group?: LineupGroupKind | undefined;
  open?: boolean | undefined;
  television?: boolean | undefined;
}

type ChannelListEvent =
  | { type: "open" }
  | { type: "close" }
  /** Up (-1) or Down (1). */
  | { type: "move"; by: -1 | 1 }
  /** To the group tabs. */
  | { type: "tabs" }
  /** Left (-1) or Right (1): between tabs on the tabs, and straight between groups in the list. */
  | { type: "across"; by: -1 | 1 }
  | { type: "chooseTab"; group: LineupGroupKind }
  | { type: "find"; query: string }
  | { type: "clearFind" }
  | { type: "showAll" }
  /** OK, Enter, a click or a tap; without an id it acts on the cursor. */
  | { type: "choose"; channelId?: string | undefined }
  /** A remote or keyboard focus engine landed on a row or a tab. */
  | { type: "focusRow"; channelId: string }
  | { type: "focusTab"; group: LineupGroupKind }
  /** The watched channel changed, by this list or from outside. */
  | { type: "tuned"; channelId: string };

interface ChannelListStep {
  state: ChannelListState;
  /** Set when the caller should tune this channel. */
  tune?: string | undefined;
}

/** Why a list has nothing but, at most, the watched channel in it. */
type ChannelListNotice = "no-match" | "empty-recent" | "empty-favourites" | "empty-decade";

interface ChannelListView<C extends LineupChannel> {
  /** The channels in the order they are listed. */
  rows: readonly C[];
  /** The decade heading that precedes a row, keyed by channel id; only By decade has any. */
  headings: ReadonlyMap<string, number>;
  /** True while find lists results. */
  finding: boolean;
  notice: ChannelListNotice | undefined;
}

/** Where and when a pointer or finger pressed. */
interface ChannelListPress {
  x: number;
  y: number;
  /** Milliseconds from any fixed point. */
  at: number;
  touch: boolean;
}

export type {
  ChannelListCursor,
  ChannelListEvent,
  ChannelListNotice,
  ChannelListPress,
  ChannelListState,
  ChannelListStateArgs,
  ChannelListStep,
  ChannelListView,
};
