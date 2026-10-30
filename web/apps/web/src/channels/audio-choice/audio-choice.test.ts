import { describe, expect, it } from "vitest";
import { audioChoice } from "./audio-choice";

// The stream's Default and Alternate audio, labelled from the tracks API at the playhead, with the
// viewer's remembered language choosing between them (#2088 decision 6).
describe("audioChoice", () => {
  const dubbed = {
    audio: [
      { index: 0, language: "spa" },
      { index: 1, language: "eng" },
      { index: 2, language: "eng", title: "Commentary" },
    ],
    defaultAudio: 1,
    alternateAudio: 0,
  };

  it("offers the default, then the alternate, named in themselves as the mock shows", () => {
    expect(audioChoice(dubbed, "default")).toEqual({
      alternate: false,
      options: [
        { label: "English · stereo", selected: true, value: "default" },
        { label: "Español · stereo", selected: false, value: "spa" },
      ],
    });
  });

  it("plays the alternate while it carries the viewer's language", () => {
    const choice = audioChoice(dubbed, "spa");
    expect(choice.alternate).toBe(true);
    expect(choice.options.map((o) => o.selected)).toEqual([false, true]);
  });

  it("falls back to the default when the alternate is another language, or there is none", () => {
    expect(audioChoice(dubbed, "fra").alternate).toBe(false);
    const single = audioChoice({ audio: [{ index: 0, language: "eng" }], defaultAudio: 0 }, "spa");
    expect(single).toEqual({
      alternate: false,
      options: [{ label: "English · stereo", selected: true, value: "default" }],
    });
  });

  it("offers nothing until the tracks are known", () => {
    expect(audioChoice(undefined, "spa")).toEqual({ alternate: false, options: [] });
    expect(audioChoice({ audio: [], defaultAudio: 0 }, "default")).toEqual({ alternate: false, options: [] });
  });
});
