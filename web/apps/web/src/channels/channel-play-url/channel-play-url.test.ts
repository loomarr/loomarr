import { HttpResponse, http } from "msw";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { server } from "@/test/msw/server";

const { channelPlayUrl, unwrap } = vi.hoisted(() => ({
  channelPlayUrl: vi.fn(),
  unwrap: vi.fn((response: unknown) => response),
}));

vi.mock("@loomarr/api/endpoints/channels", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@loomarr/api/endpoints/channels")>()),
  channelPlayUrl,
}));
vi.mock("@loomarr/api/unwrap", () => ({ unwrap }));
vi.mock("@loomarr/api/mutator", () => ({
  toProblem: (error: unknown) => ({ detail: (error as Error)?.message, title: "Problem" }),
}));

import { channelSourcePort } from "./channel-play-url";

const channel = { id: "ch-2", inAppPlayable: true, name: "Two", number: 2 };

describe("channel source port", () => {
  beforeEach(() => {
    channelPlayUrl.mockReset();
    unwrap.mockReset().mockImplementation((response: unknown) => response);
  });

  describe("mint", () => {
    it("mints with a quality hint and prefers the browser's same-origin URL", async () => {
      channelPlayUrl.mockResolvedValue({
        expiresAt: "2030-01-01T00:00:00Z",
        relativeUrl: "/v1/playout/hls/ch-2/master.m3u8?sig=signed",
        serverTimeMs: 1_700_000_000_000,
        url: "https://loomarr.example/v1/playout/hls/ch-2/master.m3u8?sig=signed",
      });
      const controller = new AbortController();

      await expect(channelSourcePort.mint(channel, controller.signal)).resolves.toEqual({
        expiresAt: new Date("2030-01-01T00:00:00Z").getTime(),
        serverTimeMs: 1_700_000_000_000,
        uri: "/v1/playout/hls/ch-2/master.m3u8?sig=signed",
      });
      expect(channelPlayUrl).toHaveBeenCalledWith(
        "ch-2",
        { quality: expect.stringMatching(/^(auto|720|480)$/) },
        { signal: controller.signal },
      );
    });

    it("carries the channel's same-origin still so a tune can show it before video", async () => {
      channelPlayUrl.mockResolvedValue({
        expiresAt: "2030-01-01T00:00:00Z",
        relativeStillUrl: "/v1/playout/still/ch-2?sig=signed",
        relativeUrl: "/v1/playout/hls/ch-2/master.m3u8?sig=signed",
        stillUrl: "https://loomarr.example/v1/playout/still/ch-2?sig=signed",
      });

      await expect(channelSourcePort.mint(channel, new AbortController().signal)).resolves.toMatchObject({
        stillUri: "/v1/playout/still/ch-2?sig=signed",
      });
    });

    it("says why when the server returns no stream address", async () => {
      channelPlayUrl.mockResolvedValue({}); // neither relativeUrl nor url

      await expect(channelSourcePort.mint(channel, new AbortController().signal)).rejects.toThrow(
        /couldn't get a stream for this channel/i,
      );
    });

    it("reads the server's problem detail from a failed mint", async () => {
      channelPlayUrl.mockResolvedValue({});
      unwrap.mockImplementation(() => {
        throw new Error("network down");
      });

      await expect(channelSourcePort.mint(channel, new AbortController().signal)).rejects.toThrow(
        "network down",
      );
    });
  });

  describe("warm", () => {
    const warm = () => channelSourcePort.warm?.(channel, new AbortController().signal);

    it("warms live, then fetches init and media bytes while preserving the normal signed URL", async () => {
      channelPlayUrl.mockResolvedValue({
        expiresAt: new Date(Date.now() + 60_000).toISOString(),
        relativeUrl: "/v1/playout/hls/ch-2/master.m3u8?sig=signed",
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

      await expect(warm()).resolves.toMatchObject({
        uri: "/v1/playout/hls/ch-2/master.m3u8?sig=signed",
        warmed: true,
      });
      expect(requests).toHaveLength(3);
      expect(requests[0]?.searchParams.get("mode")).toBe("warm");
      expect(requests.slice(1).every((request) => !request.searchParams.has("mode"))).toBe(true);
    });

    it("does not report an asset warm until its response body has been consumed", async () => {
      channelPlayUrl.mockResolvedValue({
        expiresAt: new Date(Date.now() + 60_000).toISOString(),
        relativeUrl: "/v1/playout/hls/ch-2/master.m3u8?sig=signed",
      });
      const initBytes = vi.fn().mockResolvedValue(new ArrayBuffer(1));
      const mediaBytes = vi.fn().mockResolvedValue(new ArrayBuffer(1));
      vi.spyOn(globalThis, "fetch")
        .mockResolvedValueOnce({
          ok: true,
          status: 200,
          text: vi
            .fn()
            .mockResolvedValue(
              '#EXTM3U\n#EXT-X-MAP:URI="init.mp4?sig=signed"\n#EXTINF:2,\nsegment.m4s?sig=signed\n',
            ),
          url: "http://localhost/v1/playout/hls/ch-2/master.m3u8?sig=signed&mode=warm",
        } as unknown as Response)
        .mockResolvedValueOnce({ arrayBuffer: initBytes, ok: true } as unknown as Response)
        .mockResolvedValueOnce({ arrayBuffer: mediaBytes, ok: true } as unknown as Response);

      await expect(warm()).resolves.toMatchObject({ warmed: true });

      expect(initBytes).toHaveBeenCalledOnce();
      expect(mediaBytes).toHaveBeenCalledOnce();
    });

    it("prefetches the neighbour's still so a surf paints it from cache", async () => {
      channelPlayUrl.mockResolvedValue({
        expiresAt: new Date(Date.now() + 60_000).toISOString(),
        relativeStillUrl: "/v1/playout/still/ch-2?sig=signed",
        relativeUrl: "/v1/playout/hls/ch-2/master.m3u8?sig=signed",
      });
      const stills: URL[] = [];
      server.use(
        http.get(
          "*/v1/playout/hls/ch-2/master.m3u8",
          () => new HttpResponse("#EXTM3U\n#EXTINF:1,\nseg-0.m4s\n"),
        ),
        http.get("*/v1/playout/hls/ch-2/seg-0.m4s", () => new HttpResponse(new Uint8Array([0]))),
        http.get("*/v1/playout/still/ch-2", ({ request }) => {
          stills.push(new URL(request.url));
          return new HttpResponse(new Uint8Array([0xff, 0xd8]), {
            headers: { "Content-Type": "image/jpeg" },
          });
        }),
      );

      await expect(warm()).resolves.toMatchObject({
        stillUri: "/v1/playout/still/ch-2?sig=signed",
        warmed: true,
      });
      expect(stills.map((url) => url.searchParams.get("sig"))).toEqual(["signed"]);
    });
  });
});
