import { useEffect, useState } from "react";

// The station-ID card's time on screen after a settled tune (playback-clients: Station ID).
const IDENT_MS = 5_000;

// The station-ID card (§9.1): every settled tune shows it for five seconds, while the controls are
// hidden. A retune drops it until the new channel settles, then shows it afresh.
// A retune hides it by derivation: a state change here would render this tree a second time
// inside the press, holding up the tuning overlay.
const useStationIdent = (settled: boolean) => {
  const [identUp, setIdentUp] = useState(false);
  useEffect(() => {
    if (!settled) return;
    setIdentUp(true);
    const timer = window.setTimeout(() => setIdentUp(false), IDENT_MS);
    return () => window.clearTimeout(timer);
  }, [settled]);
  return settled && identUp;
};

export { useStationIdent };
