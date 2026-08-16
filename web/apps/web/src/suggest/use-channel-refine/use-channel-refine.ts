import * as channelsApi from "@loomarr/api/endpoints/channels";
import { useState } from "react";
import { useProposalJobTracker } from "../use-proposal-job-tracker";
import type { ChannelRefine } from "./use-channel-refine.type";

const useChannelRefine = (): ChannelRefine => {
  const tracker = useProposalJobTracker();
  const refine = channelsApi.useRefineChannel();
  const [last, setLast] = useState<{ channelId: string; change: string }>();

  const start = (channelId: string, change: string) => {
    setLast({ channelId, change });
    refine.mutate(
      { id: channelId, data: { change } },
      { onSuccess: (response) => response.status === 200 && tracker.track(response.data.jobId) },
    );
  };

  return {
    ...tracker,
    isRunning: refine.isPending || tracker.isRunning,
    error: refine.error ?? tracker.error,
    start,
    retry: () => last && start(last.channelId, last.change),
  };
};

export { useChannelRefine };
