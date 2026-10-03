/** The two kinds of decision an admin makes on Needs you. They never share a selection or a bulk approve. */
type ReviewGroup = "filler" | "requests";

/** One id's line in the result sheet: BulkApproveResult's `{ok, error}` with the title it was approved as. */
interface ReviewRow {
  error?: string;
  id: string;
  ok: boolean;
  title: string;
}

type ReviewSheet =
  | { group: ReviewGroup; id: string; kind: "deny" }
  | { group: ReviewGroup; kind: "bulk" }
  | { id: string; kind: "edit" }
  | { group: ReviewGroup; kind: "result"; rows: readonly ReviewRow[] };

interface ReviewState {
  /** Ids ticked in each group; a group's selection is its own. */
  selected: Readonly<Record<ReviewGroup, readonly string[]>>;
  /** Whether each group is in Select mode, which swaps a row's actions for a checkbox. */
  selecting: Readonly<Record<ReviewGroup, boolean>>;
  /** The one sheet docked above the tab bar, if any. */
  sheet?: ReviewSheet;
}

type ReviewAction =
  | { group: ReviewGroup; id: string; on: boolean; type: "toggle-selected" }
  | { group: ReviewGroup; id: string; type: "open-deny" }
  | { group: ReviewGroup; rows: readonly ReviewRow[]; type: "show-result" }
  | { group: ReviewGroup; type: "toggle-selecting" }
  | { id: string; type: "open-edit" }
  | { type: "close-sheet" };

export type { ReviewAction, ReviewGroup, ReviewRow, ReviewSheet, ReviewState };
