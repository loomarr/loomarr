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
  /** The focused/expanded form: programme, time left and progress. */
  selected: boolean;
}

export type { SurfChannelCardData, SurfChannelCardProps };
