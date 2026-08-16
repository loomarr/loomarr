import type { TuneAttempt, TunePhase } from "./tuner-timing.type";

let nextAttemptId = 1;

const markName = (attempt: TuneAttempt, phase: "request" | TunePhase) =>
  `loomarr:tune:${attempt.id}:${phase}`;

const beginTune = (adjacent: boolean, warmed = false, playURL?: string): TuneAttempt => {
  const attempt = { id: nextAttemptId++, adjacent, warmed, playURL };
  if (typeof performance?.mark === "function") performance.mark(markName(attempt, "request"));
  return attempt;
};

const markTunePhase = (attempt: TuneAttempt | undefined, phase: TunePhase) => {
  if (!attempt || typeof performance?.mark !== "function" || typeof performance?.measure !== "function") {
    return;
  }
  const start = markName(attempt, "request");
  const end = markName(attempt, phase);
  performance.mark(end);
  const name = `loomarr:tune:request-to-${phase}`;
  try {
    performance.measure(name, {
      start,
      end,
      detail: { attemptId: attempt.id, adjacent: attempt.adjacent, warmed: attempt.warmed },
    });
  } catch {
    // Older WebKit implements the original three-argument User Timing form but not measure options.
    performance.measure(name, start, end);
  }
};

export { beginTune, markTunePhase };
