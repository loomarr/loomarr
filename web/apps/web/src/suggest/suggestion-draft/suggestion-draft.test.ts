import { beforeEach, describe, expect, it } from "vitest";
import {
  ACTIVE_SUGGESTION_JOB_KEY,
  clearSuggestionDraft,
  readActiveSuggestionJob,
  readSuggestionDraft,
  SUGGESTION_DRAFT_KEY,
  writeActiveSuggestionJob,
  writeSuggestionDraft,
} from "./suggestion-draft";

describe("suggestion draft", () => {
  beforeEach(() => window.sessionStorage.clear());

  it("round-trips the complete intent and clears it", () => {
    const intent = { description: "Smart 1990s science fiction", era: "1990s" };
    writeSuggestionDraft(intent);
    expect(readSuggestionDraft()).toEqual(intent);

    clearSuggestionDraft();
    expect(readSuggestionDraft()).toBeUndefined();
  });

  it("discards corrupt browser state", () => {
    window.sessionStorage.setItem(SUGGESTION_DRAFT_KEY, "not-json");
    expect(readSuggestionDraft()).toBeUndefined();
    expect(window.sessionStorage.getItem(SUGGESTION_DRAFT_KEY)).toBeNull();
  });

  it("round-trips the active suggestion job and clears it", () => {
    writeActiveSuggestionJob("job-1");
    expect(readActiveSuggestionJob()).toBe("job-1");
    expect(window.sessionStorage.getItem(ACTIVE_SUGGESTION_JOB_KEY)).toBe("job-1");

    writeActiveSuggestionJob(undefined);
    expect(readActiveSuggestionJob()).toBeUndefined();
  });
});
