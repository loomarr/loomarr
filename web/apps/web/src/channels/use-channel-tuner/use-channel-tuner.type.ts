import type { ChannelDTO } from "@loomarr/api/models/channelDTO";
import type { ChannelNowNext } from "@loomarr/api/models/channelNowNext";
import type { WarmedChannel } from "../channel-warmer";
import type { TuneAttempt } from "../tuner-timing";

type TuneDirection = -1 | 1;

interface UseChannelTunerOptions {
  currentId: string;
  channels: ChannelDTO[];
  nowNext: ChannelNowNext[];
  onTune: (channel: ChannelDTO) => void;
  warmChannel?: (channelId: string, signal: AbortSignal) => Promise<WarmedChannel | undefined>;
}

interface UseChannelTuner {
  channel?: ChannelDTO;
  currentTitle?: string;
  attempt?: TuneAttempt;
  canSurf: boolean;
  step: (direction: TuneDirection) => void;
  retry: () => void;
}

export type { TuneDirection, UseChannelTuner, UseChannelTunerOptions };
