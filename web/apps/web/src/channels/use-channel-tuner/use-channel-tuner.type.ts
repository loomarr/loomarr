import type { ChannelDTO, ChannelNowNext } from "@loomarr/api";
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
  requestedChannel?: ChannelDTO;
  currentTitle?: string;
  attempt?: TuneAttempt;
  acknowledging: boolean;
  canSurf: boolean;
  ready: (channelId: string) => void;
  step: (direction: TuneDirection) => void;
  retry: () => void;
}

export type { TuneDirection, UseChannelTuner, UseChannelTunerOptions };
