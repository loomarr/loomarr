import * as proposalsApi from "@loomarr/api/endpoints/proposals";
import type { Intent } from "@loomarr/api/models/intent";
import { unwrap } from "@loomarr/api/unwrap";
import type { SuggestionPhase } from "@loomarr/core/events";
import { useState } from "react";
import { useLoomarrEventListener } from "@/events/events-provider";
import { roundOf } from "../round";
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
// Per §8 the stream is a latency optimisation, never the source of truth. The phases ride
// the stream; the proposal rides the list. This hook only tracks the phase for the
// stepper — it does NOT refetch the list itself, because the app-lifetime stream already
// does: useLoomarrEvents invalidates the `/v1/proposals` prefix on every suggestion
// frame (events.ts), and the proposals query lives under that prefix, so the proposal is
// pulled in as the run progresses. A dropped frame therefore costs a beat, not a proposal
// — the next frame (or a manual reload) still surfaces it.
const useSuggestionRun = (): SuggestionRun => {
  const [jobId, setJobId] = useState<string | undefined>();
  const [phase, setPhase] = useState<SuggestionPhase | undefined>();
  const [round, setRound] = useState<number | undefined>();

  const submit = proposalsApi.useSubmitProposal();
  const proposals = proposalsApi.useListProposals(
    { status: "submitted" },
    { query: { enabled: jobId !== undefined } },
  );

  useLoomarrEventListener({
    onSuggestion: (e) => {
      if (e.jobId !== jobId) return;
      setPhase(e.phase);
      setRound(roundOf(e.round));
    },
  });

  const rows = unwrap(proposals.data, (b) => b.proposals) ?? [];
  const proposal = jobId ? rows.find((p) => p.jobId === jobId) : undefined;

  return {
    phase,
    round,
    proposal,
    // A run stops being "running" once it lands a proposal or reports a terminal phase,
    // whichever the client learns first.
    isRunning: jobId !== undefined && proposal === undefined && !(phase && TERMINAL.includes(phase)),
    // A job that reached the terminal `failed` phase without ever producing a proposal.
    // The submit succeeded (so `error` is null); the failure arrived on the SSE stream.
    // Without surfacing this, the panel falls through to a blank form as if nothing ran.
    failed: jobId !== undefined && proposal === undefined && phase === "failed",
    error: submit.error,
    start: (intent: Intent) => {
      setPhase(undefined);
      setRound(undefined);
      submit.mutate({ data: intent }, { onSuccess: (res) => res.status === 200 && setJobId(res.data.jobId) });
    },
    reset: () => {
      setJobId(undefined);
      setPhase(undefined);
      setRound(undefined);
    },
  };
};

export { useSuggestionRun };
