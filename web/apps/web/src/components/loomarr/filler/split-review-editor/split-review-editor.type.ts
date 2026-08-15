import type { SplitProposal } from "@loomarr/api/models/splitProposal";
import type { SplitSegment } from "@loomarr/api/models/splitSegment";

// The proposal is the orval-generated SplitProposal (§10 V34) — no hand-written mirror.
// The editor owns the draft state; the page owns the query, the confirm mutation and the
// navigation, so the editor stays storyable and testable without a router or a backend.
interface SplitReviewEditorProps {
  proposal: SplitProposal;
  // Resolved `filler.min_duration`, in milliseconds. Detection discards below-floor fragments,
  // but an operator can still create one by editing cut points or by opening a legacy proposal.
  // Absent means the setting could not be read, so the editor makes no invented claim.
  minClipDurationMs?: number;
  // The confirm mutation is in flight — disables the footer so a double-click can't
  // commit the same cut list twice.
  confirming?: boolean;
  // The operator's EDITED cut list, indices renumbered to the draft order. This is the
  // exact body the page POSTs to /confirm.
  onConfirm: (segments: SplitSegment[]) => void;
  // Leave WITHOUT confirming — the proposal stays persisted server-side (there is
  // deliberately no reject endpoint; review can happen later).
  onBack: () => void;
  className?: string;
}

export type { SplitReviewEditorProps };
