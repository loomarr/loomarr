// Runs work after the page has painted and the browser has nothing better to do (#3399, 9.6): the
// main nav's route code is warmed this way, so a first click on a nav item finds its chunk already
// downloaded, without the warming competing with the first paint for the network or the main thread.

// Without requestIdleCallback (Safari) the wait is a short timer, which still lands after paint.
const FALLBACK_DELAY_MS = 200;
// An idle callback the browser never finds a gap for still runs by then.
const IDLE_TIMEOUT_MS = 2_000;

const whenIdle = (run: () => void): (() => void) => {
  if (typeof requestIdleCallback === "function") {
    const id = requestIdleCallback(run, { timeout: IDLE_TIMEOUT_MS });
    return () => cancelIdleCallback(id);
  }
  const id = setTimeout(run, FALLBACK_DELAY_MS);
  return () => clearTimeout(id);
};

/**
 * Starts each task in its own idle slot, the next only once the last has settled, so a nav's worth
 * of route preloads is a trickle and never a burst. A task that fails is dropped: warming is an
 * optimisation, and what it would have loaded loads on the click instead. Returns a function that
 * stops anything not yet started.
 */
const runAtIdle = (tasks: readonly (() => Promise<unknown>)[]): (() => void) => {
  let stopped = false;
  let cancelPending = () => {};
  const runFrom = (index: number) => {
    const task = tasks[index];
    if (stopped || !task) return;
    cancelPending = whenIdle(() => {
      void task()
        .catch(() => undefined)
        .finally(() => runFrom(index + 1));
    });
  };
  runFrom(0);
  return () => {
    stopped = true;
    cancelPending();
  };
};

export { runAtIdle };
