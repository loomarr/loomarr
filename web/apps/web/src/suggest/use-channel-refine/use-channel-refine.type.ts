import type { ProposalDTO } from "@loomarr/api/models/proposalDTO";
import type { ProposalJourneyFailureDTO } from "@loomarr/api/models/proposalJourneyFailureDTO";
import type { ProposalJourneyProgressDTO } from "@loomarr/api/models/proposalJourneyProgressDTO";
import type { SuggestionPhase } from "@loomarr/core/events";

interface ChannelRefine {
  channelId?: string;
  // The live phase from the SSE stream, or undefined before anything is running.
  phase?: SuggestionPhase;
  // The tool-loop round the phase belongs to (1-based; undefined outside the loop).
  round?: number;
  // The server's live snapshot while generating (stage, search terms, streamed picks).
  progress?: ProposalJourneyProgressDTO;
  // The refined proposal, once it exists.
  proposal?: ProposalDTO;
  failure?: ProposalJourneyFailureDTO;
  actions: string[];
  // True from submit until the run reaches a terminal phase.
  isRunning: boolean;
  error?: unknown;
  start: (channelId: string, change: string) => void;
  reset: () => void;
}

export type { ChannelRefine };
