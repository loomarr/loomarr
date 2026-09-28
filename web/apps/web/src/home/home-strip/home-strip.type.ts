import type { GuideChannelTimeline } from "@loomarr/api/models/guideChannelTimeline";

interface HomeStripProps {
  // Every channel the guide read: the headline counts the live ones.
  channels: readonly GuideChannelTimeline[];
  // The guide has not answered yet ("Checking your channels…").
  loading: boolean;
  isAdmin: boolean;
}

export type { HomeStripProps };
