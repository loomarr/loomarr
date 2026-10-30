import { describe, expect, it, vi } from "vitest";

import { createChannelSubtitlesSource } from "./channel-subtitles";

describe("channel subtitles source", () => {
  it("asks for the window, and the chosen track's cues", async () => {
    const body = { airings: [] };
    const request = vi.fn().mockResolvedValue(new Response(JSON.stringify(body), { status: 200 }));
    const signal = new AbortController().signal;

    await expect(
      createChannelSubtitlesSource(request).load(
        "ch-1",
        { from: 0, language: "eng", title: "", to: 90 },
        signal,
      ),
    ).resolves.toEqual(body);
    expect(request).toHaveBeenCalledWith("/v1/channels/ch-1/subtitles?from=0&language=eng&title=&to=90", {
      method: "GET",
      signal,
    });
  });

  it("says the subtitles failed rather than showing none", async () => {
    const request = vi.fn().mockResolvedValue(new Response(null, { status: 500 }));
    await expect(
      createChannelSubtitlesSource(request).load("ch-1", { from: 0, to: 90 }, new AbortController().signal),
    ).rejects.toThrow("Couldn't load the channel's subtitles (500).");
  });
});
