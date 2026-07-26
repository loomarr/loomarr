import type { TitleDTO, TitleDTOState } from "@loomarr/api";
import type { JourneyStage } from "./journey.type";

// §13's member framing: a member should read their submission as a JOURNEY —
// "pending → acquiring (3/7) → live" — not as a table of provisioning states they have
// to interpret. The five API states (§4) collapse into the three things a person
// actually wants to know: is it waiting, is it coming, is it here.
//
// `unavailable` deliberately does NOT become a fourth stage. It is a title that gave up
// (§4 give-up after TTL) and it belongs to the "waiting" conversation — something the
// operator may need to retry — rather than reading as a failure of the whole channel.
const STAGE_BY_STATE: Record<TitleDTOState, JourneyStage> = {
  wanted: "waiting",
  requested: "acquiring",
  downloading: "acquiring",
  available: "ready",
  unavailable: "waiting",
};

const stageOf = (title: TitleDTO): JourneyStage => STAGE_BY_STATE[title.state] ?? "waiting";

// Progress is counted over titles that are ON the journey — anything that reached
// `available`, out of everything asked for. It answers "how far along am I", which is
// why an unavailable title still counts in the denominator: it was asked for, and
// quietly dropping it would make the fraction lie about what was requested.
const journeyProgress = (titles: TitleDTO[]): { ready: number; total: number } => ({
  ready: titles.filter((t) => t.state === "available").length,
  total: titles.length,
});

export { journeyProgress, stageOf };
