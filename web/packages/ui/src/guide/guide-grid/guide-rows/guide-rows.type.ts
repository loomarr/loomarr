import type { GuideChannelLayout } from "@loomarr/core/guide";
import type { ReactElement } from "react";

// The grid's scrolling list of channel rows, one implementation per platform: FlatList on phone
// and TV (guide-rows.tsx), react-virtual on web (guide-rows.web.tsx).
interface GuideRowsProps {
  channels: GuideChannelLayout[];
  /** The row holding the selection; kept in view as the selection moves. Web only, as is onKey. */
  focusIndex?: number;
  /**
   * A key pressed inside the rows, with whether a modifier (Alt, Ctrl, Meta) was held. Returns
   * whether it was handled, so the page doesn't also scroll. Web only: TV navigates through its
   * own surface, and phones have no keys.
   */
  onKey?: (key: string, modified: boolean) => boolean;
  /** The time ruler, pinned above the rows while they scroll. */
  header: ReactElement;
  /** The ruler's height in pixels, border included. */
  headerHeight: number;
  /** Every row's height in pixels, border included. */
  rowHeight: number;
  renderRow: (channel: GuideChannelLayout) => ReactElement;
}

export type { GuideRowsProps };
