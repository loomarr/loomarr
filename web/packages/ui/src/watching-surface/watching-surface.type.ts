import type { Density } from "@loomarr/design-system";
import type { PlayerSnapshot } from "@loomarr/player";
import type { ReactNode } from "react";

import type { ProgrammeIdentityData } from "../identity";

interface ChannelNumberEntry {
  channelName?: string;
  digits: string;
}

interface WatchingProgrammeData extends ProgrammeIdentityData {
  progressPercent?: number;
}

interface WatchingScheduleData {
  next?: Pick<ProgrammeIdentityData, "timeLabel" | "title">;
  now?: WatchingProgrammeData;
}

interface WatchingSurfaceProps {
  chromeVisible?: boolean;
  density: Density;
  loadError?: string;
  numberEntry?: ChannelNumberEntry;
  onChannelDown: () => void;
  onChannelUp: () => void;
  onDismissControls: () => void;
  onGoLive: () => void;
  onOpenGuide: () => void;
  onOpenSurf: () => void;
  onPause: () => void;
  onPlay: () => void;
  onPrevious: () => void;
  onRetry: () => void;
  onShowControls: () => void;
  player: ReactNode;
  snapshot: PlayerSnapshot;
  schedule?: WatchingScheduleData;
}

export type { ChannelNumberEntry, WatchingProgrammeData, WatchingScheduleData, WatchingSurfaceProps };
