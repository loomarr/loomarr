import type { SuggestionPhase } from "@loomarr/core/events";

interface GenerationProgressProps {
  phase: SuggestionPhase;
  // Retained at the shared boundary for event compatibility; the calm progress surface
  // deliberately does not expose model loop rounds to the user.
  round?: number;
  // Whole seconds the run has been going. Used only to add patient long-wait copy.
  elapsedSeconds?: number;
  error?: string;
  className?: string;
}

export type { GenerationProgressProps };
