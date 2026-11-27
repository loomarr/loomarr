import { describe, expect, it } from "vitest";
import { airingEpisode, airingWhen } from "./airing-words";

// The household's zone is five hours behind UTC in March after the clocks go forward.
const ZONE = "America/Chicago";
const NOW = Date.parse("2026-03-10T17:00:00Z"); // noon there

describe("airingWhen", () => {
  it.each([
    ["2026-03-10T16:30:00Z", "On now"],
    ["2026-03-10T19:00:00Z", "Next today at 2:00 PM"],
    ["2026-03-11T02:00:00Z", "Next tonight at 9:00 PM"],
    ["2026-03-11T15:00:00Z", "Next tomorrow at 10:00 AM"],
    ["2026-03-14T16:00:00Z", "Next Saturday at 11:00 AM"],
    ["2026-04-02T16:00:00Z", "Next April 2 at 11:00 AM"],
  ])("words %s on the household's clock", (start, words) => {
    expect(airingWhen(start, ZONE, NOW)).toBe(words);
  });
});

describe("airingEpisode", () => {
  it("names the season, the episode and its name", () => {
    expect(airingEpisode({ id: "1", season: 1, number: 3, name: "Dead Line" })).toBe(
      "Season 1, episode 3 · Dead Line",
    );
  });

  it("names both episodes of a double, and a special with no number", () => {
    expect(airingEpisode({ id: "1", season: 2, number: 3, end: 4, name: "" })).toBe(
      "Season 2, episodes 3 and 4",
    );
    expect(airingEpisode({ id: "1", season: 0, number: 0, name: "Holiday" })).toBe("Special · Holiday");
  });
});
