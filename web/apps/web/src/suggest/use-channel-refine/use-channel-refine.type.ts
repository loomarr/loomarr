import type { ProposalDTO } from "@loomarr/api";
import type { SuggestionPhase } from "@loomarr/core";

interface ChannelRefine {
  // The live phase from the SSE stream, or undefined before anything is running.
  phase?: SuggestionPhase;
  // The tool-loop round the phase belongs to (1-based; undefined outside the loop).
  round?: number;
  // The refined proposal, once it exists.
  proposal?: ProposalDTO;
  // True from submit until the run reaches a terminal phase.
  isRunning: boolean;
  error?: unknown;
  start: (channelId: string, change: string) => void;
  reset: () => void;
}

export type { ChannelRefine };
