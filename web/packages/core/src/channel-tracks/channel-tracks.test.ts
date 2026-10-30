import { describe, expect, it, vi } from "vitest";

import { createChannelTracksSource } from "./channel-tracks";

describe("channel tracks source", () => {
  it("asks for the programme airing at the instant", async () => {
    const body = { audio: [{ index: 1, language: "eng" }], defaultAudio: 1, subtitles: [] };
    const request = vi.fn().mockResolvedValue(new Response(JSON.stringify(body), { status: 200 }));
    const signal = new AbortController().signal;

    await expect(createChannelTracksSource(request).load("ch-1", 1_700_000_000_000, signal)).resolves.toEqual(
      body,
    );
    expect(request).toHaveBeenCalledWith("/v1/channels/ch-1/tracks?at=1700000000000", {
      method: "GET",
      signal,
    });
  });

  it("says the tracks failed rather than offering no audio", async () => {
    const request = vi.fn().mockResolvedValue(new Response(null, { status: 503 }));
    await expect(
      createChannelTracksSource(request).load("ch-1", 0, new AbortController().signal),
    ).rejects.toThrow("Couldn't load the programme's tracks (503).");
  });
});
