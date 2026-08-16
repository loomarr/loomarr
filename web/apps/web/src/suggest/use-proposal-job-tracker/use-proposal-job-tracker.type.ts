import type { Intent } from "@loomarr/api/models/intent";
import type { ProposalDTO } from "@loomarr/api/models/proposalDTO";
import type { ProposalFailure } from "@loomarr/api/models/proposalFailure";
import type { ProposalJobDTO } from "@loomarr/api/models/proposalJobDTO";
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
