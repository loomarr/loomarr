import type { ReactNode } from "react";
import type { StatusTone } from "../status-dot";

interface ListRowProgress {
  // 0–100. Omitted ⇒ indeterminate (queued, waiting for a slot).
  value?: number;
  // The bar's accessible name ("Downloading"). Required: a bare bar says nothing.
  label: string;
  // The quiet mono readout under the bar ("about 40 min", "queued").
  eta?: string;
}

interface ListRowProps {
  tone: StatusTone;
  // What the dot means, when the title does not already say it. "" (the default) hides it.
  toneLabel?: string;
  title: ReactNode;
  sub?: ReactNode;
  progress?: ListRowProgress;
  // One trailing control, usually `<Button size="sm" variant="outline">`. Kept OUTSIDE any
  // clickable region: the row itself is never a button, so the action never nests in one.
  action?: ReactNode;
  className?: string;
}

interface ListGroupProps {
  children: ReactNode;
  // Names the list for assistive tech when no heading labels it.
  "aria-label"?: string;
  "aria-labelledby"?: string;
  className?: string;
}

export type { ListGroupProps, ListRowProgress, ListRowProps };
