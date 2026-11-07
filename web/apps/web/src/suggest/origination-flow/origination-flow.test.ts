import { beforeEach, describe, expect, it } from "vitest";
import { writeActiveSuggestionJob, writeSuggestionDraft } from "../suggestion-draft";
import type { SuggestionRun } from "../use-suggestion-run/use-suggestion-run.type";
import {
  isOriginating,
  originationHeading,
  originationStage,
  parseOriginationSearch,
  rememberedOrigination,
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
  // The router parses `?new=1` as the number 1; a link built in code passes the string.
  it("accepts the flags as the router parses them", () => {
    expect(parseOriginationSearch({ new: 1, manual: "1" })).toEqual({ new: "1", manual: "1" });
  });

  it("drops what is not a flow param", () => {
    expect(parseOriginationSearch({ new: "yes", job: 42, intent: "cartoons", other: "x" })).toEqual({
      intent: "cartoons",
    });
  });
});

describe("isOriginating", () => {
  it("is open while any flow param is present", () => {
    expect(isOriginating({})).toBe(false);
    expect(isOriginating({ ideaId: "genre:comedy" })).toBe(false);
    expect(isOriginating({ new: "1" })).toBe(true);
    expect(isOriginating({ job: "job-1" })).toBe(true);
    expect(isOriginating({ intent: "cartoons" })).toBe(true);
    expect(isOriginating({ manual: "1" })).toBe(true);
  });
});

describe("rememberedOrigination", () => {
  beforeEach(() => window.sessionStorage.clear());

  it("remembers nothing in a fresh tab", () => {
    expect(rememberedOrigination()).toBeUndefined();
  });

  it("returns to the Job the tab follows, ahead of any draft", () => {
    writeSuggestionDraft({ description: "cartoons" });
    writeActiveSuggestionJob("job-1");
    expect(rememberedOrigination()).toEqual({ job: "job-1" });
  });

  it("reopens the describe form for a draft that has no Job yet", () => {
    writeSuggestionDraft({ description: "cartoons" });
    expect(rememberedOrigination()).toEqual({ new: "1" });
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
