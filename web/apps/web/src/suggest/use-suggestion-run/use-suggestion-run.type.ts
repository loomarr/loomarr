import type { Intent } from "@loomarr/api/models/intent";
import type { ProposalJourneyFailureDTO } from "@loomarr/api/models/proposalJourneyFailureDTO";
import type { ProposalJourneyProposalDTO } from "@loomarr/api/models/proposalJourneyProposalDTO";
import type { SuggestionPhase } from "@loomarr/core/events";

interface SuggestionRun {
  // The live phase from the SSE stream, or undefined before anything is running.
  phase?: SuggestionPhase;
  // The tool-loop round the phase belongs to (1-based; undefined outside the loop).
  // Phases repeat as the model alternates thinking and searching, so this is what tells
  // a viewer a slow run is still advancing.
  round?: number;
  // The proposal the run produced, once it exists.
  proposal?: ProposalJourneyProposalDTO;
  // Bounded, requester-safe failure guidance from the authoritative Journey.
  failure?: ProposalJourneyFailureDTO;
  // Server-authorized actions; the UI does not reconstruct retry or settings policy.
  actions: string[];
  // True from submit until the run reaches a terminal phase.
  isRunning: boolean;
  // True when the run reached the terminal `failed` phase without producing a proposal.
  // Distinct from `error`: `error` is a failure of the SUBMIT request (the job never
  // started); `failed` is a job that started, ran, and errored mid-flight (e.g. the LLM
  // is unreachable). The submit succeeds (200 + jobId) in that case, so `error` stays
  // null and only this flag surfaces the runtime failure — without it the panel silently
  // drops back to an empty form.
  failed: boolean;
  error?: unknown;
  start: (intent: Intent) => void;
  retry: () => void;
  reset: () => void;
}

export type { SuggestionRun };
