import { describe, expect, it } from "vitest";
import { provisionKey } from "./provision";

// Parity with Go's `provision.Title.Key()` (internal/provision/title.go:52). The cases below are
// the branches that function has, in its order — if it changes, this fails rather than the
// mismatch surfacing as an approval edit that silently fails to drop a title.
describe("provisionKey", () => {
  it("prefers TVDB for a series", () => {
    expect(provisionKey({ mediaType: "series", tvdbId: 71663, tmdbId: 456 })).toBe("series:tvdb:71663");
  });

  it("falls back to TMDB for a series with no TVDB id", () => {
    expect(provisionKey({ mediaType: "series", tmdbId: 4229 })).toBe("series:tmdb:4229");
  });

  it("uses TMDB for a movie", () => {
    expect(provisionKey({ mediaType: "movie", tmdbId: 603 })).toBe("movie:tmdb:603");
  });

  // A movie's tvdbId is NOT a fallback — Go's series+TVDB branch is guarded on mediaType, so a
  // movie with only a TVDB id has no usable key.
  it("ignores a TVDB id on a movie", () => {
    expect(provisionKey({ mediaType: "movie", tvdbId: 71663 })).toBe("");
  });

  // Go returns an error here; "" is the TS equivalent and means "not addable". Callers must
  // filter rather than inventing a `name:`-style key — ParseKey rejects any other shape.
  it("returns empty when there is no usable id", () => {
    expect(provisionKey({ mediaType: "movie" })).toBe("");
    expect(provisionKey({ mediaType: "series" })).toBe("");
    expect(provisionKey({})).toBe("");
  });

  // Zero is not an id. Go guards on `> 0`, so a 0 must not produce "movie:tmdb:0".
  it("treats a zero id as absent", () => {
    expect(provisionKey({ mediaType: "movie", tmdbId: 0 })).toBe("");
    expect(provisionKey({ mediaType: "series", tvdbId: 0, tmdbId: 0 })).toBe("");
  });
});
