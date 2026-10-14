import type { ReactNode } from "react";

import type { SurfChannelData } from "../surf-rail.type";

/** What the card reads of a channel. A `SurfChannelData` satisfies it; so does the tuning channel. */
type SurfChannelCardData = Pick<SurfChannelData, "channelName" | "channelNumber"> & {
  now?: Pick<
    NonNullable<SurfChannelData["now"]>,
    "progressPercent" | "remainingLabel" | "seriesTitle" | "title"
  >;
};

interface SurfChannelCardProps {
  channel: SurfChannelCardData;
  /** Draws the live dot: this is the channel on air for the viewer. */
  current: boolean;
  /** A ready logo image; absent draws the channel's monogram ident (#1659 decision N8). */
  logo?: ReactNode;
  /** The expanded form: programme, time left and progress. OK tunes the selected channel. */
  selected: boolean;
  /**
   * The selected card has the remote's focus, so it alone draws amber (border and number). A
   * selected card without it keeps its expanded form at rest (raised, hairline, muted number), so
   * a focused rail action is the only amber on screen (#1887).
   */
  focused: boolean;
}

export type { SurfChannelCardData, SurfChannelCardProps };
