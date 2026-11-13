// How much of the tune a wait has earned (§9.4). The tune only dresses a wait that already exists, so
// it grows with the wait: a quiet flip first, the full signature only when the picture is slow, and
// the hold (the fanart behind the ident) when it is slower still.
type TunePhase = "flip" | "hold" | "idle" | "signature";

export type { TunePhase };
