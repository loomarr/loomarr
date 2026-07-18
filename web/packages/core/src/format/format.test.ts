import { describe, expect, it } from "vitest";
import {
  channelNumber,
  formatClipDuration,
  formatDuration,
  formatEpgTime,
  formatRelative,
  formatRuntime,
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
});
