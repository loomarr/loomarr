import { HttpResponse, http } from "msw";
import { describe, expect, it, vi } from "vitest";
import { server } from "@/test/msw/server";

const { mintChannelPlaySource } = vi.hoisted(() => ({ mintChannelPlaySource: vi.fn() }));

vi.mock("../channel-play-url", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../channel-play-url")>()),
  mintChannelPlaySource,
}));

import { warmChannel } from "./channel-warmer";

describe("channel warmer", () => {
  it("warms live, then fetches init and media bytes while preserving the normal signed URL", async () => {
    mintChannelPlaySource.mockResolvedValue({
      url: "/v1/playout/hls/ch-2/master.m3u8?sig=signed",
      expiresAt: Date.now() + 60_000,
    });
    const requests: URL[] = [];
    server.use(
      http.get("*/v1/playout/hls/ch-2/master.m3u8", ({ request }) => {
        requests.push(new URL(request.url));
        return new HttpResponse(
          '#EXTM3U\n#EXT-X-MAP:URI="pub/init.mp4?sig=signed"\n#EXTINF:2,\npub/seg.m4s?sig=signed\n',
        );
      }),
      http.get("*/v1/playout/hls/ch-2/pub/init.mp4", ({ request }) => {
        requests.push(new URL(request.url));
        return new HttpResponse(new Uint8Array([0]));
      }),
      http.get("*/v1/playout/hls/ch-2/pub/seg.m4s", ({ request }) => {
        requests.push(new URL(request.url));
        return new HttpResponse(new Uint8Array([0]));
      }),
    );

    const result = await warmChannel("ch-2", new AbortController().signal);

    expect(result).toEqual(
      expect.objectContaining({
        url: "/v1/playout/hls/ch-2/master.m3u8?sig=signed",
        warmed: true,
      }),
    );
    expect(requests).toHaveLength(3);
    expect(requests[0]?.searchParams.get("mode")).toBe("warm");
    expect(requests.slice(1).every((request) => !request.searchParams.has("mode"))).toBe(true);
  });

  it("does not report an asset warm until its response body has been consumed", async () => {
    mintChannelPlaySource.mockResolvedValue({
      url: "/v1/playout/hls/ch-2/master.m3u8?sig=signed",
      expiresAt: Date.now() + 60_000,
    });
    const initBytes = vi.fn().mockResolvedValue(new ArrayBuffer(1));
    const mediaBytes = vi.fn().mockResolvedValue(new ArrayBuffer(1));
    vi.spyOn(globalThis, "fetch")
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        url: "http://localhost/v1/playout/hls/ch-2/master.m3u8?sig=signed&mode=warm",
        text: vi
          .fn()
          .mockResolvedValue(
            '#EXTM3U\n#EXT-X-MAP:URI="init.mp4?sig=signed"\n#EXTINF:2,\nsegment.m4s?sig=signed\n',
          ),
      } as unknown as Response)
      .mockResolvedValueOnce({ ok: true, arrayBuffer: initBytes } as unknown as Response)
      .mockResolvedValueOnce({ ok: true, arrayBuffer: mediaBytes } as unknown as Response);

    await expect(warmChannel("ch-2", new AbortController().signal)).resolves.toMatchObject({
      warmed: true,
    });

    expect(initBytes).toHaveBeenCalledOnce();
    expect(mediaBytes).toHaveBeenCalledOnce();
  });

  it("prefetches the neighbour's still so a surf paints it from cache", async () => {
    mintChannelPlaySource.mockResolvedValue({
      url: "/v1/playout/hls/ch-4/master.m3u8?sig=signed",
      stillURL: "/v1/playout/still/ch-4?sig=signed",
      expiresAt: Date.now() + 60_000,
    });
    const stills: URL[] = [];
    server.use(
      http.get(
        "*/v1/playout/hls/ch-4/master.m3u8",
        () => new HttpResponse("#EXTM3U\n#EXTINF:1,\nseg-0.m4s\n"),
      ),
      http.get("*/v1/playout/hls/ch-4/seg-0.m4s", () => new HttpResponse(new Uint8Array([0]))),
      http.get("*/v1/playout/still/ch-4", ({ request }) => {
        stills.push(new URL(request.url));
        return new HttpResponse(new Uint8Array([0xff, 0xd8]), { headers: { "Content-Type": "image/jpeg" } });
      }),
    );

    await expect(warmChannel("ch-4", new AbortController().signal)).resolves.toMatchObject({
      stillURL: "/v1/playout/still/ch-4?sig=signed",
      warmed: true,
    });
    expect(stills.map((url) => url.searchParams.get("sig"))).toEqual(["signed"]);
  });
});
