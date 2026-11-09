import { describe, expect, it } from "vitest";
import type { SuggestionRun } from "../use-suggestion-run/use-suggestion-run.type";
import {
  legacyGuideOrigination,
  originationHeading,
  originationStage,
  parseOriginationSearch,
} from "./origination-flow";

const run = (over: Partial<SuggestionRun>): SuggestionRun => ({
  actions: [],
  isRunning: false,
  failed: false,
  start: () => {},
  revise: () => {},
  retry: () => {},
  reset: () => {},
  resume: () => {},
  ...over,
});

describe("parseOriginationSearch", () => {
  // `?manual=1` is the number 1 to the router, so the flag is numeric; an old link or a link built
  // in code may still pass the string.
  it("reads the manual flag as a number, from either spelling", () => {
    expect(parseOriginationSearch({ manual: 1 })).toEqual({ manual: 1 });
    expect(parseOriginationSearch({ manual: "1" })).toEqual({ manual: 1 });
  });

  it("drops what is not a flow param", () => {
    expect(parseOriginationSearch({ manual: "yes", job: 42, intent: "cartoons", other: "x" })).toEqual({
      intent: "cartoons",
    });
  });
});

// #2746: the flow had a home on the Guide route, as `/guide?new=%221%22`. Those URLs still arrive.
describe("legacyGuideOrigination", () => {
  it("sends an old describe-form URL to the request route with no params", () => {
    expect(legacyGuideOrigination({ new: 1 })).toEqual({});
    expect(legacyGuideOrigination({ new: "1" })).toEqual({});
  });

  it("carries the flow's own params over", () => {
    expect(legacyGuideOrigination({ job: "job-1" })).toEqual({ job: "job-1" });
    expect(legacyGuideOrigination({ manual: 1, ideaId: "genre:comedy" })).toEqual({
      ideaId: "genre:comedy",
      manual: 1,
    });
    expect(legacyGuideOrigination({ intent: "cartoons", new: 1 })).toEqual({ intent: "cartoons" });
  });

  it("leaves an ordinary Guide visit alone", () => {
    expect(legacyGuideOrigination({})).toBeUndefined();
    expect(legacyGuideOrigination({ other: "x" })).toBeUndefined();
  });
});

describe("originationStage", () => {
  const proposal = { id: "p-1", status: "submitted" } as SuggestionRun["proposal"];

  it("names each step of the run", () => {
    expect(originationStage(run({}))).toBe("describe");
    expect(originationStage(run({ isRunning: true }))).toBe("generating");
    expect(originationStage(run({ failed: true }))).toBe("failed");
    expect(originationStage(run({ proposal }))).toBe("review");
    expect(originationStage(run({ proposal, isRunning: true }))).toBe("updating");
  });
});

describe("originationHeading", () => {
  it("names the describe step for who is asking", () => {
    expect(originationHeading("describe", true).title).toBe("Add a channel");
    expect(originationHeading("describe", false).title).toBe("Request a channel");
  });

  it("names the review", () => {
    expect(originationHeading("review", false)).toEqual({
      title: "Review your channel",
      description: "Choose what belongs. Nothing is created until you continue.",
    });
  });
});
