import type { ReactNode } from "react";

interface DockedPanelProps {
  /**
   * The panel's body, in a column that fills what the header leaves. It scrolls nothing itself: the
   * caller puts a scroll area (and a pinned footer) inside it.
   */
  children: ReactNode;
  /** The close button's name, which says what closes ("Close channels"). */
  closeLabel: string;
  /** Native only: the panel's density. A TV's close button is the larger focus target. */
  density?: "touch" | "tv";
  /** The edge the panel is docked to. Defaults to `right`. */
  edge?: "left" | "right";
  /** Muted text beside the title, for a short qualifier ("now to next 2h"). */
  meta?: ReactNode;
  /**
   * Called by the close button and by Escape. The caller owns whether the panel is shown: it
   * unmounts the panel to close it, so the panel keeps no open state.
   */
  onClose: () => void;
  /** The panel's name, shown in its header and read as its dialog name. */
  title: string;
  /** Width in px, never wider than the frame. Defaults to 372, the Watch page's drawer. */
  width?: number;
}

export type { DockedPanelProps };
