import type { ApprovalEditDTO } from "@loomarr/api/models/approvalEditDTO";
import type { Proposal } from "@loomarr/api/models/proposal";
import type { ReactNode } from "react";

// The proposal shape is the orval-generated `Proposal` (from the BE's typed
// suggest.Proposal — 1:1, §12). Only the review UI's own status and prop interface
// live here.
type ProposalStatus = "draft" | "submitted" | "approved" | "denied" | "partially-edited";

interface ProposalReviewProps {
  proposal: Proposal;
  outlook?: ReactNode;
  edit?: ApprovalEditDTO;
  status?: ProposalStatus;
  selfService?: boolean;
  busy?: boolean;
  onApprove?: () => void;
  // Deny carries the admin's optional reason. The API has persisted `denyReason` and
  // ApprovalQueueItem has rendered it since day one — but every call site sent `{}`,
  // so the field was always empty and a member never learned why. The reason is
  // OPTIONAL by design: requiring one would make denying a chore and produce "no"
  // fifty times over, but offering one costs a click.
  onDeny?: (reason?: string) => void;
  onEdit?: (edit: ApprovalEditDTO | undefined) => void;
  onEditRequest?: () => void;
  className?: string;
}

export type { ProposalReviewProps, ProposalStatus };
