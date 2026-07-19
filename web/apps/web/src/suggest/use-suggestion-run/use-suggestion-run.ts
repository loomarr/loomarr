import { type Intent, suggestionsApi } from "@loomarr/api";
import type { SuggestionPhase } from "@loomarr/core";
import { useState } from "react";
import { useLoomarrEventListener } from "@/events";
import type { SuggestionRun } from "./use-suggestion-run.type";

const TERMINAL: SuggestionPhase[] = ["done", "failed"];

// Owns one suggestion run: submit an intent, follow it through the live phases, and
// surface the proposal it produced.
//
// The phases (searching · reasoning · scoring) exist ONLY on the SSE stream — no GET
// returns them — so they come from the shared event fan-out. The proposal itself comes
// from the list, matched on jobId: `GET /v1/proposals` filters by status but not by
// job, and ProposalDTO carries jobId, so the match is client-side rather than an
// invented query param. The list is the approval queue, so it is bounded.
//
// Per §8 the stream is a latency optimisation, never the source of truth: a dropped
// `done` frame costs a spinner, not a proposal, because the list refetches on the same
// event and the proposal appears the moment it lands.
const useSuggestionRun = (): SuggestionRun => {
  const [jobId, setJobId] = useState<string | undefined>();
  const [phase, setPhase] = useState<SuggestionPhase | undefined>();

  const submit = suggestionsApi.useSubmitSuggestion();
  const proposals = suggestionsApi.useListProposals(
    { status: "submitted" },
    { query: { enabled: jobId !== undefined } },
  );

  useLoomarrEventListener({
    onSuggestion: (e) => {
      if (e.jobId === jobId) setPhase(e.phase);
    },
  });

  const rows = proposals.data?.status === 200 ? (proposals.data.data.proposals ?? []) : [];
  const proposal = jobId ? rows.find((p) => p.jobId === jobId) : undefined;

  return {
    phase,
    proposal,
    // A run stops being "running" once it lands a proposal or reports a terminal phase,
    // whichever the client learns first.
    isRunning: jobId !== undefined && proposal === undefined && !(phase && TERMINAL.includes(phase)),
    error: submit.error,
    start: (intent: Intent) => {
      setPhase(undefined);
      submit.mutate({ data: intent }, { onSuccess: (res) => res.status === 200 && setJobId(res.data.jobId) });
    },
    reset: () => {
      setJobId(undefined);
      setPhase(undefined);
    },
  };
};

export { useSuggestionRun };
