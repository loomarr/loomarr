import type { ProposalItemView, ProposalMediaType, ProposalScores, ProposalView } from "@loomarr/core";

// The proposal data contracts (ProposalView/ProposalItemView/…) live in @loomarr/core —
// shared with packages/fixtures and the future mobile app (§4.2). The UI status and the
// web prop interface stay here; the data types are re-exported for barrel consumers.
type ProposalStatus = "draft" | "submitted" | "approved" | "denied" | "partially-edited";

interface ProposalReviewProps {
  proposal: ProposalView;
  status?: ProposalStatus;
  busy?: boolean;
  onApprove?: () => void;
  onDeny?: () => void;
  onEditItem?: (item: ProposalItemView) => void;
  className?: string;
}

export type {
  ProposalItemView,
  ProposalMediaType,
  ProposalReviewProps,
  ProposalScores,
  ProposalStatus,
  ProposalView,
};
