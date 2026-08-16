import type { ProposalFailure } from "@loomarr/api/models/proposalFailure";

interface ProposalJobFailureProps {
  failure: ProposalFailure;
  isAdmin?: boolean;
  onRetry: () => void;
  onEdit?: () => void;
}

export type { ProposalJobFailureProps };
