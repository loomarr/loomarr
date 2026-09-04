import type { GuideController } from "@loomarr/core/guide";
import type { Density } from "@loomarr/design-system";
import type { ReactNode } from "react";

import type { SurfChannelData } from "../surf-rail.type";

interface SurfJourneyProps {
  clientVersion: string;
  controller: GuideController;
  currentChannelId?: string;
  density?: Density;
  /** Authoritative preference IDs. Omit when no preference contract is available. */
  favoriteChannelIds?: readonly string[];
  now?: () => number;
  onTune: (channelId: string) => void;
  playableChannelIds: readonly string[];
  recentChannelIds: readonly string[];
  renderArtwork?: (channel: SurfChannelData) => ReactNode;
  renderChannelLogo?: (channel: SurfChannelData) => ReactNode;
  serverVersion?: string;
}

export type { SurfJourneyProps };
