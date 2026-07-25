import type { ProposalItem } from "@loomarr/api";

type ApprovalStatus = "pending" | "approving" | "denied";

interface ApprovalQueueItemProps {
  title: string;
  requestedBy?: string;
  summary?: string;
  acquisitions?: number;
  status?: ApprovalStatus;
  denyReason?: string;
  // The grounded picks behind this proposal, so an admin can SEE what they're
  // approving (titles, in-library vs. acquire, season windows) before it goes live —
  // the review the approval gate is supposed to enable (§7). Shown behind a toggle so
  // the queue stays scannable; omit to keep the compact row.
  lineup?: ProposalItem[];
  acquisitionItems?: ProposalItem[];
  onApprove?: () => void;
  // Deny carries the admin's optional reason — the same string this component already
  // renders back via `denyReason` once the proposal is denied. The two halves were
  // never connected: the display shipped, the capture didn't, so the field was always
  // empty in practice.
  onDeny?: (reason?: string) => void;
  className?: string;
}

export type { ApprovalQueueItemProps, ApprovalStatus };
