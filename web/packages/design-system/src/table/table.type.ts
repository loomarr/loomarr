import type { ReactNode } from "react";

import type { Density } from "../tokens";

type TableProps = {
  /** The table's accessible name (a visually hidden caption on web, the label on native). */
  caption: string;
  children: ReactNode;
  density?: Density;
  /**
   * The bordered, rounded box the mock draws around a table. Turn it off when the table already
   * sits inside a bordered card.
   */
  framed?: boolean;
  /** A width below which the table scrolls sideways instead of squeezing its columns (web). */
  minWidth?: number;
};

type TableSectionProps = { children: ReactNode };

type TableRowProps = {
  children: ReactNode;
  /** A row that is out of play (a paused task): drawn at reduced strength but still readable. */
  dimmed?: boolean;
  /** The row's own id, for a button's `aria-controls` (an expanded detail row). */
  id?: string;
  /** A row that belongs to the one above it (an opened detail row): drawn on the raised surface. */
  tone?: "default" | "raised";
};

type TableCellProps = {
  /** Which edge the content lines up on. `end` suits a trailing action or a number. */
  align?: "start" | "end";
  children?: ReactNode;
  /** Join this many columns (web). Native rows have no columns to join: a lone cell fills the row. */
  colSpan?: number;
  /** The cell's text in the mono voice: a key, an id, a path. */
  mono?: boolean;
  /** Where the content sits when a neighbour is taller. Defaults to `middle`. */
  verticalAlign?: "top" | "middle";
  /**
   * A column's width in px. Put it on the header cell: web lays the column out from it. Native has
   * no table layout, so give it on every cell of the column, which then share the row by it.
   */
  width?: number;
};

type TableHeaderCellProps = Omit<TableCellProps, "mono" | "verticalAlign">;

export type { TableCellProps, TableHeaderCellProps, TableProps, TableRowProps, TableSectionProps };
