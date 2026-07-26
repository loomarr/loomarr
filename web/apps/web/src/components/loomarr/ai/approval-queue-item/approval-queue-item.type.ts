import type { ApprovalEditDTO, ProposalItem } from "@loomarr/api";

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
  // Edit-before-approve (V25b). When supplied, the "Show picks" disclosure becomes the EDIT
  // surface — drop a title, add one via search, leave the requester a note — and this fires with
  // the resulting delta, or `undefined` when nothing has been modified. Omit it and the
  // disclosure stays read-only, which is what every non-admin surface wants.
  //
  // `undefined` is load-bearing, not laziness: the caller must send no body at all in that case
  // so an unmodified approval is byte-identical to the pre-V25 behaviour (see ProposalEdit).
  onEdit?: (edit: ApprovalEditDTO | undefined) => void;
  onApprove?: () => void;
  // Deny carries the admin's optional reason — the same string this component already
  // renders back via `denyReason` once the proposal is denied. The two halves were
  // never connected: the display shipped, the capture didn't, so the field was always
  // empty in practice.
  onDeny?: (reason?: string) => void;
  className?: string;
}

export type { ApprovalQueueItemProps, ApprovalStatus };
