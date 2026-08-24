import type { ArtworkState, BadgeTone, Density } from "@loomarr/design-system";
import type { ReactNode } from "react";

interface ChannelIdentityData {
  channelLogoState: ArtworkState;
  channelName: string;
  channelNumber: string;
}

interface ChannelIdentityProps {
  channel: ChannelIdentityData;
  density?: Density;
  logo?: ReactNode;
}

interface ProgrammeIdentityData {
  badge?: { label: string; tone: BadgeTone };
  description?: string;
  episodeLabel?: string;
  seriesTitle?: string;
  timeLabel: string;
  title: string;
}

interface ProgrammeIdentityProps {
  density?: Density;
  programme: ProgrammeIdentityData;
}

export type {
  ChannelIdentityData,
  ChannelIdentityProps,
  ProgrammeIdentityData,
  ProgrammeIdentityProps,
};
