import type { ChannelHealth } from "@loomarr/api";
import { describe, expect, it } from "vitest";
import { adminRows, describeCodecs, streamRows } from "./playback-info-rows";

const rungLabel = (height: number) => (height >= 2160 ? "4K" : `${height}p`);

describe("describeCodecs", () => {
  it("names the codecs as a person does", () => {
    expect(describeCodecs("avc1.640028,mp4a.40.2")).toBe("H.264 High · AAC-LC");
    expect(describeCodecs("avc1.4d401f, mp4a.40.5")).toBe("H.264 Main · HE-AAC");
    expect(describeCodecs("hvc1.2.4.L150.B0,ec-3")).toBe("HEVC Main 10 · E-AC-3");
    // One it doesn't know reads as the master wrote it.
    expect(describeCodecs("av01.0.08M.08")).toBe("av01.0.08M.08");
  });
});

describe("streamRows", () => {
  it("reads what this browser is playing, rung first", () => {
    expect(
      streamRows({
        auto: true,
        facts: {
          bitrate: 3_100_000,
          bufferMs: 12_430,
          codecs: "avc1.640028,mp4a.40.2",
          frameRate: 29.97,
          height: 1080,
          rung: { number: 1, of: 2 },
          url: "/v1/stream",
          width: 1920,
        },
        lagSeconds: 134,
        rungLabel,
      }),
    ).toEqual([
      ["Quality", "Auto · 1080p (rung 1 of 2)"],
      ["Resolution", "1920×1080 @ 29.97"],
      ["Codec", "H.264 High · AAC-LC"],
      ["Bitrate", "3.1 Mbps"],
      ["Buffer", "12.4 s"],
      ["Behind live", "2m 14s"],
    ]);
  });

  it("knows only how far behind live before a source is bound", () => {
    expect(streamRows({ auto: true, lagSeconds: 5, rungLabel })).toEqual([["Behind live", "5 s"]]);
  });

  it("leaves out what native HLS doesn't tell, and what hasn't decoded yet", () => {
    expect(
      streamRows({
        auto: false,
        facts: { bufferMs: 0, height: 0, url: "/s", width: 0 },
        lagSeconds: 0,
        rungLabel,
      }),
    ).toEqual([
      ["Buffer", "0.0 s"],
      ["Behind live", "0 s"],
    ]);
    expect(
      streamRows({
        auto: false,
        facts: { bufferMs: 0, frameRate: 25, height: 2160, url: "/s", width: 3840 },
        lagSeconds: 0,
        rungLabel,
      }).slice(0, 2),
    ).toEqual([
      ["Quality", "4K"],
      ["Resolution", "3840×2160 @ 25"],
    ]);
  });
});

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
