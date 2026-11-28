import { describe, expect, it } from "vitest";
import { airingEpisode, airingsChangeAt, airingWhen, dateWords } from "./airing-words";

// The household's zone is five hours behind UTC in March after the clocks go forward.
const ZONE = "America/Chicago";
const NOW = Date.parse("2026-03-10T17:00:00Z"); // noon there

describe("airingWhen", () => {
  it.each([
    ["2026-03-10T16:30:00Z", "Watching now"],
    ["2026-03-10T19:00:00Z", "Next today at 2:00 PM"],
    ["2026-03-11T02:00:00Z", "Next tonight at 9:00 PM"],
    ["2026-03-11T15:00:00Z", "Next tomorrow at 10:00 AM"],
    ["2026-03-14T16:00:00Z", "Next Saturday at 11:00 AM"],
    ["2026-04-02T16:00:00Z", "Next April 2 at 11:00 AM"],
  ])("words %s on the household's clock", (start, words) => {
    const end = new Date(Date.parse(start) + 60 * 60 * 1000).toISOString();
    expect(airingWhen({ start, end }, ZONE, NOW)).toBe(words);
  });

  it("says nothing about an airing that has ended", () => {
    const ended = { start: "2026-03-10T15:00:00Z", end: "2026-03-10T17:00:00Z" };
    expect(airingWhen(ended, ZONE, NOW)).toBeUndefined();
  });
});

describe("airingsChangeAt", () => {
  const channel = { id: "90000971", name: "Quiet Hours", number: 21 };
  const airing = (start: string, end: string) => ({ channel, start, end, forecast: false });

  it("is the earliest coming start, or the end of what is on", () => {
    const on = airing("2026-03-10T16:00:00Z", "2026-03-10T18:00:00Z");
    const later = airing("2026-03-10T19:00:00Z", "2026-03-10T20:00:00Z");
    expect(airingsChangeAt([later, on], NOW)).toBe(Date.parse(on.end));
    expect(airingsChangeAt([later], NOW)).toBe(Date.parse(later.start));
    expect(airingsChangeAt([], NOW)).toBeUndefined();
  });
});

describe("dateWords", () => {
  it("writes a date in the locale the airings are told in", () => {
    expect(dateWords("1979-03-14")).toBe("March 14, 1979");
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
