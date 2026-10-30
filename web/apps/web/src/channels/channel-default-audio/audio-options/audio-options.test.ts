import { describe, expect, it } from "vitest";
import { AUTO_SENTINEL, audioOptions, burnOptions, OFF_SENTINEL, withSaved } from "./audio-options";

describe("burnOptions", () => {
  it("leads with Off, then each language of the airing programme's image tracks, never text ones", () => {
    const opts = burnOptions([
      { index: 0, language: "eng" },
      { index: 1, language: "eng", burnable: true },
      { index: 2, language: "eng", burnable: true, title: "SDH" },
      { index: 3, language: "fra" },
      { index: 4, burnable: true },
    ]);
    expect(opts).toEqual([
      { value: OFF_SENTINEL, label: "Off" },
      { value: "eng", label: "English" },
    ]);
  });
});

describe("audioOptions", () => {
  it("leads with Auto, then one entry per distinct language the airing media carries", () => {
    const opts = audioOptions([
      { index: 0, language: "eng" },
      { index: 1, language: "eng", title: "Commentary" },
      { index: 2, language: "fra", title: "Stereo" },
      { index: 3 },
    ]);
    expect(opts).toEqual([
      { value: AUTO_SENTINEL, label: "Auto (default)" },
      { value: "eng", label: "English" },
      { value: "fra", label: "French · Stereo" },
    ]);
  });
});

describe("withSaved", () => {
  const opts = [{ value: AUTO_SENTINEL, label: "Auto (default)" }];

  it("keeps a saved language the airing media doesn't carry, labelled by its code", () => {
    expect(withSaved(opts, "jpn")).toEqual([...opts, { value: "jpn", label: "JPN" }]);
  });

  it("adds nothing when the saved value is already offered", () => {
    expect(withSaved(opts, AUTO_SENTINEL)).toBe(opts);
  });
});
