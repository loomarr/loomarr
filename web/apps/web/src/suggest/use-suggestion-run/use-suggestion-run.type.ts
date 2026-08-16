import type { Intent } from "@loomarr/api/models/intent";
import type { ProposalDTO } from "@loomarr/api/models/proposalDTO";
import type { ProposalFailure } from "@loomarr/api/models/proposalFailure";
import type { SuggestionPhase } from "@loomarr/core/events";

interface SuggestionRun {
  jobId?: string;
  // The live phase from the SSE stream, or undefined before anything is running.
  phase?: SuggestionPhase;
  // The tool-loop round the phase belongs to (1-based; undefined outside the loop).
  // Phases repeat as the model alternates thinking and searching, so this is what tells
  // a viewer a slow run is still advancing.
  round?: number;
  // The proposal the run produced, once it exists.
  proposal?: ProposalDTO;
  intent?: Intent;
  failure?: ProposalFailure;
  // True from submit until the run reaches a terminal phase.
  isRunning: boolean;
  error?: unknown;
  start: (intent: Intent) => void;
  retry: () => void;
  reset: () => void;
}

export type { SuggestionRun };
