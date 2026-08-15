type TunePhase = "osd" | "manifest" | "first-frame";

type TuneAttempt = {
  id: number;
  adjacent: boolean;
  warmed: boolean;
  /** Exact signed source reused from adjacent warming; never copied into telemetry. */
  playURL?: string;
};

export type { TuneAttempt, TunePhase };
