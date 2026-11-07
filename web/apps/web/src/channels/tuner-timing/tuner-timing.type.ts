type TunePhase = "osd" | "still" | "manifest" | "first-frame";

type TuneAttempt = {
  id: number;
  adjacent: boolean;
  warmed: boolean;
};

export type { TuneAttempt, TunePhase };
