import type { ReactElement, ReactNode } from "react";

interface TooltipProps {
  /**
   * The one element that opens the tooltip on hover and keyboard focus. It must carry its own
   * accessible name: the tooltip repeats what the control does, it never names it.
   */
  children: ReactElement;
  /** What the tooltip says. Plain text for a label; a few lines for a preview. */
  content: ReactNode;
  /** Set when the trigger points at the tooltip with `aria-describedby`. */
  id?: string;
  side?: "bottom" | "left" | "right" | "top";
  align?: "center" | "end" | "start";
  /** A fixed width for multi-line previews; a label sizes to its text. */
  width?: number;
}

export type { TooltipProps };
