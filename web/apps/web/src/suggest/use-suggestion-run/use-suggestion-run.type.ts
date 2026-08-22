import type { Intent } from "@loomarr/api/models/intent";
import type { ProposalDTO } from "@loomarr/api/models/proposalDTO";
import type { ProposalFailure } from "@loomarr/api/models/proposalFailure";
import type { SuggestionPhase } from "@loomarr/core/events";

interface SuggestionRun {
  // The live phase from the SSE stream, or undefined before anything is running.
  phase?: SuggestionPhase;
  // The tool-loop round the phase belongs to (1-based; undefined outside the loop).
  // Phases repeat as the model alternates thinking and searching, so this is what tells
  // a viewer a slow run is still advancing.
  round?: number;
  // The proposal the run produced, once it exists.
  proposal?: ProposalDTO;
  // True from submit until the run reaches a terminal phase.
  isRunning: boolean;
  // True when the run reached the terminal `failed` phase without producing a proposal.
  // Distinct from `error`: `error` is a failure of the SUBMIT request (the job never
  // started); `failed` is a job that started, ran, and errored mid-flight (e.g. the LLM
  // is unreachable). The submit succeeds (200 + jobId) in that case, so `error` stays
  // null and only this flag surfaces the runtime failure — without it the panel silently
  // drops back to an empty form.
  failed: boolean;
  // Bounded server-owned classification for a failed execution. Raw provider diagnostics
  // never cross the API boundary.
  failure?: ProposalFailure;
  error?: unknown;
  start: (intent: Intent) => void;
  // Re-submit the exact preserved Intent as a fresh execution. Failed jobs are deliberately
  // excluded from the cache, so this performs real generation again.
  retry: () => void;
  reset: () => void;
}

export type { SuggestionRun };
