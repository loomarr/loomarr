import { getChannelTracksUrl } from "@loomarr/api/endpoints/channels";
import type { ChannelTracksOutputBody } from "@loomarr/api/models/channelTracksOutputBody";

// The tracks of the programme airing at an instant on one channel (#2088 decision 6): its audio,
// which of them the stream's Default and Alternate renditions carry, and its subtitles. A client
// asks once per programme, by its start, so a viewer behind live hears the choices of what they
// are watching.

interface ChannelTracksSource {
  load: (channelId: string, atMs: number, signal: AbortSignal) => Promise<ChannelTracksOutputBody>;
}

const createChannelTracksSource = (request: typeof globalThis.fetch): ChannelTracksSource => ({
  load: async (channelId, atMs, signal) => {
    const response = await request(getChannelTracksUrl(channelId, { at: atMs }), { method: "GET", signal });
    if (!response.ok) throw new Error(`Couldn't load the programme's tracks (${response.status}).`);
    return (await response.json()) as ChannelTracksOutputBody;
  },
});

export type { ChannelTracksSource };
export { createChannelTracksSource };
