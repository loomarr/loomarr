import type { GuideLayout, GuideSelection } from "@loomarr/core/guide";
import type { Density } from "@loomarr/design-system";

import type { GuideArtworkRenderer, GuideLogoRenderer } from "../guide.type";

interface GuideProgrammeDetailProps {
  density?: Density;
  /** Selection outline; disable when the initiating block owns focus in a preview. */
  focused?: boolean;
  layout: GuideLayout;
  /** The empty state's height; three guide rows. */
  minHeight?: number;
  renderArtwork?: GuideArtworkRenderer;
  renderChannelLogo?: GuideLogoRenderer;
  /** The programme to describe; without one, a prompt to choose one. */
  selection?: GuideSelection;
}

export type { GuideProgrammeDetailProps };
