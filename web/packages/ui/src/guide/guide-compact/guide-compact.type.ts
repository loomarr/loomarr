import type { GuideLayout, GuideSelection } from "@loomarr/core/guide";
import type { MyChannels } from "@loomarr/core/my-channels";
import type { Density } from "@loomarr/design-system";

import type { GuideArtworkRenderer, GuideFilter } from "../guide.type";

interface GuideCompactProps {
  density?: Density;
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
