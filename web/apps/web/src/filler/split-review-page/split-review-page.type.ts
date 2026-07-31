interface SplitReviewPageProps {
  // The persisted proposal id — from the route param. The proposal outlives the
  // detection job (§10 V34: review can happen long after detection), so the page
  // always reads it back rather than trusting anything the SSE frame carried.
  proposalId: string;
}

export type { SplitReviewPageProps };
