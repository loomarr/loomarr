import { getChannelTracksUrl, getGetChannelUrl } from "@loomarr/api/endpoints/channels";
import type { ChannelDTO } from "@loomarr/api/models/channelDTO";
import type { ChannelTracksOutputBody } from "@loomarr/api/models/channelTracksOutputBody";

import { audioChoices } from "./audio-options";

type AudioChoice = ReturnType<typeof audioChoices>[number];

interface ChannelAudioSource {
  /** The Audio menu for a channel: what the programme on air carries, the channel's language marked. */
  load: (channelId: string, signal: AbortSignal) => Promise<AudioChoice[]>;
}

/**
 * The player's Audio menu read from the server, for a client with no query cache (the phone and the
 * TV). Two reads, both a member's: the airing programme's tracks, and the channel itself for the
 * language an admin set on it. The web Watch page reads the same two and builds the same choices.
 */
const createChannelAudioSource = (request: typeof globalThis.fetch): ChannelAudioSource => {
  const read = async <T>(url: string, signal: AbortSignal) => {
    const response = await request(url, { method: "GET", signal });
    if (!response.ok) throw new Error(`Couldn't read the channel's audio (${response.status}).`);
    return (await response.json()) as T;
  };
  return {
    // As on the web, tracks that can't be read still leave the channel's language to show; only the
    // channel itself is required, since without it nothing can be marked as the one in use.
    load: async (channelId, signal) => {
      const [tracks, channel] = await Promise.all([
        read<ChannelTracksOutputBody>(getChannelTracksUrl(channelId), signal).catch((error: unknown) => {
          if (signal.aborted) throw error;
          return undefined;
        }),
        read<ChannelDTO>(getGetChannelUrl(channelId), signal),
      ]);
      return audioChoices(tracks?.audio ?? [], channel.policy?.playout?.audioLanguage);
    },
  };
};

export type { AudioChoice, ChannelAudioSource };
export { createChannelAudioSource };
