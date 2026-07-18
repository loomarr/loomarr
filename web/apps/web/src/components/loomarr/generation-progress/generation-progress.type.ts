import type { SuggestionPhase } from "@loomarr/core";

interface GenerationProgressProps {
  phase: SuggestionPhase;
  error?: string;
  className?: string;
}

export type { GenerationProgressProps };
