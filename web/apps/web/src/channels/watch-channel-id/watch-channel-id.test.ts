import { describe, expect, it } from "vitest";
import { channel } from "@/test/fixtures/channels";
import { watchChannelId } from "./watch-channel-id";

describe("watchChannelId", () => {
  it("hides Watch's slot when no channel exists", () => {
    expect(watchChannelId([], undefined)).toBeUndefined();
  });

  it("opens the lowest-numbered playable channel before anyone has tuned", () => {
    const channels = [
      channel({ id: "ch-30", number: 30 }),
      channel({ id: "ch-10", number: 10 }),
      channel({ id: "ch-20", number: 20 }),
    ];
    expect(watchChannelId(channels, undefined)).toBe("ch-10");
  });

  it("opens the last-tuned channel once one exists, even when it isn't the lowest number", () => {
    const channels = [channel({ id: "ch-10", number: 10 }), channel({ id: "ch-30", number: 30 })];
    expect(watchChannelId(channels, { channelId: "ch-30", tunedAt: "2026-10-03T00:00:00Z" })).toBe("ch-30");
  });

  it("falls back to the lowest number when the last-tuned channel no longer exists", () => {
    const channels = [channel({ id: "ch-10", number: 10 })];
    expect(watchChannelId(channels, { channelId: "ch-99", tunedAt: "2026-10-03T00:00:00Z" })).toBe("ch-10");
  });

  it("falls back to the lowest number when the last-tuned channel is no longer playable", () => {
    const channels = [
      channel({ id: "ch-10", number: 10, inAppPlayable: true }),
      channel({ id: "ch-20", number: 20, inAppPlayable: false }),
    ];
    expect(watchChannelId(channels, { channelId: "ch-20", tunedAt: "2026-10-03T00:00:00Z" })).toBe("ch-10");
  });
});
