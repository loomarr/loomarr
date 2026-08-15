import type { Intent, ProposalDTO } from "@loomarr/api";
import type { SuggestionPhase } from "@loomarr/core";

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
  error?: unknown;
  start: (intent: Intent) => void;
  reset: () => void;
}

export type { SuggestionRun };
