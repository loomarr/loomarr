import type { ProposalItem } from "@loomarr/api/models/proposalItem";
import { describe, expect, it } from "vitest";
import {
  isSuggestionExpansion,
  MAX_SUGGESTION_OPTIONS,
  suggestionExpansionRequest,
  suggestionIdentity,
} from "./suggestion-expansion";

const movie = (name: string, year: number, tmdbId?: number): ProposalItem => ({
  name,
  year,
  mediaType: "movie",
  inLibrary: false,
  ...(tmdbId ? { tmdbId } : {}),
});

describe("suggestion expansion", () => {
  it("builds a repeatable request that excludes each unique option already shown", () => {
    expect(
      suggestionExpansionRequest([
        movie("Lemon Tree Heist", 1997, 90000754),
        movie("Lemon Tree Heist", 1997, 90000754),
        movie("The Seventh Satellite Choir", 1991, 90001089),
      ]),
    ).toBe(
      'Find 6–8 additional titles that match this brief. Do not repeat or replace the selected lineup. Do not return these suggestions already shown: "Lemon Tree Heist" (1997); "The Seventh Satellite Choir" (1991).',
    );
  });

  it("recognizes only the dedicated expansion refinement", () => {
    const request = suggestionExpansionRequest([]);

    expect(isSuggestionExpansion(request)).toBe(true);
    expect(isSuggestionExpansion("Add more variety to the channel")).toBe(false);
    expect(isSuggestionExpansion()).toBe(false);
  });

  it("uses grounded identity when available and keeps the pool bounded", () => {
    expect(suggestionIdentity(movie("Lemon Tree Heist", 1997, 90000754))).toBe("movie:tmdb:90000754");
    expect(suggestionIdentity(movie("Unknown Cut", 1998))).toBe("movie:unknown cut:1998");
    expect(MAX_SUGGESTION_OPTIONS).toBe(24);
  });
});
