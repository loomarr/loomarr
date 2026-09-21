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
}

export type { DevicePlaybackProfile, PlayerChannel, PlayerSource, PlayerSourcePort };
