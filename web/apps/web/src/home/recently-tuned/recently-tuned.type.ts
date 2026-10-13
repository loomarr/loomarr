import type { ContinueWatchingDTO } from "@loomarr/api/models/continueWatchingDTO";
import type { GuideLayout } from "@loomarr/core/guide";

interface RecentlyTunedProps {
  // GET /v1/household/viewing's continueWatching: the caller's own last-tuned channel (X2).
  continueWatching?: ContinueWatchingDTO;
  layout: GuideLayout;
  nowMs: number;
  // Channels an active own-viewing card already represents (Watching now): the record says to
  // hide Recently tuned rather than show the same channel twice.
  activeChannelIds: ReadonlySet<string>;
}

export type { RecentlyTunedProps };
