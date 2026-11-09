// A whole-year range, both ends set. The wire's Range leaves either end optional, so a caller hands
// those through normalizeYearRanges, which keeps only the complete, in-bounds ones.
interface YearRange {
  from: number;
  to: number;
}

interface YearRangeListFieldProps {
  // Names the list in every accessible name ("From year (Movie release, range 2)"), so several lists
  // on one screen do not read as identical rows of "From year" boxes and "Remove" buttons.
  label: string;
  // Sorted and merged, as normalizeYearRanges returns them. The field edits these rows in place.
  ranges: YearRange[];
  // Called with the next sorted, merged list after an add, an edit or a removal. A pair that is not
  // a valid range never reaches it: the field shows the error and waits for the pair to be fixed.
  onChange: (next: YearRange[]) => void;
  disabled?: boolean;
  // The list stops offering a new row at this many ranges.
  maxRanges?: number;
  // Shown instead of rows while the list is empty.
  emptyText?: string;
}

export type { YearRange, YearRangeListFieldProps };
