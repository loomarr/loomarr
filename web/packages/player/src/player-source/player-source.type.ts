import type { ChannelDTO } from "@loomarr/api/models/channelDTO";
import type { DeviceProfileBody } from "@loomarr/api/models/deviceProfileBody";

type DevicePlaybackProfile = Omit<DeviceProfileBody, "$schema">;

type PlayerChannel = Pick<ChannelDTO, "id" | "inAppPlayable" | "name" | "number">;

interface PlayerSource {
  /** Unix milliseconds. Absent only when a malformed server response cannot be parsed. */
  expiresAt?: number;
  /** Transport-specific headers. Signed Loomarr HLS sources intentionally omit these. */
  headers?: Readonly<Record<string, string>>;
  /** Server-authored Unix milliseconds at mint time, used to keep native live chrome off device RTC. */
  serverTimeMs?: number;
  uri: string;
}

interface PlayerSourcePort {
  mint: (
    channel: PlayerChannel,
    profile: DevicePlaybackProfile,
    signal: AbortSignal,
  ) => Promise<PlayerSource>;
  /**
   * Speculatively starts a channel's stream on the server without playing it, so a later tune finds
   * it running. Optional: a source that cannot warm is simply never pre-warmed. Resolves whether or
   * not the server had capacity; a refusal is a miss, not an error.
   */
  warm?: (channel: PlayerChannel, profile: DevicePlaybackProfile, signal: AbortSignal) => Promise<void>;
}

export type { DevicePlaybackProfile, PlayerChannel, PlayerSource, PlayerSourcePort };
