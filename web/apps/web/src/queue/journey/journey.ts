import type { TitleDTO } from "@loomarr/api/models/titleDTO";
import type { TitleDTOState } from "@loomarr/api/models/titleDTOState";
import type { JourneyStage } from "./journey.type";

// §13's member framing: a member should read their submission as a JOURNEY —
// "pending → acquiring (3/7) → live" — not as a table of provisioning states they have
// to interpret. The five API states (§4) collapse into the three things a person
// actually wants to know: is it waiting, is it coming, is it here.
//
// `unavailable` is its own "attention" stage. It is a title the reconciler gave up on (§4
// give-up after TTL): nothing will request it next, so filing it under "waiting" ("Approved and
// queued") described a dead title as alive (#1404). It still reads as one title's outcome, not
// a failure of the whole channel.
const STAGE_BY_STATE: Record<TitleDTOState, JourneyStage> = {
  wanted: "waiting",
  requested: "acquiring",
  downloading: "acquiring",
  available: "ready",
  unavailable: "attention",
};

const stageOf = (title: TitleDTO): JourneyStage => STAGE_BY_STATE[title.state] ?? "waiting";

// In flight = still moving towards `available`: wanted, requested or downloading. Neither a
// landed title nor a given-up one is in flight, so neither belongs in an "in flight" count.
const isInFlight = (title: TitleDTO): boolean => {
  const stage = stageOf(title);
  return stage === "waiting" || stage === "acquiring";
};

// Progress is counted over titles that are ON the journey — anything that reached
// `available`, out of everything asked for. It answers "how far along am I", which is
// why an unavailable title still counts in the denominator: it was asked for, and
// quietly dropping it would make the fraction lie about what was requested.
const journeyProgress = (titles: TitleDTO[]): { ready: number; total: number } => ({
  ready: titles.filter((t) => t.state === "available").length,
  total: titles.length,
});

export { isInFlight, journeyProgress, stageOf };
