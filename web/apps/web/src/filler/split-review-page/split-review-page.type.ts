import type { SplitReviewOrigin } from "../split-review-search";

interface SplitReviewPageProps {
  // The persisted proposal id — from the route param. The proposal outlives the
  // detection job (§10 V34: review can happen long after detection), so the page
  // always reads it back rather than trusting anything the SSE frame carried.
  proposalId: string;
  // From the route's validated `from=` search param, which defaults to Library.
  from: SplitReviewOrigin;
}

export type { SplitReviewPageProps };
