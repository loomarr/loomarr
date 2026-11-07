import * as proposalJobsApi from "@loomarr/api/endpoints/proposal-jobs";
import * as proposalsApi from "@loomarr/api/endpoints/proposals";
import type { Intent } from "@loomarr/api/models/intent";
import { unwrap } from "@loomarr/api/unwrap";
import type { SuggestionPhase } from "@loomarr/core/events";
import { useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { useLoomarrEventListener } from "@/events/events-provider";
import { roundOf } from "../round";
import { clearSuggestionDraft, readSuggestionDraft, writeSuggestionDraft } from "../suggestion-draft";
import type { SuggestionRun, SuggestionRunJob } from "./use-suggestion-run.type";

// Owns one suggestion run: submit an intent, follow it through the live phases, and
// surface the proposal it produced.
//
// Detailed phases (searching · reasoning · scoring) are transient SSE hints. The
// Proposal Job Journey is the durable source of truth, polled while generating; SSE only
// invalidates that same read sooner. Which Job that is belongs to the caller (the Guide keeps
// it in the URL): a submit or resume reports the new Job through `onJob`, and the run stays
// pending until `onJob` settles, so no frame shows the form between the two.
const useSuggestionRun = ({ jobId, onJob }: SuggestionRunJob): SuggestionRun => {
  const queryClient = useQueryClient();
  // Keep the complete intent after an authorized edit clears the finished job. The form must
  // retain constraints; its description alone is not an honest retry of the request.
  const [intent, setIntent] = useState<Intent | undefined>(() =>
    jobId === undefined ? readSuggestionDraft() : undefined,
  );
  const [phase, setPhase] = useState<SuggestionPhase | undefined>();
  const [round, setRound] = useState<number | undefined>();

  const submit = proposalsApi.useSubmitProposal({
    mutation: {
      onSuccess: async (res) => {
        if (res.status !== 200) return;
        clearSuggestionDraft();
        await onJob(res.data.jobId);
      },
    },
  });
  const revision = proposalJobsApi.useReviseProposalJob({
    mutation: {
      // Keep the mutation pending until the authoritative Journey has been
      // refreshed. This prevents the old review from briefly becoming active
      // between the revision acknowledgement and the queued Journey read.
      onSuccess: async (response, variables) => {
        if (response.status !== 200) return;
        await queryClient.invalidateQueries({
          queryKey: proposalJobsApi.getGetProposalJobQueryKey(variables.jobId),
        });
      },
    },
  });
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
  const start = (intent: Intent) => {
    setIntent(intent);
    // The POST can reject before it creates a durable Proposal Job (for example when
    // TMDB grounding is missing). Keep the complete validated Intent across the setup
    // round trip until a Job exists to own it.
    writeSuggestionDraft(intent);
    setPhase(undefined);
    setRound(undefined);
    submit.mutate({ data: intent });
  };
  const revise = (nextIntent: Intent) => {
    if (!jobId) return;
    setIntent(nextIntent);
    setPhase(undefined);
    setRound(undefined);
    revision.mutate({ jobId, data: nextIntent });
  };

  return {
    jobId,
    phase,
    round,
    progress: journey?.progress,
    proposal: journey?.proposal,
    failure: journey?.failure,
    intent: journey?.intent ?? intent,
    actions: journey?.actions ?? [],
    isRunning:
      submit.isPending ||
      revision.isPending ||
      (jobId !== undefined && (!journey || journey.milestone === "generating")),
    failed: journey?.milestone === "failed",
    error: submit.error ?? revision.error ?? journeyQuery.error,
    start,
    revise,
    retry: () => {
      if (!journey) return;
      if (journey.proposal) revise(journey.intent);
      else start(journey.intent);
    },
    resume: (nextJobId: string) => {
      setIntent(undefined);
      setPhase(undefined);
      setRound(undefined);
      void onJob(nextJobId);
    },
    // Clears this run's own state. Leaving the Job is the caller's: it owns which Job is shown.
    reset: (preserveIntent = false) => {
      if (preserveIntent && journey?.intent) setIntent(journey.intent);
      submit.reset();
      revision.reset();
      setPhase(undefined);
      setRound(undefined);
      if (!preserveIntent) {
        setIntent(undefined);
        clearSuggestionDraft();
      }
    },
  };
};

export { useSuggestionRun };
