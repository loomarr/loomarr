import { describe, expect, it } from "vitest";
import { AUTO_SENTINEL, audioOptions, withSaved } from "./audio-options";

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
      { value: "fra", label: "French" },
    ]);
  });

  it("never shows a track's raw title (an encoder handler name such as root)", () => {
    const opts = audioOptions([{ index: 0, language: "eng", title: "root" }]);
    expect(opts.map((o) => o.label)).toEqual(["Auto (default)", "English"]);
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
