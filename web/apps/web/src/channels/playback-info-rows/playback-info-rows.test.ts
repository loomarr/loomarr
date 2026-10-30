import type { ChannelHealth } from "@loomarr/api";
import { describe, expect, it } from "vitest";
import { adminRows } from "./playback-info-rows";

describe("adminRows", () => {
  const health = (overrides: Partial<ChannelHealth>): ChannelHealth => ({
    bufferedMs: 4_000,
    channelId: "ch-1",
    coldStartMs: 900,
    encoder: "nvidia",
    hardware: true,
    health: "ok",
    mode: "transcode",
    name: "Coast 90s",
    number: 7,
    reason: "Transcoding at 1.40× realtime — comfortable margin.",
    speed: 1.4,
    target: "browser",
    viewers: 1,
    ...overrides,
  });

  it("reads the server's encode in its own words, then the session", () => {
    expect(adminRows(health({}), "ps_1")).toEqual([
      ["Encoder", "nvidia · hardware"],
      ["Transcode", "Transcoding at 1.40× realtime — comfortable margin."],
      ["Health", "ok"],
      ["Session", "ps_1"],
    ]);
    expect(
      adminRows(
        health({ encoder: "copy", mode: "direct-play", reason: "Direct play — no transcode, no GPU." }),
        "ps_1",
      )[0],
    ).toEqual(["Encoder", "copy · direct play"]);
  });

  it("has only the session while the server reports no encode for the channel", () => {
    expect(adminRows(undefined, "ps_1")).toEqual([["Session", "ps_1"]]);
  });
});
