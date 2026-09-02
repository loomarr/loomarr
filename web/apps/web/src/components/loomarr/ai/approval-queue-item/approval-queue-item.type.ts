import type { ApprovalEditDTO } from "@loomarr/api/models/approvalEditDTO";
import type { EpisodeSelection } from "@loomarr/api/models/episodeSelection";
import type { ProposalItem } from "@loomarr/api/models/proposalItem";
import type { RefusedPick } from "@loomarr/api/models/refusedPick";
import type { ReactNode } from "react";

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
  episodeSelectionPreview?: EpisodeSelection;
  // Picks the model grounded that this proposal's OWN audience ceiling cannot air (§4, #259).
  //
  // ⚠ Rendered OUTSIDE the "Show picks" disclosure, unlike the picks themselves. The picks are
  // detail you open when you want them; a refusal changes what approving MEANS — before this,
  // an admin approved seven titles and got five, with nothing on the card saying so.
  refused?: RefusedPick[];
  // Edit-before-approve (V25b). When supplied, the "Show picks" disclosure becomes the EDIT
  // surface — drop a title, add one via search, leave the requester a note — and this fires with
  // the resulting delta, or `undefined` when nothing has been modified. Omit it and the
  // disclosure stays read-only, which is what every non-admin surface wants.
  //
  // `undefined` is load-bearing, not laziness: the caller sends no human edit in that case.
  // The server may still ground a missing/crafted series selector from the original Intent.
  onEdit?: (edit: ApprovalEditDTO | undefined) => void;
  // The queue only chooses where feedback belongs in a pick row. The feature module owns
  // effective state, scope explanation, replacement, and Undo so those semantics are shared
  // with Channel programming rather than reimplemented here.
  renderFeedback?: (item: ProposalItem) => ReactNode;
  onApprove?: () => void;
  // Deny carries the admin's optional reason — the same string this component already
  // renders back via `denyReason` once the proposal is denied. The two halves were
  // never connected: the display shipped, the capture didn't, so the field was always
  // empty in practice.
  onDeny?: (reason?: string) => void;
  className?: string;
}

export type { ApprovalQueueItemProps, ApprovalStatus };
