// What a member is actually asking: is it waiting, is it coming, is it here — and, when the
// answer is "none of those", that Loomarr gave up and someone has to decide what to do.
type JourneyStage = "waiting" | "acquiring" | "ready" | "attention";

export type { JourneyStage };
