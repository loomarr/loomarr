import type * as React from "react";

interface ApprovalGroupRow {
  id: string;
  // Names the row's checkbox for a screen reader: "Select {label} for bulk approve".
  label: string;
  // A row bulk cannot take (it carries a pending edit) gets no checkbox and says why.
  excluded?: boolean;
  content: React.ReactNode;
}

interface ApprovalGroupProps {
  // "Channel requests" / "Filler downloads" — also the group's accessible name.
  title: string;
  // Who asks for everything in this group, when it is one party ("Requested by Loomarr").
  requestedBy?: string;
  rows: readonly ApprovalGroupRow[];
  selected: ReadonlySet<string>;
  onSelectedChange: (next: Set<string>) => void;
  // Opens the consequence summary for the selected rows; it never approves directly.
  onApprove: () => void;
  // A decision is in flight somewhere in the queue.
  disabled?: boolean;
  className?: string;
}

export type { ApprovalGroupProps, ApprovalGroupRow };
