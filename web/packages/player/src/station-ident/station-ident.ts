import { useEffect, useRef, useState } from "react";

import type { PlayerSnapshot } from "../player-controller";

/** The station-ID card shows for five seconds on every settled tune (§9.1). */
const IDENT_MS = 5_000;

/**
 * Whether the station-ID card shows: once as each tune settles, dropped while the next one tunes.
 * Keyed by the tune, not by playing: resuming from pause is not a new arrival. The card keeps its
 * own five seconds, whatever the viewer does meanwhile.
 */
const useStationIdent = ({ attemptId, status }: Pick<PlayerSnapshot, "attemptId" | "status">): boolean => {
  const [identFor, setIdentFor] = useState<number>();
  const settledAttempt = status === "playing" ? attemptId : undefined;
  const shownAttempt = useRef<number | undefined>(undefined);
  useEffect(() => {
    if (settledAttempt === undefined || settledAttempt === shownAttempt.current) return;
    shownAttempt.current = settledAttempt;
    setIdentFor(settledAttempt);
  }, [settledAttempt]);
  useEffect(() => {
    if (identFor === undefined) return;
    const timer = setTimeout(() => setIdentFor(undefined), IDENT_MS);
    return () => clearTimeout(timer);
  }, [identFor]);
  return identFor !== undefined && identFor === attemptId && status !== "tuning";
};

export { useStationIdent };
