import type { ReactNode } from "react";

/** One row of a bulk group. The row's content is the caller's: the web queue passes its rich cards. */
interface ApprovalBulkRow {
  id: string;
  /** Names the row's checkbox for a screen reader: "Select {label} for bulk approve". */
  label: string;
  /** A row bulk cannot take (it carries a pending edit) gets no checkbox and says why. */
  excluded?: boolean | undefined;
  content: ReactNode;
}

interface ApprovalBulkGroupProps {
  /** "Channel requests" / "Filler downloads" — also the group's accessible name. */
  title: string;
  /** Who asks for everything in this group, when it is one party ("Requested by Loomarr"). */
  requestedBy?: string | undefined;
  rows: readonly ApprovalBulkRow[];
  selected: ReadonlySet<string>;
  onSelectedChange: (next: Set<string>) => void;
  /** Opens the consequence summary for the selected rows; it never approves directly. */
  onApprove: () => void;
  /** A decision is in flight somewhere in the queue. */
  disabled?: boolean | undefined;
}

export type { ApprovalBulkGroupProps, ApprovalBulkRow };
