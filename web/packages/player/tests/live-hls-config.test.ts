import { liveHlsConfig } from "@loomarr/player/browser";
import Hls from "hls.js";
import { describe, expect, it } from "vitest";

// The channel packager (#1512 phase 2) cuts 1 s fMP4 segments, lists them up to wall-clock now + 6 s
// and advertises EXT-X-SERVER-CONTROL:HOLD-BACK=6.0.
const packagerHoldBackSeconds = 6;
const dvrHorizonSeconds = 15 * 60;

describe("live channel hls.js config", () => {
  it("plays at the packager's hold-back, where 6 s of encoded media is already listed", () => {
    expect(liveHlsConfig.liveSyncDuration).toBe(packagerHoldBackSeconds);
    expect(liveHlsConfig.liveMaxLatencyDuration).toBeGreaterThan(dvrHorizonSeconds);
  });

  it("asks for the first fragment while the manifest is still being parsed", () => {
    expect(liveHlsConfig.startFragPrefetch).toBe(true);
  });

  it("does not use LL-HLS, which the packager does not serve", () => {
    expect(liveHlsConfig.lowLatencyMode).toBe(false);
  });

  it("is a config real hls.js accepts", () => {
    // hls.js throws when a *Count latency option is mixed with a seconds one.
    const hls = new Hls({ ...liveHlsConfig });
    try {
      expect(hls.config.liveSyncDuration).toBe(packagerHoldBackSeconds);
      expect(hls.config.lowLatencyMode).toBe(false);
    } finally {
      hls.destroy();
    }
  });
});
