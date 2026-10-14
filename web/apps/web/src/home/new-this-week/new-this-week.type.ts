import type { ChannelDTO } from "@loomarr/api/models/channelDTO";
import type { TitleDTO } from "@loomarr/api/models/titleDTO";

interface NewThisWeekProps {
  // GET /v1/titles?since=…: arrivals, newest first (#1663).
  titles: readonly TitleDTO[];
  // Every channel; the ones created since `since` are this week's new channels.
  channels: readonly ChannelDTO[];
  since: number;
  // The signed-in person's name, so their own request reads "Your request".
  viewerName?: string;
  timeZone?: string;
}

export type { NewThisWeekProps };
