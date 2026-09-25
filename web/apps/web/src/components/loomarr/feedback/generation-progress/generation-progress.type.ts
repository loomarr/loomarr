import type { ProposalJourneyProgressDTO } from "@loomarr/api/models/proposalJourneyProgressDTO";
import type { SuggestionPhase } from "@loomarr/core/events";

interface GenerationProgressProps {
  phase: SuggestionPhase;
  // Retained at the shared boundary for event compatibility; the calm progress surface
  // deliberately does not expose model loop rounds to the user.
  round?: number;
  // Whole seconds the run has been going. Used only to add patient long-wait copy, and as the
  // clock when there is no server `progress.startedAt` to count from.
  elapsedSeconds?: number;
  // The server's live snapshot of the run (GET /v1/proposal-jobs/{jobId}): the real stage, what
  // was searched, and each title as it is chosen. Absent until the first read lands; the surface
  // then falls back to the SSE `phase` alone.
  progress?: ProposalJourneyProgressDTO;
  error?: string;
  className?: string;
}

export type { GenerationProgressProps };
