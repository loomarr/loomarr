import type { PullDTO } from "@loomarr/api";

interface PullCardProps {
  // The server's answer, verbatim (contract 1:1).
  pull: PullDTO;
  // Approve carries the operator's edits — the rows they dropped, and a note narrowing what to
  // fetch. ⚠ This is the COMMIT point: it is the only path on which a pull downloads anything.
  onApprove: (edits: { dropSourceIds: string[]; note: string }) => void;
  onDismiss: () => void;
  // A decision is in flight, so both buttons say so rather than looking inert.
  deciding?: boolean;
  className?: string;
}

export type { PullCardProps };
