import { describe, expect, it, vi } from "vitest";

import { createChannelTimelineSource, serverNowOf } from "./channel-timeline";

describe("channel timeline source", () => {
  it("reads the channel's timeline route and stamps when the answer arrived", async () => {
    const request = vi
      .fn()
      .mockResolvedValue(new Response(JSON.stringify({ airings: [], serverNowMs: 50_000 }), { status: 200 }));
    const signal = new AbortController().signal;

    await expect(createChannelTimelineSource(request, () => 7_000).load("ch-1", signal)).resolves.toEqual({
      airings: [],
      receivedAtMs: 7_000,
      serverNowMs: 50_000,
    });
    expect(request).toHaveBeenCalledWith("/v1/channels/ch-1/timeline", { method: "GET", signal });
  });

  it("says the schedule failed rather than handing back an empty one", async () => {
    const request = vi.fn().mockResolvedValue(new Response(null, { status: 502 }));
    await expect(
      createChannelTimelineSource(request).load("ch-1", new AbortController().signal),
    ).rejects.toThrow("Couldn't load the channel's schedule (502).");
  });

  it("projects the server's clock by the time since the answer, never backwards", () => {
    const timeline = { airings: [], receivedAtMs: 7_000, serverNowMs: 50_000 };
    expect(serverNowOf(timeline, 9_500)).toBe(52_500);
    expect(serverNowOf(timeline, 6_000)).toBe(50_000);
  });
});
