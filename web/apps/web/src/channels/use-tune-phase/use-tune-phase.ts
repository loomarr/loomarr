import { useEffect, useState } from "react";
import type { TunePhase } from "./use-tune-phase.type";

// A wait of this long or less is the warm flip, and never shows the full signature (§9.4).
const FLIP_MS = 960;
// From this wait up the signature folds into the hold: the programme's fanart, dimmed.
const HOLD_MS = 1_500;

// The phase of the tune for a wait that began when `tuning` went true on `channelId`. The length comes
// from playback readiness, never a timer of its own: when the picture is ready `tuning` goes false and
// the phase is `idle`, however far it had got. A new channel while still waiting re-targets: the clock
// starts again, so surfing never climbs to the signature.
const useTunePhase = (tuning: boolean, channelId: string): TunePhase => {
  const [phase, setPhase] = useState<TunePhase>("idle");
  // Derived, not an effect: the first render of a wait is already the flip, so the chrome never
  // flashes the signature's layout for a frame.
  const shown: TunePhase = tuning ? (phase === "idle" ? "flip" : phase) : "idle";

  // biome-ignore lint/correctness/useExhaustiveDependencies: `channelId` restarts the clock on purpose
  useEffect(() => {
    if (!tuning) {
      setPhase("idle");
      return;
    }
    setPhase("flip");
    const signature = window.setTimeout(() => setPhase("signature"), FLIP_MS);
    const hold = window.setTimeout(() => setPhase("hold"), HOLD_MS);
    return () => {
      window.clearTimeout(signature);
      window.clearTimeout(hold);
    };
  }, [tuning, channelId]);

  return shown;
};

export { FLIP_MS, HOLD_MS, useTunePhase };
