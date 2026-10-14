import { liveHlsConfig } from "@loomarr/player/browser";
import Hls from "hls.js";
import { describe, expect, it } from "vitest";

// The channel packager (#1512 phase 2) cuts 1 s fMP4 segments, lists them up to wall-clock now + 6 s
// and advertises EXT-X-SERVER-CONTROL:HOLD-BACK=6.0.
const packagerHoldBackSeconds = 6;
const dvrHorizonSeconds = 15 * 60;

// The real hls.js attach/transfer/destroy path needs only an EventTarget when no source is loaded.
// Track the video listeners so retired controllers cannot remain reachable after a channel switch.
class TransferMedia extends EventTarget {
  readonly listeners = new Map<string, Set<EventListenerOrEventListenerObject>>();
  paused = true;
  seeking = false;
  currentTime = 0;

  override addEventListener(
    type: string,
    callback: EventListenerOrEventListenerObject | null,
    options?: boolean | AddEventListenerOptions,
  ) {
    if (callback) {
      const callbacks = this.listeners.get(type) ?? new Set();
      callbacks.add(callback);
      this.listeners.set(type, callbacks);
    }
    super.addEventListener(type, callback, options);
  }

  override removeEventListener(
    type: string,
    callback: EventListenerOrEventListenerObject | null,
    options?: boolean | EventListenerOptions,
  ) {
    if (callback) this.listeners.get(type)?.delete(callback);
    super.removeEventListener(type, callback, options);
  }

  getAttribute() {
    return null;
  }
  removeAttribute() {}
  load() {}
}

describe("live channel hls.js config", () => {
  it("releases every retired controller's media listeners after repeated channel transfers", () => {
    const media = new TransferMedia();
    for (let index = 0; index < 10; index++) {
      const hls = new Hls({ ...liveHlsConfig });
      try {
        hls.attachMedia(media as unknown as HTMLVideoElement);
        expect(hls.transferMedia()?.media).toBe(media);
      } finally {
        hls.destroy();
      }
    }
    const remaining = [...media.listeners.values()].reduce((count, listeners) => count + listeners.size, 0);
    expect(remaining).toBe(0);
  });

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
