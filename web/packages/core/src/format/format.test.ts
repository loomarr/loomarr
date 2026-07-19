import { describe, expect, it } from "vitest";
import {
  channelNumber,
  formatClipDuration,
  formatDuration,
  formatEpgTime,
  formatGiB,
  formatPercent,
  formatPercentPoints,
  formatRelative,
  formatRuntime,
  formatUntil,
  humanizeSettingKey,
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
});
