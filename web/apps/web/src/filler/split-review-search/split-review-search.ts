// The split-review route's search params, apart from the page: the route validates them when the
// app starts, so nothing here may pull in UI.

// Where review was opened: Incoming's "Needs your help" card, or a Library card's "Split into
// clips". Back returns there (#1817, decided on #1908).
type SplitReviewOrigin = "incoming" | "library";

// The route's `from=` search param names where review was opened, so Back can return there. Only
// the two known origins are kept; anything else (a deep link, a hand-edited URL) is dropped and
// Back falls back to Library, where it always went before. Optional, so a link may omit it.
const validateSplitReviewSearch = (search: Record<string, unknown>): { from?: SplitReviewOrigin } =>
  search.from === "incoming" || search.from === "library" ? { from: search.from } : {};

export type { SplitReviewOrigin };
export { validateSplitReviewSearch };
