import type { GuideLayout, GuideSelection } from "@loomarr/core/guide";
import type { MyChannels } from "@loomarr/core/my-channels";
import type { BottomSheetProps, Density } from "@loomarr/design-system";
import type { ComponentType } from "react";

import type { GuideArtworkRenderer, GuideFilter } from "../guide.type";

interface GuideCompactProps {
  density?: Density;
  /**
   * How the selected programme docks: Android's strip with Watch beside it (5f, the default), or
   * the iPhone's sheet with its grabber and a full-width "Watch 21 now" (5d). The sheet is the
   * host's `BottomSheet`, handed in: its Animated and PanResponder weigh over 100 KiB on the web,
   * which only ever docks the strip, so the web bundle must not reach them through this module.
   */
  dock?: "strip" | ComponentType<BottomSheetProps>;
  /** The chosen filter. A personal one that has emptied reads as All. */
  filter: GuideFilter;
  layout: GuideLayout;
  /** The viewer's favourites and recents: the filters' counts and the rows they keep. */
  myChannels?: MyChannels;
  /** The clock the layout was computed against: the now line and its badge. */
  nowMs: number;
  onFilterChange: (filter: GuideFilter) => void;
  /** A cell was tapped: it becomes the selection, docked with Watch. */
  onSelect: (selection: GuideSelection) => void;
  /** The dock's Watch: tune the selected programme's channel. */
  onWatch: (channelId: string) => void;
  renderArtwork?: GuideArtworkRenderer;
  selection?: GuideSelection;
}

export type { GuideCompactProps };
