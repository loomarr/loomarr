import type { ReactNode } from "react";

interface RowCardProps {
  /** The one action at the trailing edge. */
  action?: ReactNode;
  /** The round lead: an icon or an initial. */
  lead: ReactNode;
  meta: ReactNode[];
  /** A string, or a link to the item's detail when the row has a second place to go. */
  title: ReactNode;
}

export type { RowCardProps };
