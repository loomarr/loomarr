import type { Density } from "@loomarr/design-system";

import type { SurfChannelData } from "../../surf-rail";
import type { WatchingScheduleData } from "../watching-surface.type";

interface WatchingPanelProps {
  /** A channel was tuned before this one, so Previous has somewhere to go. */
  canPrevious: boolean;
  /** More than one channel, so Channel − and Channel + have somewhere to go. */
  canSurf: boolean;
  channel: { name: string; number: string };
  /** The wall clock under the progress bar ("8:26"). */
  clockLabel?: string;
  density: Density;
  /** The viewer's favourite channels, each with what it has on now (surfGroupsFromGuide's). */
  favourites: readonly SurfChannelData[];
  /** The tuned channel is one of the viewer's favourites: the star beside its name is filled. */
  isFavourite?: boolean;
  /** Where playback sits against the live edge: paused and behind offer Go Live. */
  live: { lagSeconds: number; mode: "behind" | "live" | "paused" };
  onChannelDown: () => void;
  onChannelUp: () => void;
  onGoLive: () => void;
  onPause: () => void;
  onPlay: () => void;
  onPrevious: () => void;
  /** Star or unstar the tuned channel. Omit where the client can't write favourites: no star is drawn. */
  onToggleFavourite?: () => void;
  onTune: (channelId: string) => void;
  schedule?: WatchingScheduleData;
}

export type { WatchingPanelProps };
