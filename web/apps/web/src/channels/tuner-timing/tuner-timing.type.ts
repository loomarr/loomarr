type TunePhase = "osd" | "still" | "manifest" | "first-frame";

type TuneAttempt = {
  id: number;
  adjacent: boolean;
  warmed: boolean;
  /** Exact signed source reused from adjacent warming; never copied into telemetry. */
  playURL?: string;
  /** The warmed neighbour's prefetched still; never copied into telemetry. */
  stillURL?: string;
};

export type { TuneAttempt, TunePhase };
