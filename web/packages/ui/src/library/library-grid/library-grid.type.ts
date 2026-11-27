import type { ReactElement } from "react";

// The Library's poster grid as a scrolling list of rows, one implementation per platform:
// react-virtual on web (library-grid.web.tsx), every row on phone and TV (library-grid.tsx).
interface LibraryGridProps<Row> {
  /** The rows, each a full row of titles; the shelf splits the titles by its column count. */
  rows: Row[];
  /** A row's stable key: its first title's id. */
  rowKey: (row: Row, index: number) => string;
  /** One row as drawn, the space below it included: each row's height is measured from it. */
  renderRow: (row: Row) => ReactElement;
  /** A first guess at a row's height in pixels, until the row is measured. */
  estimateRowHeight: number;
  /** Called while the last rows are on screen, so the next page loads before the end shows. */
  onEndReached?: () => void;
  /** The list's accessible name. */
  label: string;
  /**
   * Scrolls the row to the top of the view, once per `request`. Web only: the other platforms
   * draw no control that asks.
   */
  scrollTo?: { row: number; request: number };
  /** Told the row at the top of the view as it changes. Web only. */
  onTopRowChange?: (row: number) => void;
}

export type { LibraryGridProps };
