import * as proposalJobsApi from "@loomarr/api/endpoints/proposal-jobs";
import { unwrap } from "@loomarr/api/unwrap";
import type { SuggestionPhase } from "@loomarr/core/events";
import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useLoomarrEventListener } from "@/events/events-provider";
import { roundOf } from "../round";
import type { ProposalJobTracker, ProposalJobTrackerOptions } from "./use-proposal-job-tracker.type";

const POLL_MS = 1_000;

// One authoritative state machine for both origination and Refine. GET owns durable
// lifecycle, Intent, failure, and Proposal; SSE owns only the transient phase/round and
// accelerates the same GET. Polling active jobs is the reconnect/backstop path.
const useProposalJobTracker = ({
  jobId: controlledJobId,
  onJobIdChange,
}: ProposalJobTrackerOptions = {}): ProposalJobTracker => {
  const controlled = onJobIdChange !== undefined;
  const [localJobId, setLocalJobId] = useState(controlledJobId);
  const [phase, setPhase] = useState<SuggestionPhase | undefined>();
  const [round, setRound] = useState<number | undefined>();
  const jobId = controlled ? controlledJobId : localJobId;
  const queryClient = useQueryClient();

  const query = proposalJobsApi.useGetProposalJob(jobId ?? "", {
    query: {
      enabled: Boolean(jobId),
      refetchInterval: (current) => {
        const currentJob = unwrap(current.state.data);
        const generationActive = currentJob?.status === "queued" || currentJob?.status === "running";
        // A done generation is still unsettled while its Proposal awaits an admin decision.
        // Keep GET as the cross-browser backstop even when approval enqueues zero titles and
        // therefore produces no title frame for the requester to hear.
        const decisionPending = currentJob?.status === "done" && currentJob.proposal?.status === "submitted";
        return generationActive || decisionPending ? POLL_MS : false;
      },
    },
  });
  const job = unwrap(query.data);

  useLoomarrEventListener({
    onSuggestion: (event) => {
      if (event.jobId !== jobId) return;
      setPhase(event.phase);
      setRound(roundOf(event.round));
      void queryClient.invalidateQueries({ queryKey: proposalJobsApi.getGetProposalJobQueryKey(jobId) });
    },
  });

  const changeJob = (next: string | undefined) => {
    setPhase(undefined);
    setRound(undefined);
    if (controlled) onJobIdChange(next);
    else setLocalJobId(next);
  };

  return {
    jobId,
    job,
    intent: job?.intent,
    proposal: job?.proposal,
    failure: job?.status === "failed" ? job.failure : undefined,
    phase,
    round,
    isRunning:
      Boolean(jobId) &&
      query.error == null &&
      (job === undefined || job.status === "queued" || job.status === "running"),
    error: query.error,
    track: (id) => changeJob(id),
    reset: () => changeJob(undefined),
  };
};

export { useProposalJobTracker };
