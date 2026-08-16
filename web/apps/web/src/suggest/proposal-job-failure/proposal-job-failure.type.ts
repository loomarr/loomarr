import type { ProposalFailure } from "@loomarr/api/models/proposalFailure";

interface ProposalJobFailureProps {
  failure: ProposalFailure;
  onRetry: () => void;
  onEdit?: () => void;
}

export type { ProposalJobFailureProps };
