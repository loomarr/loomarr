import { describe, expect, it } from "vitest";

import { nextSubtitle, SUBTITLES_OFF, subtitleChoices } from "./subtitle-choices";

const track = (key: string, extra: { forced?: boolean; hearingImpaired?: boolean } = {}) => ({
  forced: false,
  hearingImpaired: false,
  key,
  language: key.split("-")[0] === "und" ? "" : (key.split("-")[0] ?? ""),
  title: "",
  ...extra,
});

const rows = (choices: ReturnType<typeof subtitleChoices>) =>
  choices.map((c) => `${c.selected ? "✓ " : ""}${c.label}${c.detail ? ` (${c.detail})` : ""}`);

describe("subtitleChoices", () => {
  it("leads with Off, then each language, a hearing-impaired track as with sounds", () => {
    const tracks = [track("en"), track("en-sdh", { hearingImpaired: true }), track("es")];
    expect(rows(subtitleChoices(tracks, SUBTITLES_OFF))).toEqual([
      "✓ Off",
      "English",
      "English, with sounds (For the deaf and hard of hearing)",
      "Spanish",
    ]);
  });

  it("names an untagged track Unknown language, never by its own title", () => {
    expect(rows(subtitleChoices([{ ...track("und"), title: "Track 3" }], "und"))).toEqual([
      "Off",
      "✓ Unknown language",
    ]);
  });

  it("lists a forced track only for a language with no full track", () => {
    const tracks = [track("en-forced", { forced: true }), track("en"), track("fr-forced", { forced: true })];
    expect(subtitleChoices(tracks, "").map((c) => c.value)).toEqual([SUBTITLES_OFF, "en", "fr-forced"]);
  });

  it("marks the language's full track when the programme has no track by the remembered key", () => {
    // The server plays English for en-sdh on a programme without one, so English is what is marked.
    expect(rows(subtitleChoices([track("en"), track("es")], "en-sdh"))).toEqual([
      "Off",
      "✓ English",
      "Spanish",
    ]);
  });

  it("keeps a remembered language this programme doesn't carry, marked", () => {
    expect(rows(subtitleChoices([track("en")], "ja"))).toEqual(["Off", "English", "✓ Japanese"]);
  });

  it("is Off alone, marked, for a programme with no text subtitles", () => {
    expect(rows(subtitleChoices([], ""))).toEqual(["✓ Off"]);
  });
});

describe("nextSubtitle", () => {
  it("steps to the choice after the marked one, round to Off", () => {
    const tracks = [track("en"), track("es")];
    expect(nextSubtitle(subtitleChoices(tracks, SUBTITLES_OFF))).toBe("en");
    expect(nextSubtitle(subtitleChoices(tracks, "en"))).toBe("es");
    expect(nextSubtitle(subtitleChoices(tracks, "es"))).toBe(SUBTITLES_OFF);
  });
});
