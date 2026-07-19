import type { Intent, ProposalDTO } from "@loomarr/api";
import type { SuggestionPhase } from "@loomarr/core";

interface SuggestionRun {
  // The live phase from the SSE stream, or undefined before anything is running.
  phase?: SuggestionPhase;
  // The proposal the run produced, once it exists.
  proposal?: ProposalDTO;
  // True from submit until the run reaches a terminal phase.
  isRunning: boolean;
  error?: unknown;
  start: (intent: Intent) => void;
  reset: () => void;
}

export type { SuggestionRun };
