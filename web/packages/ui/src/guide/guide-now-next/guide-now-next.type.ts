import type { GuideLayout } from "@loomarr/core/guide";
import type { ReactNode } from "react";

interface GuideNowNextProps {
  /**
   * The viewer's favourite channels: they sort to the top, then the rest by channel number
   * (#2787). The list has no filter chips; the grid keeps them.
   */
  favouriteIds?: readonly string[];
  layout: GuideLayout;
  /** The clock the layout was computed against: what is on now, and how far through it is. */
  nowMs: number;
  /** A row was long-pressed, or its ⋯ chosen: open the channel's info. */
  onOpenChannelInfo?: (channelId: string) => void;
  /** A row was tapped: tune its channel. */
  onWatch: (channelId: string) => void;
  /**
   * `wide` is the desktop list (the Direction mock's variant C, desktop): one row of three columns,
   * Channel, On now and Next, under their headings, with a ⋯ shown on hover or focus.
   */
  wide?: boolean;
}

interface GuidePhoneHeaderProps {
  /** The view switch, right of the title. */
  children?: ReactNode;
  subtitle: string;
  /** The view's own heading. Left out where the page heading already names the view. */
  title?: string;
  /** The desktop page's wider gutter. */
  wide?: boolean;
}

export type { GuideNowNextProps, GuidePhoneHeaderProps };
