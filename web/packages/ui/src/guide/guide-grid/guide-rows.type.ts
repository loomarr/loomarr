import type { GuideChannelLayout } from "@loomarr/core/guide";
import type { ReactElement } from "react";

// The grid's scrolling list of channel rows, one implementation per platform: FlatList on phone
// and TV (guide-rows.tsx), react-virtual on web (guide-rows.web.tsx).
interface GuideRowsProps {
  channels: GuideChannelLayout[];
  /** The time ruler, pinned above the rows while they scroll. */
  header: ReactElement;
  /** The ruler's height in pixels, border included. */
  headerHeight: number;
  /** Every row's height in pixels, border included. */
  rowHeight: number;
  renderRow: (channel: GuideChannelLayout) => ReactElement;
}

export type { GuideRowsProps };
