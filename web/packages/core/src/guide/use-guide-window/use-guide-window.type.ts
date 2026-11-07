import type { GuideWindow } from "../guide.type";

interface GuideWindowClock {
  /** The live clock, ticking on its own timer; never part of a query key. */
  nowMs: number;
  /** The clock the window was last pinned at: stable until the window slides. */
  anchorMs: number;
  /** TODAY's window at the requested span, derived from `anchorMs`. */
  window: GuideWindow;
}

export type { GuideWindowClock };
