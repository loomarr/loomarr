import type { LineupChannel, LineupLastChannel } from "@loomarr/core/channel-lineup";
import type { TuneView } from "@loomarr/core/tune";
import type { WatchMenuName } from "@loomarr/core/watch-screen";
import type { ReactNode } from "react";

import type { ChannelListDetail } from "../channel-list";
import type { StripAiring } from "../film-strip";
import type { LowerThirdState } from "../lower-third";
import type { WatchScreenModel } from "../use-watch-screen";

/**
 * The layouts this part draws: a desktop at full screen, the list over the picture, and a desktop
 * window, the list beside the picture and the film strip under it. A television and a phone are
 * their own arrangements, to come.
 */
type WatchScreenLayout = "full" | "window";

/** The named control that leaves the screen. */
interface WatchBack {
  label: string;
  onPress: () => void;
}

/** What a window's film strip and facts are drawn from. */
interface WatchSchedule<C extends LineupChannel> {
  /**
   * A channel's airings for its day, from before what is on now to past what the strip draws, so a
   * series run at either edge of the strip keeps its episode count.
   */
  airings: (channel: C) => readonly StripAiring[];
  /** A clock time. For a programme after midnight the caller's formatter adds the day. */
  formatTime: (ms: number) => string;
  /** Live, now. */
  nowMs: number;
}

/** What an open menu's slot is handed. */
interface WatchMenuSlot<C extends LineupChannel> {
  name: WatchMenuName;
  /** The inner page open, if any. */
  page: string | undefined;
  /** Where Last channel goes; `playable` false means it is drawn as unavailable. */
  lastChannel: LineupLastChannel<C> | undefined;
  /** Opens an inner page, or returns to the menu's first (`undefined`). */
  openPage: (page: string | undefined) => void;
  close: () => void;
}

/** What is on a channel now; the screen adds typed digits, tuning, pause and behind live. */
type WatchProgramme = Exclude<LowerThirdState, { kind: "typing" } | { kind: "tuning" }>;

interface WatchScreenProps<C extends LineupChannel> {
  layout: WatchScreenLayout;
  /** From `useWatchScreen`: the model's state and view, and where events go. */
  model: WatchScreenModel<C>;
  /** The player's picture, filling the screen. */
  picture: ReactNode;
  /** The tune's view at this moment (`@loomarr/core/tune`). */
  tune: TuneView;
  programme: (channel: C) => WatchProgramme;
  muted?: boolean | undefined;
  /** What each of the list's rows says about time left. */
  detail?: ((channel: C) => ChannelListDetail) | undefined;
  /** The open menu's contents, which the screen places over the quiet row. */
  renderMenu?: ((slot: WatchMenuSlot<C>) => ReactNode) | undefined;
  /** A cover over the picture: off air, not started, stream stopped. */
  cover?: ReactNode;
  reducedMotion?: boolean | undefined;
  /** For story baselines only: draws the static without its flicker. */
  holdSnow?: boolean | undefined;
  /**
   * The named back control. Over the picture it stands at the top leading corner; in a window it
   * leads the list's header, before the Guide control.
   */
  back?: WatchBack | undefined;
  /** A window's film strip and facts; left out, a window draws neither. */
  schedule?: WatchSchedule<C> | undefined;
}

export type { WatchBack, WatchMenuSlot, WatchProgramme, WatchSchedule, WatchScreenLayout, WatchScreenProps };
