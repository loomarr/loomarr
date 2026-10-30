import { syncSubtitleTrack } from "@loomarr/player/browser";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// jsdom has neither text tracks nor VTTCue: a track that keeps its cues, and a cue that keeps its times.
class FakeCue {
  constructor(
    readonly startTime: number,
    readonly endTime: number,
    readonly text: string,
  ) {}
}

const fakeTrack = (label: string) => {
  const cues: FakeCue[] = [];
  return {
    addCue: (c: FakeCue) => cues.push(c),
    cues,
    label,
    mode: "disabled" as TextTrackMode,
    removeCue: (c: FakeCue) => cues.splice(cues.indexOf(c), 1),
  };
};

const fakeVideo = () => {
  const tracks: ReturnType<typeof fakeTrack>[] = [];
  const addTextTrack = vi.fn((_kind: string, label: string) => {
    tracks.push(fakeTrack(label));
    return tracks.at(-1);
  });
  return {
    addTextTrack,
    get cues() {
      return tracks[0]?.cues ?? [];
    },
    get track() {
      return tracks[0];
    },
    tracks,
    video: { addTextTrack } as unknown as HTMLVideoElement,
  };
};

const WALL = Date.UTC(2026, 9, 30, 20);
const english = (text = "Hello") => ({
  cues: [{ endMs: WALL + 3_000, startMs: WALL + 1_000, text }],
  label: "English",
  language: "eng",
});

beforeEach(() => vi.stubGlobal("VTTCue", FakeCue));
afterEach(() => vi.unstubAllGlobals());

describe("syncSubtitleTrack", () => {
  it("places wall-clock cues on media time by the anchor's offset, and shows the track", () => {
    const fake = fakeVideo();
    // Media time 0 is WALL − 100 s: a cue 1 s after WALL plays at 101 s.
    syncSubtitleTrack(fake.video, english(), WALL - 100_000);
    expect(fake.track?.mode).toBe("showing");
    expect(fake.cues.map((c) => [c.startTime, c.endTime, c.text])).toEqual([[101, 103, "Hello"]]);
  });

  it("keeps one track per choice, re-placing only when the cues or the offset move", () => {
    const fake = fakeVideo();
    const subtitles = english();
    syncSubtitleTrack(fake.video, subtitles, WALL - 100_000);
    const first = fake.cues[0];
    syncSubtitleTrack(fake.video, subtitles, WALL - 100_100); // a wobble, not a move
    expect(fake.cues[0]).toBe(first);

    syncSubtitleTrack(fake.video, subtitles, WALL - 40_000); // a retune: media time restarted
    expect(fake.cues.map((c) => c.startTime)).toEqual([41]);
    syncSubtitleTrack(fake.video, english("Next"), WALL - 40_000);
    expect(fake.cues.map((c) => c.text)).toEqual(["Next"]);
    expect(fake.addTextTrack).toHaveBeenCalledOnce();
  });

  it("gives another choice its own track, named for it, with the first turned off", () => {
    const fake = fakeVideo();
    syncSubtitleTrack(fake.video, english(), WALL);
    const spanish = {
      cues: [{ endMs: WALL + 3_000, startMs: WALL + 1_000, text: "Hola" }],
      label: "Spanish",
      language: "spa",
    };
    syncSubtitleTrack(fake.video, spanish, WALL);
    expect(fake.tracks.map((t) => [t.label, t.mode])).toEqual([
      ["English", "disabled"],
      ["Spanish", "showing"],
    ]);
    // Back to English: its track again, not a third.
    syncSubtitleTrack(fake.video, english(), WALL);
    expect(fake.tracks.map((t) => t.mode)).toEqual(["showing", "disabled"]);
  });

  it("hides the track when subtitles are off, and waits for an anchor before placing", () => {
    const fake = fakeVideo();
    syncSubtitleTrack(fake.video, english(), undefined);
    expect(fake.tracks).toEqual([]);
    syncSubtitleTrack(fake.video, english(), WALL);
    syncSubtitleTrack(fake.video, undefined, WALL);
    expect(fake.track?.mode).toBe("disabled");
  });
});
