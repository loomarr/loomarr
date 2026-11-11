import { describe, expect, it, vi } from "vitest";

import { createChannelAudioSource } from "./channel-audio";

const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), { status });

describe("channel audio source", () => {
  it("reads the airing tracks and the channel, and marks the channel's language", async () => {
    const request = vi.fn((url: string) =>
      Promise.resolve(
        url.endsWith("/tracks")
          ? json({
              audio: [
                { index: 0, language: "eng" },
                { index: 1, language: "spa" },
              ],
              subtitles: [],
            })
          : json({ id: "ch-1", policy: { playout: { audioLanguage: "spa" } } }),
      ),
    );
    const signal = new AbortController().signal;

    const choices = await createChannelAudioSource(request as typeof fetch).load("ch-1", signal);

    expect(request).toHaveBeenCalledWith("/v1/channels/ch-1/tracks", { method: "GET", signal });
    expect(request).toHaveBeenCalledWith("/v1/channels/ch-1", { method: "GET", signal });
    expect(choices.map((c) => [c.value, c.selected])).toEqual([
      ["auto", false],
      ["eng", false],
      ["spa", true],
    ]);
  });

  it("fails when either read fails, so the menu shows nothing stale", async () => {
    const request = vi.fn((url: string) =>
      Promise.resolve(url.endsWith("/tracks") ? json({ audio: [], subtitles: [] }) : json({}, 404)),
    );
    await expect(
      createChannelAudioSource(request as typeof fetch).load("ch-1", new AbortController().signal),
    ).rejects.toThrow("Couldn't read the channel's audio (404).");
  });
});
