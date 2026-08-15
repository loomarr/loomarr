type TunePhase = "osd" | "manifest" | "first-frame";

type TuneAttempt = {
  id: number;
  adjacent: boolean;
};

export type { TuneAttempt, TunePhase };
