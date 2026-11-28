import type { ReactNode } from "react";

interface PanelRowProps {
  // The identity region. It grows, shrinks first and lays its own content out.
  children: ReactNode;
  // The trailing cluster (numbers, badges, actions). Stays one unit and drops to its own line when
  // the row is too narrow for both, so it is never clipped.
  meta?: ReactNode;
}

export type { PanelRowProps };
