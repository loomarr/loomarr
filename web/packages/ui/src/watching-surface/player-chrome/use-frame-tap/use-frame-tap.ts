import { useEffect, useRef } from "react";

// Two taps this close together on the same side of a touch frame are a double-tap.
const DOUBLE_TAP_MS = 300;

interface FrameTapOptions {
  controlsShown: boolean;
  hideControls: () => void;
  /** Whether the stream is live: only a live channel's double-tap steps. */
  live: boolean;
  revealControls: () => void;
  /** A double-tap on the left (−1) or right (+1) third. */
  onStep: (direction: -1 | 1) => void;
}

// Touch (#2088 decision 12): a tap on the picture shows or hides the controls rather than pausing,
// and on a live channel a double-tap on the left or right third steps 10 s. A tap waits out the
// double-tap window before acting, so the second tap of a double is never also a toggle. Returns the
// frame's tap handler, given where across the frame the tap landed (0 left edge, 1 right edge).
const useFrameTap = ({ controlsShown, hideControls, live, onStep, revealControls }: FrameTapOptions) => {
  const tapRef = useRef<{ at: number; side: number; timer?: ReturnType<typeof setTimeout> }>({
    at: 0,
    side: 0,
  });
  useEffect(() => () => clearTimeout(tapRef.current.timer), []);
  return (across: number) => {
    const side = across < 1 / 3 ? -1 : across > 2 / 3 ? 1 : 0;
    const now = Date.now();
    const tap = tapRef.current;
    clearTimeout(tap.timer);
    if (live && side !== 0 && side === tap.side && now - tap.at < DOUBLE_TAP_MS) {
      tapRef.current = { at: 0, side: 0 };
      onStep(side);
      return;
    }
    const shown = controlsShown;
    tapRef.current = {
      at: now,
      side,
      timer: setTimeout(() => (shown ? hideControls() : revealControls()), DOUBLE_TAP_MS),
    };
  };
};

export type { FrameTapOptions };
export { useFrameTap };
