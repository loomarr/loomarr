import type { DeviceProfileBody } from "@loomarr/api/models/deviceProfileBody";

// deviceProfile() — what THIS browser can direct-play, as a DeviceProfile the play-url endpoint uses
// (§9.1 V48/V50). Its role is a yes/no GATE, not a plan picker: under the V50 content-driven model the
// CHANNEL's broadcast codec decides the timeline codec/container, and this profile only tells the
// server whether this client can decode that codec natively. When the channel is HEVC and the browser
// can decode HEVC, the server serves a `-c:v copy` fMP4 stream instead of a full HEVC→h264 transcode
// (the black-frame-free, GPU-free path a media server has always had); when it can't, the server
// down-converts that same channel to h264/TS for this client. An h264 channel is served h264/TS to
// everyone regardless of what this profile advertises.
//
// ⚠ We ask MediaSource.isTypeSupported, NOT video.canPlayType — the same lesson the transport split
// learned (use-hls-player.ts): Chromium's canPlayType lies ("maybe" for things it cannot decode),
// while MSE (which is the pipeline hls.js actually feeds) answers for the real decoder. h264/aac are
// the implied floor and never sent; we only report what we can prove ABOVE it, so a browser that
// proves nothing gets the safe baseline. Being conservative here is correct: a false positive is a
// black frame, a false negative is only a transcode.

// isTypeSupported is a thin, guarded wrapper — MediaSource may be absent (old Safari uses native HLS
// and never constructs one), in which case we advertise nothing and the server serves baseline.
const isTypeSupported = (mime: string): boolean => {
  const MS = typeof self !== "undefined" ? self.MediaSource : undefined;
  return !!MS && typeof MS.isTypeSupported === "function" && MS.isTypeSupported(mime);
};

// HEVC codec strings. Main profile (hvc1.1.6.L93.B0) is the 8-bit baseline; Main 10 (hvc1.2.4.L120.B0)
// is the 10-bit profile. We test both because they gate DIFFERENT plans server-side (hevc8 vs the
// 10-bit part of hevc10), and a decoder can support one without the other. `hev1`/`hvc1` are the two
// sample-entry fourccs; a decoder that does HEVC accepts at least one, so we OR them.
const HEVC_8BIT = ['video/mp4; codecs="hvc1.1.6.L93.B0"', 'video/mp4; codecs="hev1.1.6.L93.B0"'];
const HEVC_10BIT = ['video/mp4; codecs="hvc1.2.4.L120.B0"', 'video/mp4; codecs="hev1.2.4.L120.B0"'];
// Surround audio the browser can decode in-MSE. EAC3/AC3 support is real on Safari and some Chromium
// builds; where absent, the server transcodes the audio to AAC (video still copies). mp4a.a6 = EAC3,
// mp4a.a5 = AC3.
const EAC3 = 'audio/mp4; codecs="mp4a.a6"';
const AC3 = 'audio/mp4; codecs="mp4a.a5"';

const anySupported = (mimes: string[]): boolean => mimes.some(isTypeSupported);

// deviceProfile builds the body sent on POST /v1/channels/{id}/play-url. Only capabilities ABOVE the
// h264/aac floor are listed; an empty result (no HEVC, no surround) is a valid profile that resolves
// to baseline. maxResolution comes from the screen so the server can cap the ladder for a small device.
const deviceProfile = (): DeviceProfileBody => {
  const video: string[] = [];
  const audio: string[] = [];

  const hevc8 = anySupported(HEVC_8BIT);
  const hevc10 = anySupported(HEVC_10BIT);
  if (hevc8 || hevc10) video.push("hevc");
  if (isTypeSupported(EAC3)) audio.push("eac3");
  if (isTypeSupported(AC3)) audio.push("ac3");

  const px = Math.round(Math.max(window.screen.width, window.screen.height) * (window.devicePixelRatio || 1));

  return {
    video,
    audio,
    // 10-bit is only claimed when the 10-bit HEVC profile itself decodes — the server gates its
    // 10-bit copy on this exact bit, so a decoder that does 8-bit HEVC only must NOT set it.
    video10bit: hevc10,
    // HDR display is not something MSE exposes reliably; leave it to a future, explicit probe rather
    // than guess. Omitted (false) keeps the server on the SDR/tone-map-safe path.
    hdr: false,
    maxResolution: Number.isFinite(px) && px > 0 ? px : 0,
  };
};

export { deviceProfile };
