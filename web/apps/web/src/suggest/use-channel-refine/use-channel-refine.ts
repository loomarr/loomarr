import * as channelsApi from "@loomarr/api/endpoints/channels";
import * as proposalJobsApi from "@loomarr/api/endpoints/proposal-jobs";
import type { ProposalDTO } from "@loomarr/api/models/proposalDTO";
import { unwrap } from "@loomarr/api/unwrap";
import type { SuggestionPhase } from "@loomarr/core/events";
import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useLoomarrEventListener } from "@/events/events-provider";
import { roundOf } from "../round";
import type { ChannelRefine } from "./use-channel-refine.type";

const ACTIVE_REFINE_KEY = "loomarr.activeChannelRefine";

interface ActiveRefine {
  jobId: string;
  channelId: string;
}

const storedRefine = (): ActiveRefine | undefined => {
  if (typeof window === "undefined") return undefined;
  try {
    const parsed = JSON.parse(
      window.sessionStorage.getItem(ACTIVE_REFINE_KEY) ?? "null",
    ) as Partial<ActiveRefine> | null;
    return parsed?.jobId && parsed.channelId
      ? { jobId: parsed.jobId, channelId: parsed.channelId }
      : undefined;
  } catch {
    return undefined;
  }
};

// Detailed phases are transient SSE hints; the job Journey is the durable
// result and recovery surface. It is persisted per channel for reload and
// polled while generating, while SSE only accelerates the same read.
const useChannelRefine = (): ChannelRefine => {
  const queryClient = useQueryClient();
  const [active, setActiveState] = useState<ActiveRefine | undefined>(storedRefine);
  const [phase, setPhase] = useState<SuggestionPhase | undefined>();
  const [round, setRound] = useState<number | undefined>();

  const refine = channelsApi.useRefineChannel();
  const journeyQuery = proposalJobsApi.useGetProposalJob(active?.jobId ?? "", {
    query: {
      enabled: active !== undefined,
      refetchInterval: (query) =>
        query.state.data?.status === 200 && query.state.data.data.milestone === "generating" ? 2_000 : false,
    },
  });

  useLoomarrEventListener({
    onSuggestion: (event) => {
      if (event.jobId !== active?.jobId) return;
      setPhase(event.phase);
      setRound(roundOf(event.round));
      void queryClient.invalidateQueries({
        queryKey: proposalJobsApi.getGetProposalJobQueryKey(event.jobId),
      });
    },
  });

  const journey = unwrap(journeyQuery.data);
  const proposal: ProposalDTO | undefined = journey?.proposal
    ? { ...journey.proposal, jobId: journey.jobId }
    : undefined;
  const terminalPhase = phase === "done" || phase === "failed";
  const setActive = (next: ActiveRefine | undefined) => {
    setActiveState(next);
    if (typeof window === "undefined") return;
    if (next) window.sessionStorage.setItem(ACTIVE_REFINE_KEY, JSON.stringify(next));
    else window.sessionStorage.removeItem(ACTIVE_REFINE_KEY);
  };

  return {
    channelId: active?.channelId,
    phase: phase ?? (journey?.milestone === "failed" ? "failed" : proposal ? "done" : undefined),
    round,
    proposal,
    failure: journey?.failure,
    actions: journey?.actions ?? [],
    // SSE is only a latency hint, but a terminal hint must stop the spinner while the
    // authoritative Journey refetch catches up. Reload/event loss still recovers from
    // the persisted milestone because phase starts undefined.
    isRunning: active !== undefined && !terminalPhase && (!journey || journey.milestone === "generating"),
    error: refine.error ?? journeyQuery.error,
    start: (channelId: string, change: string) => {
      setPhase(undefined);
      setRound(undefined);
      refine.mutate(
        { id: channelId, data: { change } },
        { onSuccess: (res) => res.status === 200 && setActive({ jobId: res.data.jobId, channelId }) },
      );
    },
    reset: () => {
      setActive(undefined);
      setPhase(undefined);
      setRound(undefined);
    },
  };
};

export { useChannelRefine };
