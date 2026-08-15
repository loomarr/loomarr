import type { FillerWatchOutputBodyHealth } from "@loomarr/api/models/fillerWatchOutputBodyHealth";

/**
 * How the filler pipeline is doing right now (§10 V38c).
 *
 * ⚠ **Aliased from the GENERATED client, never hand-declared.** The verdict is the server's
 * (`GET /v1/filler/watch`) — an independent union here could drift from the Go enum silently, and
 * the compiler would keep agreeing with whichever copy was wrong. A briefly hand-written version
 * of this type is why the note exists.
 *
 * Three states, not two, because "nothing is arriving" has two causes with OPPOSITE remedies: a
 * fresh install needs setting up, while a configured one that has gone quiet needs attention.
 */
type WatchHealth = FillerWatchOutputBodyHealth;

interface WatchPillProps {
  /** The mock's `watchLine` — "4 of 5 sources on · 9 clips · last scan 2m ago". */
  status: string;
  health: WatchHealth;
  className?: string;
}

export type { WatchHealth, WatchPillProps };
