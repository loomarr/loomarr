import { describe, expect, it } from "vitest";

import {
  cueLinesAt,
  nextSubtitles,
  selectSubtitleTrack,
  subtitleLabel,
  subtitlesAtPlayhead,
  subtitleWindow,
} from "../src/live-subtitles";

// An airing's text tracks, as the subtitles sidecar lists them (#2088 step 7).
const english = { language: "eng", title: "" };
const englishSDH = { language: "eng", title: "SDH" };
const spanish = { language: "spa", title: "" };
const tracks = [english, englishSDH, spanish];

describe("selectSubtitleTrack", () => {
  it("plays nothing when the viewer has them off", () => {
    expect(selectSubtitleTrack(tracks, "off")).toBe(-1);
  });

  it("plays the exact track chosen, not just its language", () => {
    expect(selectSubtitleTrack(tracks, englishSDH)).toBe(1);
  });

  it("falls back to the language where this programme lacks that exact track", () => {
    expect(selectSubtitleTrack([spanish, english], englishSDH)).toBe(1);
  });

  it("plays none where the programme does not carry the language", () => {
    expect(selectSubtitleTrack([spanish], english)).toBe(-1);
    expect(selectSubtitleTrack([], english)).toBe(-1);
  });
});

describe("nextSubtitles", () => {
  it("cycles off → each track → off", () => {
    const seen = [];
    let choice = nextSubtitles(tracks, "off");
    while (choice !== "off") {
      seen.push(choice);
      choice = nextSubtitles(tracks, choice);
    }
    expect(seen).toEqual([english, englishSDH, spanish]);
  });

  it("stays off when there is nothing to show", () => {
    expect(nextSubtitles([], "off")).toBe("off");
  });

  it("starts from the first track when the remembered one is not offered here", () => {
    expect(nextSubtitles([spanish], english)).toEqual(spanish);
  });
});

describe("subtitleWindow", () => {
  it("steps the window with the playhead, a little behind it, and asks for the chosen track", () => {
    expect(subtitleWindow(65_000, "off")).toEqual({ from: 45_000, to: 135_000 });
    expect(subtitleWindow(89_999, englishSDH)).toEqual({
      from: 45_000,
      language: "eng",
      title: "SDH",
      to: 135_000,
    });
  });
});

describe("subtitlesAtPlayhead", () => {
  // As the sidecar sends them: each track named in itself.
  const named = [
    { ...english, languageName: "English" },
    { ...spanish, languageName: "Español" },
  ];
  const airings = [
    { cues: [], startMs: 0, stopMs: 100, tracks: named },
    { cues: [], startMs: 100, stopMs: 200, tracks: [] },
  ];

  it("offers the tracks of the programme at the playhead, and the one the choice plays", () => {
    expect(subtitlesAtPlayhead(airings, 50, spanish)).toEqual({
      active: 1,
      playing: named[1],
      tracks: named,
    });
    expect(subtitlesAtPlayhead(airings, 150, spanish)).toEqual({
      active: -1,
      playing: undefined,
      tracks: [],
    });
    expect(subtitlesAtPlayhead(undefined, 50, spanish).tracks).toEqual([]);
  });
});

describe("subtitleLabel", () => {
  it("names the track in its own language, as the server names it, with its title beside it", () => {
    expect(subtitleLabel({ language: "spa", languageName: "Español", title: "" })).toBe("Español");
    expect(subtitleLabel({ language: "eng", languageName: "English", title: "SDH" })).toBe("English · SDH");
  });

  it("falls back to the code, then to Subtitles, where the server has no name", () => {
    expect(subtitleLabel({ language: "qaa", languageName: "", title: "" })).toBe("QAA");
    expect(subtitleLabel({ language: "", title: "Forced" })).toBe("Subtitles · Forced");
  });
});

describe("cueLinesAt", () => {
  it("reads the lines on screen at an instant, inline tags dropped", () => {
    const cues = [
      { endMs: 2_000, startMs: 1_000, text: "<i>Hello</i>\nthere" },
      { endMs: 3_000, startMs: 2_000, text: "Next" },
    ];
    expect(cueLinesAt(cues, 1_500)).toEqual(["Hello", "there"]);
    expect(cueLinesAt(cues, 2_000)).toEqual(["Next"]);
    expect(cueLinesAt(cues, 3_000)).toEqual([]);
  });
});
