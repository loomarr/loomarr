import type { ProposalDTO } from "@loomarr/core/requests";

type ApprovalCardProps = {
  /** A decision on this proposal is in flight. */
  busy?: boolean;
  onApprove: () => void;
  onDeny: (reason?: string) => void;
  /** Present when more than one proposal waits: the card then offers selection for bulk approve. */
  onToggleSelected?: (selected: boolean) => void;
  proposal: ProposalDTO;
  selected?: boolean;
};

export type { ApprovalCardProps };
