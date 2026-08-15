import type { SuggestionPhase } from "@loomarr/core/events";

interface GenerationProgressProps {
  phase: SuggestionPhase;
  // The tool-loop round the phase belongs to (1-based). Shown next to the active step
  // so a run that legitimately loops several times reads as progressing, not stuck.
  round?: number;
  // Whole seconds the run has been going. Shown once it passes a threshold, so a fast
  // run stays quiet and a slow one explains itself.
  elapsedSeconds?: number;
  error?: string;
  className?: string;
}

export type { GenerationProgressProps };
