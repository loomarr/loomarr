import type { PlayerChannel, PlayerSourcePort } from "@loomarr/player";
import { createChannelCatalogPort, createPlayUrlSourcePort, resolveStreamUrl } from "@loomarr/player/server";
import { describe, expect, expectTypeOf, it, vi } from "vitest";

const channel: PlayerChannel = {
  id: "science-fiction",
  inAppPlayable: true,
  name: "Science Fiction",
  number: 7,
};

describe("play URL source", () => {
  it("mints a scoped source from the paired server without forwarding auth to HLS", async () => {
    const request = vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          expiresAt: "2026-08-26T13:00:00Z",
          relativeUrl: "/v1/playout/hls/science/master.m3u8?sig=one",
          serverTimeMs: Date.parse("2026-08-26T12:59:50Z"),
          url: "http://localhost:8080/wrong.m3u8",
        }),
        { headers: { Date: "Wed, 26 Aug 2026 12:59:45 GMT" }, status: 200 },
      ),
    );
    const source = createPlayUrlSourcePort({
      baseUrl: "http://living-room:8080/",
      fetch: request,
    });
    const signal = new AbortController().signal;

    const result = await source.mint(channel, { maxResolution: 2160, video: ["hevc"] }, signal);

    expectTypeOf(source).toMatchTypeOf<PlayerSourcePort>();
    expect(result).toEqual({
      expiresAt: Date.parse("2026-08-26T13:00:00Z"),
      serverTimeMs: Date.parse("2026-08-26T12:59:50Z"),
      uri: "http://living-room:8080/v1/playout/hls/science/master.m3u8?sig=one",
    });
    expect(result.headers).toBeUndefined();
    expect(request).toHaveBeenCalledWith("/v1/channels/science-fiction/play-url", {
      body: JSON.stringify({ maxResolution: 2160, video: ["hevc"] }),
      headers: { "Content-Type": "application/json" },
      method: "POST",
      signal,
    });
  });

  it("fails closed when the authenticated mint is rejected", async () => {
    const source = createPlayUrlSourcePort({
      baseUrl: "https://loomarr.test",
      fetch: vi.fn().mockResolvedValue(new Response("revoked", { status: 401 })),
    });

    await expect(source.mint(channel, {}, new AbortController().signal)).rejects.toThrow(
      "Couldn't mint a play URL (401).",
    );
  });

  it("loads the generated Channel route into the controller's narrow catalog contract", async () => {
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
    const signal = new AbortController().signal;

    await expect(catalog.list(signal)).resolves.toEqual([
      { id: "seven", inAppPlayable: true, name: "Seven", number: 7 },
    ]);
    expect(request).toHaveBeenCalledWith("/v1/channels", { method: "GET", signal });
  });

  it("keeps an unsuccessful catalog response distinct from an empty catalog", async () => {
    const catalog = createChannelCatalogPort(
      vi.fn().mockResolvedValue(new Response("unavailable", { status: 503 })),
    );

    await expect(catalog.list(new AbortController().signal)).rejects.toThrow("Couldn't load channels (503).");
  });

  it("uses the absolute native URL only when the relative form is absent", () => {
    expect(
      resolveStreamUrl("https://paired.test", {
        relativeUrl: "",
        url: "https://edge.test/channel.m3u8?sig=two",
      }),
    ).toBe("https://edge.test/channel.m3u8?sig=two");
  });

  it("rejects a response with no usable stream address", () => {
    expect(() => resolveStreamUrl("https://paired.test", { relativeUrl: "", url: "" })).toThrow(
      "no stream address",
    );
  });

  it("joins repeated boundary slashes in linear time", () => {
    expect(
      resolveStreamUrl("https://paired.test////", {
        relativeUrl: "////v1/playout/hls/channel.m3u8",
        url: "",
      }),
    ).toBe("https://paired.test/v1/playout/hls/channel.m3u8");
  });

  it("keeps a malformed expiry explicit instead of inventing a lifetime", async () => {
    const source = createPlayUrlSourcePort({
      baseUrl: "https://paired.test",
      fetch: vi.fn().mockResolvedValue(
        new Response(
          JSON.stringify({
            expiresAt: "not-a-date",
            relativeUrl: "/v1/playout/hls/channel.m3u8?sig=three",
            url: "",
          }),
          { status: 200 },
        ),
      ),
    });

    await expect(source.mint(channel, {}, new AbortController().signal)).resolves.toEqual({
      expiresAt: undefined,
      uri: "https://paired.test/v1/playout/hls/channel.m3u8?sig=three",
    });
  });
});

describe("play URL source warm", () => {
  const mintBody = JSON.stringify({
    expiresAt: "2026-08-26T13:00:00Z",
    relativeUrl: "/v1/playout/hls/science/master.m3u8?sig=one&plan=full",
    url: "",
  });

  it("mints then fetches the signed playlist in speculative warm mode and drains the body", async () => {
    const request = vi
      .fn()
      .mockResolvedValueOnce(new Response(mintBody, { status: 200 }))
      .mockResolvedValueOnce(new Response("#EXTM3U\n", { status: 200 }));
    const source = createPlayUrlSourcePort({ baseUrl: "http://living-room:8080", fetch: request });
    const signal = new AbortController().signal;

    await source.warm?.(channel, {}, signal);

    const [warmUrl, init] = request.mock.calls[1] ?? [];
    const url = new URL(warmUrl);
    expect(url.pathname).toBe("/v1/playout/hls/science/master.m3u8");
    expect(url.searchParams.get("mode")).toBe("warm");
    expect(url.searchParams.get("sig")).toBe("one");
    expect(url.searchParams.get("plan")).toBe("full");
    expect(init).toMatchObject({ method: "GET", signal });
  });

  it("treats a busy host as a harmless miss", async () => {
    const request = vi
      .fn()
      .mockResolvedValueOnce(new Response(mintBody, { status: 200 }))
      .mockResolvedValueOnce(new Response("busy", { status: 503 }));
    const source = createPlayUrlSourcePort({ baseUrl: "http://living-room:8080", fetch: request });

    await expect(source.warm?.(channel, {}, new AbortController().signal)).resolves.toBeUndefined();
  });
});
