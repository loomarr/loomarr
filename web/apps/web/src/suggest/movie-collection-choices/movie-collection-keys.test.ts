import type { ProposalItem } from "@loomarr/api/models/proposalItem";
import { describe, expect, it } from "vitest";
import { MAX_MOVIE_COLLECTION_KEYS, movieCollectionKeys } from ".";

describe("movieCollectionKeys", () => {
  it("keeps distinct TMDB movie keys in visible order", () => {
    const items: ProposalItem[] = [
      { name: "First", mediaType: "movie", tmdbId: 671, inLibrary: true },
      { name: "Duplicate", mediaType: "movie", tmdbId: 671, inLibrary: false },
      { name: "Series", mediaType: "series", tmdbId: 1399, inLibrary: true },
      { name: "Unidentified", mediaType: "movie", inLibrary: false },
      { name: "Invalid", mediaType: "movie", tmdbId: -1, inLibrary: false },
      { name: "Fractional", mediaType: "movie", tmdbId: 67.2, inLibrary: false },
      { name: "Second", mediaType: "movie", tmdbId: 672, inLibrary: false },
    ];

    expect(movieCollectionKeys(items)).toEqual(["movie:tmdb:671", "movie:tmdb:672"]);
  });

  it("honors the public request bound", () => {
    const items: ProposalItem[] = Array.from({ length: MAX_MOVIE_COLLECTION_KEYS + 5 }, (_, index) => ({
      name: `Movie ${index + 1}`,
      mediaType: "movie",
      tmdbId: index + 1,
      inLibrary: false,
    }));

    const keys = movieCollectionKeys(items);
    expect(keys).toHaveLength(MAX_MOVIE_COLLECTION_KEYS);
    expect(keys.at(-1)).toBe("movie:tmdb:24");
  });
});
