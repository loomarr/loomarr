import { getChannelTimelineUrl } from "@loomarr/api/endpoints/channels";
import type { GuideAiring } from "@loomarr/api/models/guideAiring";
import type { TimelineOutputBody } from "@loomarr/api/models/timelineOutputBody";

// One channel's timeline for the player chrome (§9): its airings, pods and all, around now, and the
// server's clock at the moment it answered. A client that holds the answer projects that clock
// forward by its own elapsed time, so the playbar's live edge agrees with the server's.

interface ChannelTimeline {
  airings: readonly GuideAiring[];
  /** The server's clock when it answered. */
  serverNowMs: number;
  /** This device's clock when the answer arrived. */
  receivedAtMs: number;
}

interface ChannelTimelineSource {
  load: (channelId: string, signal: AbortSignal) => Promise<ChannelTimeline>;
}

const createChannelTimelineSource = (
  request: typeof globalThis.fetch,
  now: () => number = Date.now,
): ChannelTimelineSource => ({
  load: async (channelId, signal) => {
    const response = await request(getChannelTimelineUrl(channelId), { method: "GET", signal });
    if (!response.ok) throw new Error(`Couldn't load the channel's schedule (${response.status}).`);
    const body = (await response.json()) as TimelineOutputBody;
    return { airings: body.airings, receivedAtMs: now(), serverNowMs: body.serverNowMs };
  },
});

/** The server's clock now, projected from a timeline's answer by the time elapsed since. */
const serverNowOf = (timeline: ChannelTimeline, now: number = Date.now()) =>
  timeline.serverNowMs + Math.max(0, now - timeline.receivedAtMs);

export type { ChannelTimeline, ChannelTimelineSource };
export { createChannelTimelineSource, serverNowOf };
