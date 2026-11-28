import type { LineupChannel, LineupLastChannel } from "@loomarr/core/channel-lineup";
import type { TuneView } from "@loomarr/core/tune";
import type { WatchMenuName } from "@loomarr/core/watch-screen";
import type { ReactNode } from "react";

import type { ChannelListDetail } from "../channel-list";
import type { LowerThirdState } from "../lower-third";
import type { WatchScreenModel } from "../use-watch-screen";
import type { PlayerWatermark } from "../watching-surface/player-chrome/watermark-clearance";

/**
 * The layouts this part draws: a desktop at full screen, the list over the picture. A window, a
 * television and a phone are their own arrangements, to come.
 */
type WatchScreenLayout = "full";

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
  /**
   * The back control: an arrow named for where it goes (`label`), at the top leading corner over
   * the picture, resting with the rest of the chrome. Left out, there is none.
   */
  back?: { label: string; onPress: () => void } | undefined;
  /** Whether the screen is in full screen, which names the full-screen control; left out, the layout says. */
  fullScreen?: boolean | undefined;
  /**
   * The channel's logo, burned into a corner of the picture: the chrome in that corner steps clear
   * of it, and the chrome in the other corners stays where it is.
   */
  watermark?: PlayerWatermark | undefined;
  reducedMotion?: boolean | undefined;
  /** For story baselines only: draws the static without its flicker. */
  holdSnow?: boolean | undefined;
}

export type { WatchMenuSlot, WatchProgramme, WatchScreenLayout, WatchScreenProps };
