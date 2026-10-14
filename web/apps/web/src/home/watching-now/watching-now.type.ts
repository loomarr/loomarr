import type { HouseholdViewingOutputBody } from "@loomarr/api/models/householdViewingOutputBody";
import type { GuideLayout } from "@loomarr/core/guide";
import type { ProgrammeCardViewer } from "@loomarr/ui";

interface WatchingNowProps {
  // GET /v1/household/viewing: named viewers are already limited to what the caller may see.
  viewing: HouseholdViewingOutputBody;
  // The guide Home already read, for each channel's current airing.
  layout: GuideLayout;
  nowMs: number;
}

// One viewing, before it is joined to its channel's airing.
interface WatchingCard {
  key: string;
  channelId: string;
  you: boolean;
  viewer: ProgrammeCardViewer;
}

export type { WatchingCard, WatchingNowProps };
