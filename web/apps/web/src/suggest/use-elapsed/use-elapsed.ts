import { useEffect, useState } from "react";

// One second, in ms — the tick interval and the display resolution.
const TICK_MS = 1000;

// Counts whole seconds since `running` last became true, and reports 0 whenever it is
// false. Used by the suggester's progress stepper: a grounded run makes several model
// calls, and on a cold local model the first can take ~9s on its own, so without a
// visible clock a working run is indistinguishable from a hung one.
//
// The interval only exists while running, so an idle screen holds no timer. Restarting
// (running false → true) resets to 0 rather than continuing, because the number is only
// meaningful relative to the run being watched.
const useElapsed = (running: boolean): number => {
  const [seconds, setSeconds] = useState(0);

  useEffect(() => {
    if (!running) {
      setSeconds(0);
      return;
    }
    // Anchor on the mount time and derive elapsed from the clock rather than
    // incrementing a counter: a background tab throttles timers, and a counter would
    // silently drift into under-reporting exactly when a run feels slowest.
    const started = Date.now();
    setSeconds(0);
    const id = setInterval(() => setSeconds(Math.floor((Date.now() - started) / TICK_MS)), TICK_MS);
    return () => clearInterval(id);
  }, [running]);

  return seconds;
};

export { useElapsed };
