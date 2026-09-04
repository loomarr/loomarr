import type { ArtworkState, Density } from "@loomarr/design-system";
import type { ReactNode } from "react";

import type { ChannelIdentityData, ProgrammeIdentityData } from "../identity";

type SurfGroupKind = "all" | "favourites" | "recent";

interface SurfProgrammeData extends ProgrammeIdentityData {
  artworkState: ArtworkState;
  progressPercent?: number;
  remainingLabel?: string;
}

interface SurfChannelData extends ChannelIdentityData {
  id: string;
  next?: Pick<ProgrammeIdentityData, "timeLabel" | "title">;
  now?: SurfProgrammeData;
}

interface SurfGroupData {
  channels: readonly SurfChannelData[];
  kind: SurfGroupKind;
  label: string;
}

interface SurfSelection {
  channelId: string;
  group: SurfGroupKind;
}

interface SurfRailProps {
  clientVersion: string;
  currentChannelId?: string;
  density?: Density;
  groups: readonly SurfGroupData[];
  onFocusSelection: (selection: SurfSelection) => void;
  onTune: (channelId: string) => void;
  renderArtwork?: (channel: SurfChannelData) => ReactNode;
  renderChannelLogo?: (channel: SurfChannelData) => ReactNode;
  selection: SurfSelection;
  serverVersion?: string;
}

export type {
  SurfChannelData,
  SurfGroupData,
  SurfGroupKind,
  SurfProgrammeData,
  SurfRailProps,
  SurfSelection,
};
