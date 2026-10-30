import { selectAudioRendition } from "@loomarr/player/browser";
import { describe, expect, it } from "vitest";

// A watched channel's master names two audio renditions per variant (#2088 decision 6). hls.js
// lists them by `name`, a native player (Safari) by `label`; the viewer's choice picks by that name.
describe("selectAudioRendition", () => {
  const group = [{ name: "Default" }, { name: "Alternate" }];

  it("plays the Alternate when the viewer chose it, the Default otherwise", () => {
    expect(selectAudioRendition(group, true)).toBe(1);
    expect(selectAudioRendition(group, false)).toBe(0);
  });

  it("finds them by name, wherever the master lists them", () => {
    expect(selectAudioRendition([{ name: "Alternate" }, { name: "Default" }], true)).toBe(0);
    expect(selectAudioRendition([{ name: "Alternate" }, { name: "Default" }], false)).toBe(1);
  });

  it("reads a native AudioTrack's label", () => {
    expect(selectAudioRendition([{ label: "Default" }, { label: "Alternate" }], true)).toBe(1);
  });

  it("falls back to the Default when the master offers no Alternate, and to none without either", () => {
    expect(selectAudioRendition([{ name: "Default" }], true)).toBe(0);
    expect(selectAudioRendition([], true)).toBe(-1);
    expect(selectAudioRendition([{ name: "audio" }], false)).toBe(-1);
  });
});
