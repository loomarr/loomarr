import { describe, expect, it, vi } from "vitest";

import {
  createChannelSubtitlesSource,
  cueLines,
  needsStretch,
  SUBTITLE_LEAD_MS,
  SUBTITLE_WINDOW_MS,
  type SubtitleStretch,
} from "./channel-subtitles";

describe("channel subtitles source", () => {
  it("asks for the chosen track's cues over a stretch of the channel's clock", async () => {
    const body = { cues: [{ endMs: 2_000, startMs: 1_000, text: "Hello" }], pending: false, tracks: [] };
    const request = vi.fn(() => Promise.resolve(new Response(JSON.stringify(body))));
    const signal = new AbortController().signal;

    const got = await createChannelSubtitlesSource(request as typeof fetch).load(
      "ch-1",
      { from: 1_000, to: 121_000, track: "en-sdh" },
      signal,
    );

    expect(request).toHaveBeenCalledWith("/v1/channels/ch-1/subtitles?from=1000&to=121000&track=en-sdh", {
      method: "GET",
      signal,
    });
    expect(got).toEqual(body);
  });

  it("fails on an error answer, so no lines are drawn from a guess", async () => {
    const request = vi.fn(() => Promise.resolve(new Response("{}", { status: 404 })));
    await expect(
      createChannelSubtitlesSource(request as typeof fetch).load(
        "ch-1",
        { from: 0, to: 1 },
        new AbortController().signal,
      ),
    ).rejects.toThrow("Couldn't read the channel's subtitles (404).");
  });
});

describe("needsStretch", () => {
  const held: SubtitleStretch = {
    channelId: "ch-1",
    cues: [],
    from: 100_000,
    to: 100_000 + SUBTITLE_WINDOW_MS,
    track: "en",
    tracks: [],
  };
  const at = { channelId: "ch-1", playheadMs: 130_000, track: "en" };

  it("holds while the playhead is inside the stretch, short of its end", () => {
    expect(needsStretch(held, at)).toBe(false);
  });

  it("asks before the stretch runs out, so the next cues are there in time", () => {
    expect(needsStretch(held, { ...at, playheadMs: held.to - SUBTITLE_LEAD_MS })).toBe(true);
  });

  it("asks again for another channel, another track, or a playhead moved back (Start over, −15m)", () => {
    expect(needsStretch(undefined, at)).toBe(true);
    expect(needsStretch(held, { ...at, channelId: "ch-2" })).toBe(true);
    expect(needsStretch(held, { ...at, track: "es" })).toBe(true);
    expect(needsStretch(held, { ...at, playheadMs: 99_000 })).toBe(true);
  });
});

describe("cueLines", () => {
  const cues = [
    { endMs: 2_000, startMs: 1_000, text: "First" },
    { endMs: 4_000, startMs: 2_000, text: "Two\nlines" },
  ];

  it("draws the cue under the playhead, a line per newline", () => {
    expect(cueLines(cues, 2_500)).toEqual(["Two", "lines"]);
  });

  it("starts a cue on its first millisecond and ends it before its last", () => {
    expect(cueLines(cues, 1_000)).toEqual(["First"]);
    expect(cueLines(cues, 4_000)).toEqual([]);
  });
});
