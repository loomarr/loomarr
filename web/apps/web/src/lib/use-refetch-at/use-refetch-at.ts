import { useEffect, useRef } from "react";

// How long after a server deadline the read is repeated, so the server agrees it has passed.
const DEADLINE_MARGIN_MS = 1_000;
// A read that comes back with its deadline still unpassed by the server, though this clock says it
// has (this clock runs ahead), is tried again this long after, doubling, never more than a minute.
const SKEW_RETRY_MS = 5_000;
const SKEW_RETRY_MAX_MS = 60_000;

interface RefetchAt {
  /** The server's deadline in Unix ms, when the read's answer changes on time alone; none to wait for. */
  deadline: number | undefined;
  /** When the read last answered (TanStack's dataUpdatedAt), so each answer is waited on afresh. */
  answeredAt: number;
  refetch: () => unknown;
  enabled?: boolean;
}

// useRefetchAt repeats a frame-fed read at the moment its answer changes with nothing stored and
// no frame sent: a capture running out, a result going stale, a link expiring (#3352). The
// deadline is the server's clock, this one may disagree, so an answer that still names a deadline
// this clock has passed is read again after a growing wait rather than never.
const useRefetchAt = ({ deadline, answeredAt, refetch, enabled = true }: RefetchAt) => {
  const passedAnswers = useRef(0);
  const refetchRef = useRef(refetch);
  refetchRef.current = refetch;
  useEffect(() => {
    if (!enabled || !deadline || !answeredAt) return;
    const left = deadline - Date.now();
    let wait: number;
    if (left > 0) {
      passedAnswers.current = 0;
      wait = left + DEADLINE_MARGIN_MS;
    } else {
      wait = Math.min(SKEW_RETRY_MS * 2 ** passedAnswers.current, SKEW_RETRY_MAX_MS);
      passedAnswers.current += 1;
    }
    const timer = setTimeout(() => void refetchRef.current(), wait);
    return () => clearTimeout(timer);
  }, [enabled, deadline, answeredAt]);
};

export type { RefetchAt };
export { useRefetchAt };
