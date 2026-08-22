import * as proposalJobsApi from "@loomarr/api/endpoints/proposal-jobs";
import * as proposalsApi from "@loomarr/api/endpoints/proposals";
import type { Intent } from "@loomarr/api/models/intent";
import { unwrap } from "@loomarr/api/unwrap";
import type { SuggestionPhase } from "@loomarr/core/events";
import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useLoomarrEventListener } from "@/events/events-provider";
import { roundOf } from "../round";
import type { SuggestionRun } from "./use-suggestion-run.type";

const TERMINAL: SuggestionPhase[] = ["done", "failed"];

// Owns one suggestion run: submit an intent, follow it through the live phases, and
// surface the proposal it produced.
//
// The phases (searching · reasoning · scoring) exist only on the SSE stream, so they come
// from the shared event fan-out. Lifecycle, preserved Intent, and bounded failure come from
// `GET /v1/proposal-jobs/{jobId}`; SSE invalidates that read at terminal transitions and a
// short poll closes event-loss gaps. The proposal itself comes from the approval list,
// matched client-side on jobId because ProposalDTO already carries it.
//
// Per §8 the stream is a latency optimisation, never the source of truth. This hook tracks
// the phase only for the stepper. The app-lifetime event handler also invalidates the
// proposals prefix, while the job poll owns execution recovery. A dropped frame therefore
// costs at most a polling beat rather than hiding a durable failure.
const useSuggestionRun = (): SuggestionRun => {
  const [jobId, setJobId] = useState<string | undefined>();
  const [intent, setIntent] = useState<Intent | undefined>();
  const [phase, setPhase] = useState<SuggestionPhase | undefined>();
  const [round, setRound] = useState<number | undefined>();
  const queryClient = useQueryClient();

  const submit = proposalsApi.useSubmitProposal();
  const proposals = proposalsApi.useListProposals(
    { status: "submitted" },
    { query: { enabled: jobId !== undefined } },
  );
  const jobQuery = proposalJobsApi.useGetProposalJob(jobId ?? "", {
    query: {
      enabled: jobId !== undefined,
      retry: false,
      refetchInterval: (query) => {
        const response = query.state.data;
        return response?.status === 200 &&
          (response.data.status === "done" || response.data.status === "failed")
          ? false
          : 1_000;
      },
    },
  });

  useLoomarrEventListener({
    onSuggestion: (e) => {
      if (e.jobId !== jobId) return;
      setPhase(e.phase);
      setRound(roundOf(e.round));
      if (TERMINAL.includes(e.phase)) {
        void queryClient.invalidateQueries({ queryKey: proposalJobsApi.getGetProposalJobQueryKey(e.jobId) });
      }
    },
  });

  const rows = unwrap(proposals.data, (b) => b.proposals) ?? [];
  const proposal = jobId ? rows.find((p) => p.jobId === jobId) : undefined;
  const job = unwrap(jobQuery.data, (body) => body);

  const start = (nextIntent: Intent) => {
    setJobId(undefined);
    setIntent(nextIntent);
    setPhase(undefined);
    setRound(undefined);
    submit.mutate(
      { data: nextIntent },
      { onSuccess: (res) => res.status === 200 && setJobId(res.data.jobId) },
    );
  };

  const failed =
    job?.status === "failed" || (jobId !== undefined && proposal === undefined && phase === "failed");

  return {
    phase,
    round,
    proposal,
    // A run stops being "running" once it lands a proposal or reports a terminal phase,
    // whichever the client learns first.
    isRunning:
      submit.isPending ||
      (jobId !== undefined &&
        proposal === undefined &&
        !failed &&
        job?.status !== "done" &&
        !(phase && TERMINAL.includes(phase))),
    // The durable job read owns this state; the SSE phase is an immediate fallback while
    // that read catches up. Submit errors remain separate because no job exists for them.
    failed,
    failure: job?.failure,
    error: submit.error,
    start,
    retry: () => intent && start(intent),
    reset: () => {
      setJobId(undefined);
      setIntent(undefined);
      setPhase(undefined);
      setRound(undefined);
    },
  };
};

export { useSuggestionRun };
