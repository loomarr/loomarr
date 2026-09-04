import type { GuideController } from "@loomarr/core/guide";
import type { Density } from "@loomarr/design-system";

import type { GuideArtworkRenderer, GuideLogoRenderer } from "../guide.type";

interface GuideJourneyProps {
  controller: GuideController;
  density?: Density;
  onTune: (channelId: string) => void;
  preferredChannelId?: string;
  renderArtwork?: GuideArtworkRenderer;
  renderChannelLogo?: GuideLogoRenderer;
}

export type { GuideJourneyProps };
