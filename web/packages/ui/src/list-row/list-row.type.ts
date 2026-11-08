import type { StatusTone } from "@loomarr/design-system";
import type { ReactElement, ReactNode } from "react";

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
  // One trailing control. Kept OUTSIDE any clickable region: the row itself is never a button, so
  // the action never nests in one.
  action?: ReactNode;
}

interface ListGroupProps {
  children: ReactNode;
  // Names the list for assistive tech when no heading labels it.
  "aria-label"?: string;
  "aria-labelledby"?: string;
}

interface HighlightRowProps {
  // When it airs, already formatted for the viewer's locale ("9:04 PM").
  time: string;
  // The channel number, zero-padded by the caller as the guide shows it ("07").
  channelNumber: string;
  // The show, then its quieter episode or context (" · Episode name").
  title: ReactNode;
  detail?: ReactNode;
  // Why it is a highlight, worded by the client from a typed server reason ("Season 4 premiere").
  reason?: ReactNode;
  // Web only: the row's one destination, as the router's `<Link to=… />`; the default is a button.
  render?: ReactElement<Record<string, unknown>>;
  onClick?: () => void;
}

export type { HighlightRowProps, ListGroupProps, ListRowProgress, ListRowProps };
