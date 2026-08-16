import type { Intent, ProposalDTO, ProposalFailure, ProposalJobDTO } from "@loomarr/api";
import type { SuggestionPhase } from "@loomarr/core/events";

interface ProposalJobTrackerOptions {
  jobId?: string;
  onJobIdChange?: (jobId: string | undefined) => void;
}

interface ProposalJobTracker {
  jobId?: string;
  job?: ProposalJobDTO;
  intent?: Intent;
  proposal?: ProposalDTO;
  failure?: ProposalFailure;
  phase?: SuggestionPhase;
  round?: number;
  isRunning: boolean;
  error?: unknown;
  track: (jobId: string) => void;
  reset: () => void;
}

export type { ProposalJobTracker, ProposalJobTrackerOptions };
