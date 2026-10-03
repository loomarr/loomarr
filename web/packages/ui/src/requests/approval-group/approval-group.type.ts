/** One thing waiting on an admin, in the words both kinds of decision share. */
interface ApprovalRow {
  id: string;
  /** "14 titles · 3 to download": what approving would do. */
  meta: string;
  note?: string | undefined;
  requestedBy?: string | undefined;
  title: string;
}

type ApprovalGroupProps = {
  /** Ids with a decision in flight; their rows and the group's Select are disabled. */
  deciding: readonly string[];
  /** The negative action's label: a download is dismissed, a channel request is denied. */
  denyLabel?: "Deny" | "Dismiss";
  onApprove: (id: string) => void;
  onDeny: (id: string) => void;
  /** Present only where the decision can be edited first. */
  onEdit?: (id: string) => void;
  onToggleSelected: (id: string, selected: boolean) => void;
  onToggleSelecting: () => void;
  rows: readonly ApprovalRow[];
  selected: readonly string[];
  selecting: boolean;
  title: string;
};

export type { ApprovalGroupProps, ApprovalRow };
