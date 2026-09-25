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
  /** Signed address of the channel's latest still frame, for the switch overlay. Absent when the server sent none. */
  stillUri?: string;
  uri: string;
}

/** A source minted for a neighbour, plus whether its playlist and first assets were all fetched. */
interface WarmedSource extends PlayerSource {
  warmed: boolean;
}

interface PlayerSourcePort {
  mint: (
    channel: PlayerChannel,
    profile: DevicePlaybackProfile,
    signal: AbortSignal,
  ) => Promise<PlayerSource>;
  /**
   * Speculatively starts a channel's stream on the server without playing it, so a later tune finds
   * it running, and returns the exact signed source the real tune should reuse. Optional: a source
   * that cannot warm is simply never pre-warmed. A refusal (no capacity) is a miss, not an error:
   * it resolves with `warmed: false`.
   */
  warm?: (
    channel: PlayerChannel,
    profile: DevicePlaybackProfile,
    signal: AbortSignal,
  ) => Promise<WarmedSource | undefined>;
}

export type { DevicePlaybackProfile, PlayerChannel, PlayerSource, PlayerSourcePort, WarmedSource };
