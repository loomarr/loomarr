import type { ArtworkState, BadgeTone, Density } from "@loomarr/design-system";
import type { ReactNode } from "react";

interface ProgrammeCardData {
  artworkState: ArtworkState;
  badge?: { label: string; tone: BadgeTone };
  channelName: string;
  channelNumber: string;
  description?: string;
  episodeLabel?: string;
  progressPercent?: number;
  seriesTitle?: string;
  timeLabel: string;
  title: string;
}

interface ProgrammeCardProps {
  artwork?: ReactNode;
  density?: Density;
  focused?: boolean;
  programme: ProgrammeCardData;
}

export type { ProgrammeCardData, ProgrammeCardProps };
