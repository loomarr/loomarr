import { playableRungs, selectLiveLevel, selectQualityLevel } from "@loomarr/player/browser";
import { describe, expect, it } from "vitest";

// A 4K channel's master lists the baseline and one premium variant (#1512 G10). The premium runs its
// own 4K encode on the server, so a browser plays it only when it can decode that exact CODECS
// string, and never drifts onto it by bandwidth.
const baseline = { codecs: "avc1.640028,mp4a.40.2", height: 1080, videoRange: "SDR" };
const premiumHDR = { codecs: "hvc1.2.4.L150.90,mp4a.40.2", height: 2160, videoRange: "PQ" };
const premiumSDR = { codecs: "hvc1.1.6.L150.90,mp4a.40.2", height: 2160, videoRange: "SDR" };

const decodes =
  (...codecs: string[]) =>
  (mime: string) =>
    codecs.some((c) => mime === `video/mp4; codecs="${c}"`);

describe("selectLiveLevel", () => {
  it("plays the premium when the browser decodes its exact CODECS", () => {
    expect(selectLiveLevel([baseline, premiumHDR], decodes(premiumHDR.codecs))).toBe(1);
    expect(selectLiveLevel([baseline, premiumSDR], decodes(premiumSDR.codecs))).toBe(1);
  });

  it("falls back to the baseline when the premium's codec is not decodable", () => {
    // Chromium without HEVC: the premium must never be requested (it would start a 4K encode).
    expect(selectLiveLevel([baseline, premiumHDR], decodes(baseline.codecs))).toBe(0);
    // Main decodes but Main 10 does not: the exact string decides, not the codec family.
    expect(selectLiveLevel([baseline, premiumHDR], decodes(premiumSDR.codecs))).toBe(0);
  });

  it("never offers a premium whose CODECS the master left out", () => {
    const unnamed = { height: 2160, videoRange: "PQ" };
    expect(selectLiveLevel([baseline, unnamed], () => true)).toBe(0);
  });

  it("plays the only level of a baseline-only channel", () => {
    expect(selectLiveLevel([baseline], () => false)).toBe(0);
    expect(selectLiveLevel([premiumHDR, baseline], decodes(baseline.codecs))).toBe(1);
  });
});

// The viewer's quality (#2088 decision 7): the channel's own rungs, never a new encode.
describe("playableRungs and selectQualityLevel", () => {
  it("offers the baseline always and the premium only where it decodes", () => {
    expect(playableRungs([baseline, premiumHDR], decodes(premiumHDR.codecs))).toEqual([
      { height: 1080, index: 0 },
      { height: 2160, index: 1 },
    ]);
    expect(playableRungs([baseline, premiumHDR], decodes(baseline.codecs))).toEqual([
      { height: 1080, index: 0 },
    ]);
  });

  it("plays the viewer's rung when it is offered here, and Auto's choice otherwise", () => {
    const canPlay = decodes(premiumHDR.codecs);
    expect(selectQualityLevel([baseline, premiumHDR], canPlay, 1080)).toBe(0);
    expect(selectQualityLevel([baseline, premiumHDR], canPlay, "auto")).toBe(1);
    // A remembered 4K on a channel (or browser) without it: Auto decides.
    expect(selectQualityLevel([baseline], canPlay, 2160)).toBe(0);
    expect(selectQualityLevel([baseline, premiumHDR], decodes(baseline.codecs), 2160)).toBe(0);
  });
});
