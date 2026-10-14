import { describe, expect, it } from "vitest";
import {
  channelNumber,
  formatClipDuration,
  formatCompactCount,
  formatDuration,
  formatEpgTime,
  formatGiB,
  formatMmSs,
  formatPercent,
  formatPercentPoints,
  formatRelative,
  formatRuntime,
  formatUntil,
  formatUptime,
  humanizeRelaxation,
  humanizeSettingKey,
  parseMmSs,
  pluralize,
} from "./format";

describe("formatters", () => {
  it("formats durations", () => {
    expect(formatDuration(0)).toBe("0m");
    expect(formatDuration(42 * 60000)).toBe("42m");
    expect(formatDuration(102 * 60000)).toBe("1h 42m");
    expect(formatRuntime(133)).toBe("2h 13m");
  });

  it("formats sub-minute clip durations without collapsing to 1m", () => {
    expect(formatClipDuration(5000)).toBe("5s");
    expect(formatClipDuration(30000)).toBe("30s");
    expect(formatClipDuration(60000)).toBe("1m");
    expect(formatClipDuration(90000)).toBe("1m 30s");
  });

  it("formats clip offsets as mm:ss and parses them back", () => {
    expect(formatMmSs(0)).toBe("00:00");
    expect(formatMmSs(83000)).toBe("01:23");
    expect(formatMmSs(600000)).toBe("10:00");
    // Round-trips the two shapes an operator types; undefined rather than a guess on
    // anything else, because a wrong cut is what the review gate exists to catch.
    expect(parseMmSs("01:23")).toBe(83000);
    expect(parseMmSs("1:23")).toBe(83000);
    expect(parseMmSs("0:90")).toBe(90000);
    expect(parseMmSs("83")).toBeUndefined();
    expect(parseMmSs("1:2:3")).toBeUndefined();
    expect(parseMmSs("")).toBeUndefined();
  });

  it("channel numbers are bare", () => {
    expect(channelNumber(42)).toBe("42");
  });

  it("formats EPG time as 12-hour", () => {
    expect(formatEpgTime("2026-07-17T20:00:00")).toBe("8:00 PM");
    expect(formatEpgTime("2026-07-17T00:05:00")).toBe("12:05 AM");
  });

  it("formats relative time against an injected now", () => {
    const now = new Date("2026-07-17T12:00:00Z").getTime();
    expect(formatRelative("2026-07-17T11:59:30Z", now)).toBe("just now");
    expect(formatRelative("2026-07-17T11:57:00Z", now)).toBe("3m ago");
    expect(formatRelative("2026-07-17T10:00:00Z", now)).toBe("2h ago");
    expect(formatRelative("2026-07-12T12:00:00Z", now)).toBe("5d ago");
  });

  it("humanizes settings keys, upper-casing acronyms", () => {
    expect(humanizeSettingKey("library.url")).toBe("Library URL");
    expect(humanizeSettingKey("seerr.api_key")).toBe("Seerr API key");
    expect(humanizeSettingKey("llm.provider")).toBe("LLM provider");
    expect(humanizeSettingKey("filler.breaks_per_hour")).toBe("Filler breaks per hour");
    expect(humanizeSettingKey("tunarr.transcode_config_id")).toBe("Tunarr transcode config ID");
  });
});

// The API expresses time two ways — ISO strings on most payloads, Unix ms on sessions
// and now/next. formatRelative accepting only ISO is what makes a component author
// write their own ms-based copy, which is exactly what happened once already.
describe("Instant accepts both wire shapes", () => {
  const now = Date.parse("2026-07-17T12:00:00Z");

  it("formats a Unix-ms instant identically to its ISO equivalent", () => {
    const iso = "2026-07-17T11:57:00Z";
    expect(formatRelative(Date.parse(iso), now)).toBe(formatRelative(iso, now));
    expect(formatRelative(Date.parse(iso), now)).toBe("3m ago");
  });

  it("formats an EPG time from Unix ms", () => {
    // now/next carry stopMs, so the channel card's "·til 8:00 PM" needs ms support.
    expect(formatEpgTime(Date.parse("2026-07-17T20:00:00"))).toBe("8:00 PM");
  });
});

describe("formatUptime", () => {
  const MIN = 60_000;
  const HOUR = 60 * MIN;
  const DAY = 24 * HOUR;

  it("keeps days as days rather than a large hour count", () => {
    // ⚠ The reason this is not formatDuration: a week of uptime reads "168h 0m" there,
    // and days is the unit an operator thinks in.
    expect(formatUptime(6 * DAY + 4 * HOUR + 12 * MIN)).toBe("6d 4h 12m");
    expect(formatUptime(7 * DAY)).toBe("7d 0h 0m");
  });

  it("drops empty leading units", () => {
    expect(formatUptime(4 * HOUR + 12 * MIN)).toBe("4h 12m");
    expect(formatUptime(12 * MIN)).toBe("12m");
  });

  // A just-restarted server must not read "0m", which looks like a broken value; it is
  // also the state an operator is most likely looking at (they just restarted it).
  it("names the sub-minute case instead of showing 0m", () => {
    expect(formatUptime(0)).toBe("just started");
    expect(formatUptime(59_000)).toBe("just started");
    expect(formatUptime(-5)).toBe("just started");
  });

  // Floor, not round: 59 minutes of uptime is not an hour.
  it("floors rather than rounding up into a unit that has not elapsed", () => {
    expect(formatUptime(59 * MIN + 59_000)).toBe("59m");
    expect(formatUptime(23 * HOUR + 59 * MIN)).toBe("23h 59m");
  });
});

describe("formatUntil", () => {
  const now = Date.parse("2026-07-17T12:00:00Z");

  it("counts forward with the same coarseness as formatRelative", () => {
    expect(formatUntil(Date.parse("2026-07-17T12:20:00Z"), now)).toBe("in 20m");
    expect(formatUntil(Date.parse("2026-07-17T15:00:00Z"), now)).toBe("in 3h");
    expect(formatUntil(Date.parse("2026-07-22T12:00:00Z"), now)).toBe("in 5d");
  });

  it("says expired rather than counting backwards", () => {
    // A lapsed session must not read "in -3h"; the list only ever shows live ones, so
    // this is the guard against a clock-skew display bug rather than a normal state.
    expect(formatUntil(Date.parse("2026-07-17T09:00:00Z"), now)).toBe("expired");
    expect(formatUntil(now, now)).toBe("expired");
  });
});

describe("percent helpers keep their domains straight", () => {
  it("formatPercent takes a 0–1 ratio", () => {
    expect(formatPercent(0.87)).toBe("87%");
    expect(formatPercent(1)).toBe("100%");
    expect(formatPercent(0)).toBe("0%");
  });

  it("formatPercentPoints takes an already-0–100 value", () => {
    // The two exist separately because the suggester sends ratios and the LLM pull
    // sends points; one helper for both is how a 0.87 renders as "1%".
    expect(formatPercentPoints(71)).toBe("71%");
    expect(formatPercentPoints(100)).toBe("100%");
  });
});

describe("pluralize", () => {
  it("agrees with the noun at 1", () => {
    expect(pluralize(1, "session")).toBe("1 session");
    expect(pluralize(0, "session")).toBe("0 sessions");
    expect(pluralize(3, "session")).toBe("3 sessions");
  });

  it("takes an explicit plural for irregular nouns", () => {
    expect(pluralize(1, "entry", "entries")).toBe("1 entry");
    expect(pluralize(2, "entry", "entries")).toBe("2 entries");
  });
});

describe("formatGiB", () => {
  it("renders the unit the model picker shows twice", () => {
    expect(formatGiB(5)).toBe("5 GiB");
  });
  it("rounds a raw probe size to one decimal, not the full float", () => {
    expect(formatGiB(4.866521958261728)).toBe("4.9 GiB");
    expect(formatGiB(8)).toBe("8 GiB");
  });
});

describe("formatCompactCount", () => {
  it("compacts thousands and millions, dropping trailing .0", () => {
    expect(formatCompactCount(640)).toBe("640");
    expect(formatCompactCount(1200)).toBe("1.2K");
    expect(formatCompactCount(2000)).toBe("2K");
    expect(formatCompactCount(2_854_700)).toBe("2.9M");
    expect(formatCompactCount(1_000_000)).toBe("1M");
  });
  it("guards bad input to 0 rather than NaN", () => {
    expect(formatCompactCount(-5)).toBe("0");
    expect(formatCompactCount(Number.NaN)).toBe("0");
  });
});

describe("humanizeRelaxation", () => {
  it("labels the four ladder kinds and trims Go-duration values", () => {
    // The channel-detail chip was rendering "episodeNoRepeat: 30h0m0s → 24h0m0s"
    // verbatim — the raw ladder output. Each kind gets a readable label; durations
    // lose their zero units.
    expect(humanizeRelaxation({ kind: "episodeNoRepeat", from: "30h0m0s", to: "24h0m0s" })).toEqual({
      label: "Episode no-repeat",
      from: "30h",
      to: "24h",
    });
    expect(humanizeRelaxation({ kind: "seriesMinGap", from: "24h0m0s", to: "0s" })).toEqual({
      label: "Series min gap",
      from: "24h",
      to: "none", // "0s" reads as "none" — the gap was removed entirely
    });
    expect(humanizeRelaxation({ kind: "blockMax", from: "8", to: "unbounded" })).toEqual({
      label: "Block max",
      from: "8", // a count, not a duration — passes through untouched
      to: "unbounded",
    });
    expect(humanizeRelaxation({ kind: "era", from: "1990-1999", to: "1988-2001" })).toEqual({
      label: "Era",
      from: "1990-1999", // an era range — passes through
      to: "1988-2001",
    });
  });

  it("keeps meaningful sub-hour units and falls back on an unknown kind", () => {
    // A mixed duration keeps its non-zero units ("2h30m0s" → "2h30m").
    expect(humanizeRelaxation({ kind: "movieNoRepeat", from: "2h30m0s", to: "1h0m0s" })).toEqual({
      label: "Movie no-repeat",
      from: "2h30m",
      to: "1h",
    });
    // A kind with no label renders the slug rather than vanishing — so a future ladder
    // step still shows up, just unlabeled.
    expect(humanizeRelaxation({ kind: "somethingNew", from: "5m0s", to: "0s" })).toEqual({
      label: "somethingNew",
      from: "5m",
      to: "none",
    });
  });
});
