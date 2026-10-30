import { getChannelSubtitlesUrl } from "@loomarr/api/endpoints/channels";
import type { ChannelSubtitlesOutputBody } from "@loomarr/api/models/channelSubtitlesOutputBody";
import type { ChannelSubtitlesParams } from "@loomarr/api/models/channelSubtitlesParams";

// One channel's subtitles sidecar (#2088 step 7): the airings in a window of its wall clock, each
// with the text tracks it carries and, for the track asked for, its cues.

interface ChannelSubtitlesSource {
  load: (
    channelId: string,
    window: ChannelSubtitlesParams,
    signal: AbortSignal,
  ) => Promise<ChannelSubtitlesOutputBody>;
}

const createChannelSubtitlesSource = (request: typeof globalThis.fetch): ChannelSubtitlesSource => ({
  load: async (channelId, window, signal) => {
    const response = await request(getChannelSubtitlesUrl(channelId, window), { method: "GET", signal });
    if (!response.ok) throw new Error(`Couldn't load the channel's subtitles (${response.status}).`);
    return (await response.json()) as ChannelSubtitlesOutputBody;
  },
});

export type { ChannelSubtitlesSource };
export { createChannelSubtitlesSource };
