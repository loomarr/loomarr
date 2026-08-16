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

const adjacentWarmMarkName = (channelId: string) => `loomarr:tuner:warm:${channelId}`;

const markAdjacentWarm = (channelId: string) => {
  if (typeof performance?.mark !== "function") return;
  const name = adjacentWarmMarkName(channelId);
  // A long-running television session should retain only the latest proof for each bounded catalog
  // entry. The mark is emitted only after the warmer consumed every response body, so it is a more
  // faithful readiness seam than observing whether the browser happened to issue a network request.
  performance.clearMarks?.(name);
  performance.mark(name);
};

export { adjacentWarmMarkName, beginTune, markAdjacentWarm, markTunePhase };
