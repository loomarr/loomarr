import type { ReactElement, ReactNode } from "react";

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

export type { HighlightRowProps };
