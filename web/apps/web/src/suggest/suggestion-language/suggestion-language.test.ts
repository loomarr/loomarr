import { describe, expect, it } from "vitest";
import {
  friendlyCandidateName,
  friendlyProposalRationale,
  friendlyTitleRationale,
} from "./suggestion-language";

describe("suggestion presentation language", () => {
  it("translates known title evidence into plain language", () => {
    expect(
      friendlyTitleRationale(
        "Included because the resolved public reference names this title as a constituent.",
      ),
    ).toBe("This title is part of the lineup or collection you asked for.");
  });

  it("does not leak unfamiliar evidence jargon from a new backend explanation", () => {
    expect(
      friendlyTitleRationale("Grounded against constituent evidence in the public-reference source truth."),
    ).toBe("This title matches the channel you described.");
  });

  it("keeps a natural title explanation intact", () => {
    expect(friendlyTitleRationale("Same era, same energy as Heat.")).toBe("Same era, same energy as Heat.");
  });

  it("translates proposal-level evidence language", () => {
    expect(
      friendlyProposalRationale(
        "Every offered title is backed by resolved public-reference constituent evidence.",
      ),
    ).toBe("These titles are part of the lineup or collection you asked for.");
    expect(friendlyProposalRationale("Grounded by the catalog evidence set.")).toBe(
      "These titles match the channel you described.",
    );
  });

  it("hides machine identifiers when no human title was resolved", () => {
    expect(friendlyCandidateName("series:tmdb:3921")).toBe("Unidentified title");
    expect(friendlyCandidateName("movie:tvdb:123")).toBe("Unidentified title");
    expect(friendlyCandidateName("Boy Meets World")).toBe("Boy Meets World");
    expect(friendlyCandidateName(undefined)).toBe("Unidentified title");
  });
});
