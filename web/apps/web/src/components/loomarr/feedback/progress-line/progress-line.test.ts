import type { ProposalJourneyProgressDTO } from "@loomarr/api/models/proposalJourneyProgressDTO";
import { describe, expect, it } from "vitest";
import { pickCount, progressLine } from "./progress-line";

const progress = (over: Partial<ProposalJourneyProgressDTO>): ProposalJourneyProgressDTO => ({
  stage: "choosing",
  terms: [],
  picks: [],
  target: 8,
  startedAt: "2026-09-25T12:00:00Z",
  ...over,
});

describe("progressLine", () => {
  it.each([
    ["reading", [], "Reading your request…"],
    ["searching", ["speed", "action"], "Searching your library for speed, action…"],
    ["searching", [], "Searching your library…"],
    ["choosing", ["speed"], "Choosing titles…"],
    ["building", ["speed"], "Building your lineup…"],
  ] as const)("words stage %s (terms %j) as %s", (stage, terms, line) => {
    expect(progressLine(progress({ stage, terms: [...terms] }))).toBe(line);
  });
});

describe("pickCount", () => {
  it("counts against the most titles the model is asked for, not a guess", () => {
    const pick = { key: "movie:tmdb:1", mediaType: "movie", name: "A", inLibrary: true } as const;
    expect(pickCount(progress({ picks: [pick, pick, pick, pick], target: 8 }))).toBe("4 of about 8 picked");
  });

  it("says nothing before the first pick", () => {
    expect(pickCount(progress({}))).toBeUndefined();
  });
});
