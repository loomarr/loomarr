import type { ReactElement } from "react";

type DemoBannerNext = {
  /** The step in plain words, e.g. "Watch a channel live". */
  label: string;
  /**
   * The element that goes there, such as a router link. Its children are replaced by the label, so
   * the next step is a real link the browser can open, not a button that navigates.
   */
  link: ReactElement<Record<string, unknown>>;
};

type DemoBannerProps = {
  /** The phone layout: one line of the Demo badge, the next step and the X. */
  compact?: boolean;
  /** The step on offer, counting from 1. Ignored once `next` is absent, and on a phone. */
  step: number;
  /** How many steps the tour has. */
  stepCount: number;
  /** The step to try next. Absent once every step is done, which shows the finished state. */
  next?: DemoBannerNext;
  /** Hide the tour for this viewer. */
  onDismiss: () => void;
};

export type { DemoBannerNext, DemoBannerProps };
