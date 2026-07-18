import type { Proposal, ProposalItem } from "@loomarr/api";

// The proposal shape is the orval-generated `Proposal` (from the BE's typed
// suggest.Proposal — 1:1, §12). Only the review UI's own status and prop interface
// live here.
type ProposalStatus = "draft" | "submitted" | "approved" | "denied" | "partially-edited";

interface ProposalReviewProps {
  proposal: Proposal;
  status?: ProposalStatus;
  busy?: boolean;
  onApprove?: () => void;
  onDeny?: () => void;
  onEditItem?: (item: ProposalItem) => void;
  className?: string;
}

export type { ProposalReviewProps, ProposalStatus };
