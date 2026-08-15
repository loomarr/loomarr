import * as channelsApi from "@loomarr/api/endpoints/channels";
import * as proposalsApi from "@loomarr/api/endpoints/proposals";
import { unwrap } from "@loomarr/api/unwrap";
import type { SuggestionPhase } from "@loomarr/core/events";
import { useState } from "react";
import { useLoomarrEventListener } from "@/events/events-provider";
import { roundOf } from "../round";
import type { ChannelRefine } from "./use-channel-refine.type";

const TERMINAL: SuggestionPhase[] = ["done", "failed"];

// Owns one channel refine run — a near-copy of useSuggestionRun (see its header comment
// for the full rationale), but the trigger is `POST /v1/channels/{id}/refine` instead of
// a fresh intent submission. The refined proposal arrives via the SAME async pipeline as
// any other suggestion: the phases (searching · reasoning · scoring) exist only on the
// SSE stream, and the proposal itself comes from the shared approval-queue list, matched
// on jobId (`GET /v1/proposals?status=submitted` — bounded, client-side match rather than
// an invented job-scoped query param).
//
// Per §8 the stream is a latency optimisation, never the source of truth. The phases ride
// the stream; the proposal rides the list. This hook only tracks the phase for the
// stepper — it does NOT refetch the list itself, because the app-lifetime stream already
// does: useLoomarrEvents invalidates the `/v1/proposals` prefix on every suggestion
// frame (events.ts), and the proposals query lives under that prefix, so the refined
// proposal is pulled in as the run progresses. A dropped frame therefore costs a beat,
// not a proposal — the next frame (or a manual reload) still surfaces it. (Rendered
// outside the events provider — a bare story/unit test — the listener is a no-op and the
// enable-fetch is the only load; that is a test artifact, not the app's behaviour.)
const useChannelRefine = (): ChannelRefine => {
  const [jobId, setJobId] = useState<string | undefined>();
  const [phase, setPhase] = useState<SuggestionPhase | undefined>();
  const [round, setRound] = useState<number | undefined>();

  const refine = channelsApi.useRefineChannel();
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
    error: refine.error,
    start: (channelId: string, change: string) => {
      setPhase(undefined);
      setRound(undefined);
      refine.mutate(
        { id: channelId, data: { change } },
        { onSuccess: (res) => res.status === 200 && setJobId(res.data.jobId) },
      );
    },
    reset: () => {
      setJobId(undefined);
      setPhase(undefined);
      setRound(undefined);
    },
  };
};

export { useChannelRefine };
