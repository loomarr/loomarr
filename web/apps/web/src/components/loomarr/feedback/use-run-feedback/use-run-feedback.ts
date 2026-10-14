import { useCallback, useState } from "react";

// MIN_VISIBLE_MS — how long a triggered action keeps SAYING it is running.
//
// ⚠ Not an artificial delay for its own sake. Several jobs finish in well under 100ms, and a
// spinner that appears and vanishes inside one frame is indistinguishable from a click that did
// not register — which is exactly what an operator reported on the Tasks page. Nothing is held
// back meanwhile: the work is already queued and running; this governs only how long the
// control admits to it.
const MIN_VISIBLE_MS = 600;

// useRunFeedback tracks which keyed actions are in flight, so a "run it now" control can show
// feedback that a server-side flag cannot provide.
//
// ⚠ **Why this exists at all.** A queued backend accepts the request in milliseconds (202) and
// reports the work as running for only as long as a worker holds it (~250ms, measured). Both
// windows usually close before the list refetch lands, so a UI driven purely by server state
// shows nothing at all. The page has to remember what IT triggered.
//
// Combine with any real server signal at the call site:
//   const busy = feedback.isBusy(job.name) || job.running;
//
// The second term still matters — it covers a run someone else started, and a genuinely long
// job — but the first is what makes the state visible in the common case.
const useRunFeedback = (minVisibleMs: number = MIN_VISIBLE_MS) => {
  const [inFlight, setInFlight] = useState<ReadonlySet<string>>(new Set());

  const start = useCallback((key: string) => {
    setInFlight((prev) => new Set(prev).add(key));
  }, []);

  const finish = useCallback((key: string) => {
    setInFlight((prev) => {
      const next = new Set(prev);
      next.delete(key);
      return next;
    });
  }, []);

  // settle: await alongside whatever refetch follows, so the control stays busy for at least
  // the minimum visible duration AND until the fresh data is in.
  const settle = useCallback(
    async (key: string, work?: Promise<unknown>) => {
      await Promise.all([work ?? Promise.resolve(), new Promise((r) => setTimeout(r, minVisibleMs))]);
      finish(key);
    },
    [finish, minVisibleMs],
  );

  const isBusy = useCallback((key: string) => inFlight.has(key), [inFlight]);

  return { isBusy, start, finish, settle };
};

export { MIN_VISIBLE_MS, useRunFeedback };
