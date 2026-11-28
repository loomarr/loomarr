import type { ChannelDTO } from "@loomarr/api/models/channelDTO";
import { GuideAiringKind } from "@loomarr/api/models/guideAiringKind";
import type { GuideChannelTimeline } from "@loomarr/api/models/guideChannelTimeline";
import { formatProgrammeTitle } from "@loomarr/core/guide";
import type { WatchLineupChannel } from "./watch-lineup.type";

/**
 * The watching screen's channels: every channel the server lists, playable when this app can play
 * it and it is not paused, with what is on now and the minutes left from the guide when the guide
 * has read it. A channel the guide has not read yet is listed with its name and number alone.
 */
const watchLineupChannels = (
  channels: readonly ChannelDTO[],
  timelines: readonly GuideChannelTimeline[],
  nowMs: number,
): WatchLineupChannel[] => {
  const guide = new Map(timelines.map((timeline) => [timeline.channelId, timeline]));
  return channels.map((channel) => {
    const current = guide
      .get(channel.id)
      ?.airings.find(
        (airing) =>
          airing.kind === GuideAiringKind.program && airing.startMs <= nowMs && nowMs < airing.stopMs,
      );
    const offAir = channel.status === "paused" || channel.status === "detached";
    return {
      id: channel.id,
      name: channel.name,
      number: channel.number,
      playable: channel.inAppPlayable && !offAir,
      ...(channel.decade === undefined ? {} : { decade: channel.decade }),
      ...(current
        ? {
            // The one name an airing goes by everywhere: "Series · Title" (#2748).
            minutesLeft: Math.max(1, Math.round((current.stopMs - nowMs) / 60_000)),
            onNow: formatProgrammeTitle(current),
          }
        : {}),
    };
  });
};

export { watchLineupChannels };
