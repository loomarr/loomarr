import type { ArtworkState, Density } from "@loomarr/design-system";
import type { ReactNode } from "react";

import type { ChannelIdentityData, ProgrammeIdentityData } from "../identity";

interface ProgrammeCardData extends ChannelIdentityData, ProgrammeIdentityData {
  artworkState: ArtworkState;
  progressPercent?: number;
}

interface ProgrammeCardProps {
  artwork?: ReactNode;
  channelLogo?: ReactNode;
  density?: Density;
  focused?: boolean;
  programme: ProgrammeCardData;
}

export type { ProgrammeCardData, ProgrammeCardProps };
