import { formatGuideTime, type GuideLayout } from "@loomarr/core/guide";

import type { SurfChannelData } from "../../../surf-rail";
import { surfGroupsFromGuide, surfPreviousChannel, watchingScheduleFromGuide } from "../../../surf-rail";
import type { WatchingScheduleData } from "../../watching-surface.type";

interface WatchingPanelDataArgs {
  channelId?: string;
  favouriteIds: readonly string[];
  layout?: GuideLayout;
  nowMs: number;
  playableChannelIds: readonly string[];
  recentIds: readonly string[];
}

interface WatchingPanelData {
  clockLabel: string;
  favourites: readonly SurfChannelData[];
  /** Where Previous goes: the newest playable recent that isn't this channel. */
  previousId?: string;
  schedule?: WatchingScheduleData;
}

/**
 * What a portrait phone's panel under the picture shows (#1785, mocks 5e/5g), read from the guide
 * the same way on the web and the phone: now and next for the tuned channel, the favourites with
 * what each has on, the channel Previous returns to, and the wall clock.
 */
const watchingPanelFromGuide = ({
  channelId,
  favouriteIds,
  layout,
  nowMs,
  playableChannelIds,
  recentIds,
}: WatchingPanelDataArgs): WatchingPanelData => {
  const groups = layout
    ? surfGroupsFromGuide({
        currentChannelId: channelId,
        favoriteChannelIds: favouriteIds,
        layout,
        nowMs,
        playableChannelIds,
        recentChannelIds: recentIds,
      })
    : [];
  return {
    clockLabel: formatGuideTime(nowMs, layout?.timezone),
    favourites: groups.find((group) => group.kind === "favourites")?.channels ?? [],
    previousId: surfPreviousChannel(channelId, recentIds, playableChannelIds),
    schedule: watchingScheduleFromGuide(layout, channelId, nowMs),
  };
};

export type { WatchingPanelData, WatchingPanelDataArgs };
export { watchingPanelFromGuide };
