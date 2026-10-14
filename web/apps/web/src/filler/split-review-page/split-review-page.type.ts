// Where review was opened: Incoming's "Needs your help" card, or a Library card's "Split into
// clips". Back returns there (#1817, decided on #1908).
type SplitReviewOrigin = "incoming" | "library";

interface SplitReviewPageProps {
  // The persisted proposal id — from the route param. The proposal outlives the
  // detection job (§10 V34: review can happen long after detection), so the page
  // always reads it back rather than trusting anything the SSE frame carried.
  proposalId: string;
  // From the route's validated `from=` search param, which defaults to Library.
  from: SplitReviewOrigin;
}

export type { SplitReviewOrigin, SplitReviewPageProps };
