import { nextSubtitles, selectSubtitleTrack } from "@loomarr/player/browser";
import { describe, expect, it } from "vitest";

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
