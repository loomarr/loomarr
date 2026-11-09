import type { ReactNode } from "react";
import type { RequestEntry } from "@/queue/use-requests";

interface RequestRowProps {
  /** The one thing to do about this request: a `CardAction`, composed onto a link. */
  action?: ReactNode;
  /** The parts after the status on the one metadata line (when it was asked, the sentence behind the status). */
  meta?: ReactNode[];
  /** The status the request is in: its short line and the tone that draws its dot. */
  status: Pick<RequestEntry["status"], "line" | "tone">;
  /** What was asked, in the requester's words. A string, or a link to the request's detail. */
  title: ReactNode;
}

export type { RequestRowProps };
