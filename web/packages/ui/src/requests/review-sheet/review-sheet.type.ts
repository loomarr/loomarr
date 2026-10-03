import type { ApprovalEditDTO, ProposalDTO, PullDTO } from "@loomarr/core/requests";

import type { ReviewGroup, ReviewState } from "../review";

type ReviewSheetProps = {
  /** A decision is in flight: the sheet's actions wait for it. */
  busy: boolean;
  /** Approve this proposal with the admin's drops and note, on the one approve call. */
  onApproveEdited: (proposal: ProposalDTO, edit: ApprovalEditDTO) => void;
  /** Approve everything ticked in this group, and only this group. */
  onApproveSelected: (group: ReviewGroup) => void;
  onClose: () => void;
  /** A channel request's note reaches the requester; a filler download has no one to tell. */
  onDeny: (group: ReviewGroup, id: string, reason?: string) => void;
  proposals: readonly ProposalDTO[];
  pulls: readonly PullDTO[];
  review: ReviewState;
};

export type { ReviewSheetProps };
