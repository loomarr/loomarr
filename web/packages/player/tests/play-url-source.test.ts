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
    expiresAt: "2999-01-01T00:00:00Z",
    relativeUrl: "/v1/playout/hls/science/master.m3u8?sig=one&plan=full",
    serverTimeMs: 1_000_000,
    url: "",
  });
  const manifest = [
    "#EXTM3U",
    '#EXT-X-MAP:URI="init-a.mp4?sig=one"',
    "#EXTINF:4.0,",
    "seg-1.m4s?sig=one",
    '#EXT-X-MAP:URI="init-b.mp4?sig=one"',
    "#EXTINF:4.0,",
    "seg-2.m4s?sig=one",
  ].join("\n");
  const mint = () => new Response(mintBody, { status: 200 });
  const paths = (request: ReturnType<typeof vi.fn>) =>
    request.mock.calls.map(([url]) => {
      const parsed = new URL(url as string, "http://base.test");
      return `${parsed.pathname.split("/").pop()}${parsed.searchParams.get("mode") ? `?mode=${parsed.searchParams.get("mode")}` : ""}`;
    });
  const port = (request: ReturnType<typeof vi.fn>) =>
    createPlayUrlSourcePort({
      baseUrl: "http://living-room:8080",
      fetch: request as unknown as typeof fetch,
    });

  it.each([false, true])(
    "follows real variant playlists before certifying a warm (premium=%s)",
    async (premium) => {
      const variants = ["1080p-h264-sdr", ...(premium ? ["4k-hevc-hdr"] : [])];
      const master = [
        "#EXTM3U",
        ...variants.flatMap((format) => ["#EXT-X-STREAM-INF:BANDWIDTH=1", `${format}.m3u8?sig=one&viewer=v`]),
      ].join("\n");
      const drained: string[] = [];
      const request = vi.fn(async (input: string | URL | Request) => {
        const url = String(input);
        if (url.endsWith("play-url")) return mint();
        const parsed = new URL(url);
        const name = parsed.pathname.split("/").pop() ?? "";
        if (name === "master.m3u8") return new Response(master);
        if (name.endsWith(".m3u8")) {
          expect(parsed.searchParams.get("mode")).toBe("warm");
          const format = name.replace(".m3u8", "");
          return new Response(
            `#EXTM3U\n#EXT-X-MAP:URI="${format}-init.mp4?sig=one"\n#EXTINF:1,\n${format}-seg.m4s?sig=one\n`,
          );
        }
        expect(parsed.searchParams.get("mode")).toBeNull();
        const response = new Response("bytes");
        const drain = response.arrayBuffer.bind(response);
        response.arrayBuffer = async () => {
          drained.push(name);
          return drain();
        };
        return response;
      });

      const result = await port(request).warm?.(channel, {}, new AbortController().signal);

      expect(result?.warmed).toBe(true);
      expect(drained.sort()).toEqual(
        variants.flatMap((format) => [`${format}-init.mp4`, `${format}-seg.m4s`]).sort(),
      );
      expect(result?.mediaUri).toBe(
        premium
          ? undefined
          : "http://living-room:8080/v1/playout/hls/science/1080p-h264-sdr.m3u8?sig=one&viewer=v",
      );
      expect(result?.uri).not.toContain("mode=");
    },
  );

  // Prepared media is retired (#1512 phase 4): the warm is one speculative live start, never a
  // prepared probe first.
  it("warms live under speculative admission and prefetches only the init and newest segment", async () => {
    const request = vi
      .fn()
      .mockResolvedValueOnce(mint())
      .mockResolvedValueOnce(new Response(manifest, { status: 200 }))
      .mockImplementation(() => Promise.resolve(new Response("bytes", { status: 200 })));
    const signal = new AbortController().signal;

    const warmed = await port(request).warm?.(channel, {}, signal);

    expect(paths(request)).toEqual(["play-url", "master.m3u8?mode=warm", "init-b.mp4", "seg-2.m4s"]);
    expect(warmed?.warmed).toBe(true);
    expect(request.mock.calls[1]?.[1]).toMatchObject({ method: "GET", signal });
  });

  it("returns the exact signed URL for the real tune, without the warm-mode hint", async () => {
    const request = vi
      .fn()
      .mockResolvedValueOnce(mint())
      .mockResolvedValueOnce(new Response(manifest, { status: 200 }))
      .mockImplementation(() => Promise.resolve(new Response("bytes", { status: 200 })));

    const warmed = await port(request).warm?.(channel, {}, new AbortController().signal);

    expect(warmed?.uri).toBe("http://living-room:8080/v1/playout/hls/science/master.m3u8?sig=one&plan=full");
    expect(warmed?.expiresAt).toBe(Date.parse("2999-01-01T00:00:00Z"));
    expect(warmed?.serverTimeMs).toBe(1_000_000);
  });

  it("keeps the source but does not certify a warm whose assets were unavailable", async () => {
    const request = vi
      .fn()
      .mockResolvedValueOnce(mint())
      .mockResolvedValueOnce(new Response(manifest, { status: 200 }))
      .mockImplementation(() => Promise.resolve(new Response("gone", { status: 404 })));

    const warmed = await port(request).warm?.(channel, {}, new AbortController().signal);

    expect(warmed?.warmed).toBe(false);
    expect(warmed?.uri).toContain("sig=one");
  });

  // #1037: a native tune may skip the master a warm already read, but only when the master offers
  // no choice; beside a premium variant the player must still choose.
  it("hands back the master's only variant, signed, and none when the master offers a choice", async () => {
    const variant = (uri: string) => [`#EXT-X-STREAM-INF:BANDWIDTH=1,RESOLUTION=1920x1080`, uri];
    const master = (...variants: string[]) => ["#EXTM3U", ...variants.flatMap(variant)].join("\n");
    const warmWith = async (body: string) => {
      const request = vi
        .fn()
        .mockResolvedValueOnce(mint())
        .mockResolvedValueOnce(new Response(body, { status: 200 }))
        .mockImplementation((url: string) =>
          Promise.resolve(
            new Response(new URL(url).pathname.endsWith(".m3u8") ? manifest : "bytes", { status: 200 }),
          ),
        );
      return port(request).warm?.(channel, {}, new AbortController().signal);
    };

    const single = await warmWith(master("1080p-h264-sdr.m3u8?sig=one&viewer=v"));
    const choice = await warmWith(master("1080p-h264-sdr.m3u8?sig=one", "4k-hevc-hdr.m3u8?sig=one"));

    expect(single?.mediaUri).toBe(
      "http://living-room:8080/v1/playout/hls/science/1080p-h264-sdr.m3u8?sig=one&viewer=v",
    );
    expect(single?.uri).toContain("master.m3u8");
    expect(choice?.mediaUri).toBeUndefined();
  });

  it("treats a busy host as a harmless miss that still hands back the signed URL", async () => {
    const request = vi
      .fn()
      .mockResolvedValueOnce(mint())
      .mockResolvedValueOnce(new Response("busy", { status: 503 }));

    const warmed = await port(request).warm?.(channel, {}, new AbortController().signal);

    expect(warmed?.warmed).toBe(false);
    expect(warmed?.uri).toContain("sig=one");
  });

  it.each([new Response("busy", { status: 503 }), new Response("not a playlist")])(
    "does not certify an unavailable or malformed rendition",
    async (media) => {
      const request = vi
        .fn()
        .mockResolvedValueOnce(mint())
        .mockResolvedValueOnce(
          new Response("#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1\n1080p-h264-sdr.m3u8?sig=one\n"),
        )
        .mockResolvedValueOnce(media);
      const result = await port(request).warm?.(channel, {}, new AbortController().signal);
      expect(result?.warmed).toBe(false);
      expect(result?.mediaUri).toBeUndefined();
      expect(request).toHaveBeenCalledTimes(3);
      expect(media.bodyUsed).toBe(true);
    },
  );
});

describe("play URL source still address", () => {
  const respond = (body: Record<string, unknown>) =>
    vi.fn().mockResolvedValue(
      new Response(
        JSON.stringify({
          expiresAt: "2026-08-26T13:00:00Z",
          relativeUrl: "/v1/playout/hls/science/master.m3u8?sig=one",
          serverTimeMs: 1,
          url: "",
          ...body,
        }),
        { status: 200 },
      ),
    );
  const mint = (request: ReturnType<typeof respond>) =>
    createPlayUrlSourcePort({ baseUrl: "http://living-room:8080/", fetch: request }).mint(
      channel,
      {},
      new AbortController().signal,
    );

  it("carries the signed still for the switch overlay, resolved like the stream address", async () => {
    const result = await mint(
      respond({ relativeStillUrl: "/v1/playout/still/science?sig=one", stillUrl: "http://elsewhere/still" }),
    );
    expect(result.stillUri).toBe("http://living-room:8080/v1/playout/still/science?sig=one");
  });

  it("falls back to the absolute still, and omits it when the server sent none", async () => {
    expect((await mint(respond({ relativeStillUrl: "", stillUrl: "http://abs/still?sig=1" }))).stillUri).toBe(
      "http://abs/still?sig=1",
    );
    expect("stillUri" in (await mint(respond({})))).toBe(false);
  });
});
