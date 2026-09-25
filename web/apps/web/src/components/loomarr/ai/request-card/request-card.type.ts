import type { ReactNode } from "react";

interface RequestCardProps {
  jobId: string;
  /** What was asked, in the requester's words. */
  title: string;
  /** The single status line — see `requestStatus`. */
  line: string;
  tone: "suggest" | "lock" | "onair" | "caution";
  createdAt: string;
  /** Extra guidance under the status line, e.g. what to do about a failure. */
  hint?: string;
  /** The one thing to do about this request. Sits OUTSIDE the detail link so it is its own control. */
  action?: ReactNode;
  className?: string;
}

export type { RequestCardProps };
