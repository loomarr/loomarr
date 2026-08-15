import type { ChannelDTO, ChannelNowNext } from "@loomarr/api";
import type { TuneAttempt } from "../tuner-timing";

type TuneDirection = -1 | 1;

interface UseChannelTunerOptions {
  currentId: string;
  channels: ChannelDTO[];
  nowNext: ChannelNowNext[];
  onTune: (channel: ChannelDTO) => void;
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
