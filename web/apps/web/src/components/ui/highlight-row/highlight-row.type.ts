import type { useRender } from "@base-ui/react/use-render";
import type { ReactNode } from "react";

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
  // The row's one destination. Pass the router's `<Link to=… />`; the default is a `<button>`.
  render?: useRender.RenderProp;
  onClick?: () => void;
  className?: string;
}

export type { HighlightRowProps };
