import { HttpResponse, http } from "msw";
import { describe, expect, it, vi } from "vitest";
import { server } from "@/test/msw/server";

const { mintChannelPlaySource } = vi.hoisted(() => ({ mintChannelPlaySource: vi.fn() }));

vi.mock("../channel-play-url", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../channel-play-url")>()),
  mintChannelPlaySource,
}));

import { preparedURL, warmableAssets, warmPreparedChannel } from "./channel-warmer";

describe("channel warmer", () => {
  it("selects the newest fragment and the map active across its discontinuity", () => {
    const manifest = `#EXTM3U
#EXT-X-MAP:URI="old/init.mp4?sig=x"
#EXTINF:2,
old/seg.m4s?sig=x
#EXT-X-DISCONTINUITY
#EXT-X-MAP:URI="new/init.mp4?sig=x"
#EXTINF:2,
new/seg.m4s?sig=x
`;
    expect(warmableAssets(manifest)).toEqual(["new/init.mp4?sig=x", "new/seg.m4s?sig=x"]);
  });

  it("adds prepared mode only to the master probe", () => {
    const got = new URL(preparedURL("/v1/playout/hls/ch-1/master.m3u8?sig=signed&plan=hevc8"));
    expect(got.searchParams.get("mode")).toBe("prepared");
    expect(got.searchParams.get("sig")).toBe("signed");
    expect(got.searchParams.get("plan")).toBe("hevc8");
  });

  it("fetches prepared init and media bytes while preserving the normal signed URL", async () => {
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

    const result = await warmPreparedChannel("ch-2", new AbortController().signal);

    expect(result).toEqual(
      expect.objectContaining({
        url: "/v1/playout/hls/ch-2/master.m3u8?sig=signed",
        warmed: true,
      }),
    );
    expect(requests).toHaveLength(3);
    expect(requests[0]?.searchParams.get("mode")).toBe("prepared");
    expect(requests.slice(1).every((request) => !request.searchParams.has("mode"))).toBe(true);
  });

  it("keeps the signed URL but fetches no assets on a prepared miss", async () => {
    mintChannelPlaySource.mockResolvedValue({
      url: "/v1/playout/hls/ch-3/master.m3u8?sig=signed",
      expiresAt: Date.now() + 60_000,
    });
    const requests: URL[] = [];
    server.use(
      http.get("*/v1/playout/hls/ch-3/master.m3u8", ({ request }) => {
        requests.push(new URL(request.url));
        return new HttpResponse(null, { status: 204 });
      }),
    );

    await expect(warmPreparedChannel("ch-3", new AbortController().signal)).resolves.toEqual(
      expect.objectContaining({ warmed: false }),
    );
    expect(requests).toHaveLength(1);
  });
});
