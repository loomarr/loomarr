import { afterEach, describe, expect, it, vi } from "vitest";
import { deviceProfile } from "./device-profile";

// device-profile probes MediaSource.isTypeSupported to report what the client can direct-play above
// the h264/aac floor (§9.1 V48). We stub isTypeSupported per case to assert the mapping without a
// real decoder, and confirm the safety posture: prove nothing → empty profile (server → baseline).

const stubSupported = (supported: (mime: string) => boolean) => {
  vi.stubGlobal("MediaSource", { isTypeSupported: (m: string) => supported(m) });
};

afterEach(() => vi.unstubAllGlobals());

describe("deviceProfile", () => {
  it("advertises nothing above the floor when the decoder supports nothing extra", () => {
    stubSupported(() => false);
    const p = deviceProfile();
    expect(p.video).toEqual([]);
    expect(p.audio).toEqual([]);
    expect(p.video10bit).toBe(false);
  });

  it("advertises hevc (8-bit) but not 10-bit when only Main profile decodes", () => {
    // Main profile string contains "hvc1.1.6" / "hev1.1.6"; Main 10 contains ".2.4.".
    stubSupported((m) => m.includes(".1.6."));
    const p = deviceProfile();
    expect(p.video).toContain("hevc");
    // ⚠ The server gates its 10-bit COPY on this exact bit — an 8-bit-only decoder must not set it,
    // or it would be handed a 10-bit stream it can't decode (a black frame).
    expect(p.video10bit).toBe(false);
  });

  it("sets video10bit only when the 10-bit HEVC profile itself decodes", () => {
    stubSupported((m) => m.includes(".2.4.") || m.includes(".1.6."));
    const p = deviceProfile();
    expect(p.video).toContain("hevc");
    expect(p.video10bit).toBe(true);
  });

  it("advertises surround audio codecs that decode", () => {
    // mp4a.a6 = EAC3, mp4a.a5 = AC3.
    stubSupported((m) => m.includes("mp4a.a6") || m.includes("mp4a.a5"));
    const p = deviceProfile();
    expect(p.audio).toEqual(expect.arrayContaining(["eac3", "ac3"]));
  });

  it("is safe when MediaSource is absent (native-HLS browsers)", () => {
    vi.stubGlobal("MediaSource", undefined);
    const p = deviceProfile();
    // No decoder to ask ⇒ advertise nothing ⇒ server serves the baseline. Never throws.
    expect(p.video).toEqual([]);
    expect(p.audio).toEqual([]);
    expect(p.video10bit).toBe(false);
  });
});
