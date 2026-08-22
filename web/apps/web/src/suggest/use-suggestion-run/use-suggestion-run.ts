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

const ACTIVE_JOB_KEY = "loomarr.activeProposalJob";

// Owns one suggestion run: submit an intent, follow it through the live phases, and
// surface the proposal it produced.
//
// The phases (searching · reasoning · scoring) exist ONLY on the SSE stream — no GET
// returns them — so they come from the shared event fan-out. The proposal itself comes
// from the list, matched on jobId: `GET /v1/proposals` filters by status but not by
// job, and ProposalDTO carries jobId, so the match is client-side rather than an
// invented query param. The list is the approval queue, so it is bounded.
//
// Per §8 the stream is a latency optimisation, never the source of truth. The phases ride
// the stream; the proposal rides the list. This hook only tracks the phase for the
// stepper — it does NOT refetch the list itself, because the app-lifetime stream already
// does: useLoomarrEvents invalidates the `/v1/proposals` prefix on every suggestion
// frame (events.ts), and the proposals query lives under that prefix, so the proposal is
// pulled in as the run progresses. A dropped frame therefore costs a beat, not a proposal
// — the next frame (or a manual reload) still surfaces it.
const useSuggestionRun = (): SuggestionRun => {
  const queryClient = useQueryClient();
  const [jobId, setJobIdState] = useState<string | undefined>(() =>
    typeof window === "undefined" ? undefined : (window.sessionStorage.getItem(ACTIVE_JOB_KEY) ?? undefined),
  );
  const [phase, setPhase] = useState<SuggestionPhase | undefined>();
  const [round, setRound] = useState<number | undefined>();

  const submit = proposalsApi.useSubmitProposal();
  const journeyQuery = proposalJobsApi.useGetProposalJob(jobId ?? "", {
    query: {
      enabled: jobId !== undefined,
      refetchInterval: (query) => {
        const response = query.state.data;
        return response?.status === 200 && response.data.milestone === "generating" ? 2_000 : false;
      },
    },
  });

  useLoomarrEventListener({
    onSuggestion: (e) => {
      if (e.jobId !== jobId) return;
      setPhase(e.phase);
      setRound(roundOf(e.round));
      void queryClient.invalidateQueries({ queryKey: proposalJobsApi.getGetProposalJobQueryKey(e.jobId) });
    },
  });

  const journey = unwrap(journeyQuery.data);
  const setJobId = (next: string | undefined) => {
    setJobIdState(next);
    if (typeof window === "undefined") return;
    if (next) window.sessionStorage.setItem(ACTIVE_JOB_KEY, next);
    else window.sessionStorage.removeItem(ACTIVE_JOB_KEY);
  };
  const start = (intent: Intent) => {
    setPhase(undefined);
    setRound(undefined);
    submit.mutate({ data: intent }, { onSuccess: (res) => res.status === 200 && setJobId(res.data.jobId) });
  };

  return {
    phase,
    round,
    proposal: journey?.proposal,
    failure: journey?.failure,
    actions: journey?.actions ?? [],
    isRunning: jobId !== undefined && (!journey || journey.milestone === "generating"),
    failed: journey?.milestone === "failed",
    error: submit.error ?? journeyQuery.error,
    start,
    retry: () => journey && start(journey.intent),
    reset: () => {
      setJobId(undefined);
      setPhase(undefined);
      setRound(undefined);
    },
  };
};

export { useSuggestionRun };
