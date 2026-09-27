import { selectLiveLevel } from "@loomarr/player/browser";
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
