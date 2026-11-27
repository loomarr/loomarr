import type { ReactNode } from "react";

type VisuallyHiddenProps = {
  /** The words a screen reader reads and the eye never sees. */
  children: ReactNode;
  /**
   * The web element to draw, a `div` unless told otherwise so a heading or paragraph inside it stays
   * valid markup. A `<caption>` has to be a table's own child and a heading has to be a
   * heading, so the host is the caller's to name. Native has no elements and ignores it.
   */
  as?: "caption" | "div" | "h1" | "h2" | "h3" | "label" | "span";
};

export type { VisuallyHiddenProps };
