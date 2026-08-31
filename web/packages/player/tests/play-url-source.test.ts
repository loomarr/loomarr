import type { PlayerChannel } from "@loomarr/player";
import { createChannelCatalogPort, createPlayUrlSourcePort, resolveStreamUrl } from "@loomarr/player/server";
import { describe, expect, it, vi } from "vitest";

const channel: PlayerChannel = {
  id: "science-fiction",
  inAppPlayable: true,
  name: "Science Fiction",
  number: 7,
};

describe("play URL source", () => {
  it("prefers the paired server's relative stream address and never forwards the bearer to HLS", async () => {
    const request = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          expiresAt: "2026-08-26T13:00:00Z",
          relativeUrl: "/v1/playout/hls/science/master.m3u8?sig=one",
          url: "http://localhost:8080/wrong.m3u8",
        }),
        { status: 200 },
      ),
    );
    const source = createPlayUrlSourcePort({
      baseUrl: "http://living-room:8080/",
      fetch: request,
    });

    const result = await source.mint(
      channel,
      { maxResolution: 2160, video: ["hevc"] },
      new AbortController().signal,
    );

    expect(result).toEqual({
      expiresAt: Date.parse("2026-08-26T13:00:00Z"),
      uri: "http://living-room:8080/v1/playout/hls/science/master.m3u8?sig=one",
    });
    expect(result.headers).toBeUndefined();
    expect(request).toHaveBeenCalledWith(
      "/v1/channels/science-fiction/play-url",
      expect.objectContaining({
        body: JSON.stringify({ maxResolution: 2160, video: ["hevc"] }),
        headers: { "Content-Type": "application/json" },
      }),
    );
  });

  it("fails closed on an unsuccessful play URL response", async () => {
    const source = createPlayUrlSourcePort({
      baseUrl: "https://loomarr.test",
      fetch: vi.fn().mockResolvedValue(new Response("revoked", { status: 401 })),
    });

    await expect(source.mint(channel, {}, new AbortController().signal)).rejects.toThrow(
      "Couldn't mint a play URL (401).",
    );
  });

  it("reads the generated Channel route and returns only controller fields", async () => {
    const request = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          channels: [
            {
              id: "seven",
              inAppPlayable: true,
              lineup: [],
              name: "Seven",
              number: 7,
              pendingCount: 0,
              policy: {},
              programCount: 0,
              revision: 1,
              slotCount: 0,
              status: "live",
              strategy: "sequential",
            },
          ],
        }),
        { status: 200 },
      ),
    );
    const catalog = createChannelCatalogPort(request);

    await expect(catalog.list(new AbortController().signal)).resolves.toEqual([
      { id: "seven", inAppPlayable: true, name: "Seven", number: 7 },
    ]);
    expect(request).toHaveBeenCalledWith("/v1/channels", expect.objectContaining({ method: "GET" }));
  });

  it("uses the absolute URL only when the relative form is absent", () => {
    expect(
      resolveStreamUrl("https://paired.test", {
        relativeUrl: "",
        url: "https://edge.test/channel.m3u8?sig=two",
      }),
    ).toBe("https://edge.test/channel.m3u8?sig=two");
    expect(() => resolveStreamUrl("https://paired.test", { relativeUrl: "", url: "" })).toThrow(
      "no stream address",
    );
  });

  it("joins relative stream addresses with repeated slashes in linear time", () => {
    expect(
      resolveStreamUrl("https://paired.test////", {
        relativeUrl: "////v1/playout/hls/channel.m3u8",
        url: "",
      }),
    ).toBe("https://paired.test/v1/playout/hls/channel.m3u8");
  });
});
