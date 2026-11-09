import { beforeEach, describe, expect, it } from "vitest";
import { writeActiveSuggestionJob, writeSuggestionDraft } from "../suggestion-draft";
import { rememberedOrigination } from "./remembered-origination";

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

  // The request route is the describe form: it reads a draft that has no Job itself, so there is
  // nowhere to send the viewer.
  it("has nothing to return to for a draft that has no Job yet", () => {
    writeSuggestionDraft({ description: "cartoons" });
    expect(rememberedOrigination()).toBeUndefined();
  });
});
