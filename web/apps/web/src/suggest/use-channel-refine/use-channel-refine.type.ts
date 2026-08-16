import type { ProposalDTO } from "@loomarr/api/models/proposalDTO";
import type { ProposalFailure } from "@loomarr/api/models/proposalFailure";
import type { SuggestionPhase } from "@loomarr/core/events";

interface ChannelRefine {
  jobId?: string;
  // The live phase from the SSE stream, or undefined before anything is running.
  phase?: SuggestionPhase;
  // The tool-loop round the phase belongs to (1-based; undefined outside the loop).
  round?: number;
  // The refined proposal, once it exists.
  proposal?: ProposalDTO;
  failure?: ProposalFailure;
  // True from submit until the run reaches a terminal phase.
  isRunning: boolean;
  error?: unknown;
  start: (channelId: string, change: string) => void;
  retry: () => void;
  reset: () => void;
}

export type { ChannelRefine };
