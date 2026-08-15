import { afterEach, describe, expect, it, vi } from "vitest";

const { mintChannelPlaySource } = vi.hoisted(() => ({ mintChannelPlaySource: vi.fn() }));

vi.mock("../channel-play-url", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../channel-play-url")>()),
  mintChannelPlaySource,
}));

import { preparedURL, warmableAssets, warmPreparedChannel } from "./channel-warmer";

describe("channel warmer", () => {
  afterEach(() => vi.unstubAllGlobals());

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
    const fetch = vi
      .fn()
      .mockResolvedValueOnce({
        status: 200,
        ok: true,
        url: "http://localhost/v1/playout/hls/ch-2/master.m3u8?mode=prepared&sig=signed",
        text: async () =>
          '#EXTM3U\n#EXT-X-MAP:URI="pub/init.mp4?sig=signed"\n#EXTINF:2,\npub/seg.m4s?sig=signed\n',
      })
      .mockResolvedValue({ ok: true });
    vi.stubGlobal("fetch", fetch);

    const result = await warmPreparedChannel("ch-2", new AbortController().signal);

    expect(result).toEqual(
      expect.objectContaining({
        url: "/v1/playout/hls/ch-2/master.m3u8?sig=signed",
        warmed: true,
      }),
    );
    expect(fetch).toHaveBeenCalledTimes(3);
    expect(String(fetch.mock.calls[0]?.[0])).toContain("mode=prepared");
    expect(String(fetch.mock.calls[1]?.[0])).not.toContain("mode=prepared");
    expect(String(fetch.mock.calls[2]?.[0])).not.toContain("mode=prepared");
  });

  it("keeps the signed URL but fetches no assets on a prepared miss", async () => {
    mintChannelPlaySource.mockResolvedValue({
      url: "/v1/playout/hls/ch-3/master.m3u8?sig=signed",
      expiresAt: Date.now() + 60_000,
    });
    const fetch = vi.fn().mockResolvedValue({ status: 204, ok: true });
    vi.stubGlobal("fetch", fetch);

    await expect(warmPreparedChannel("ch-3", new AbortController().signal)).resolves.toEqual(
      expect.objectContaining({ warmed: false }),
    );
    expect(fetch).toHaveBeenCalledOnce();
  });
});
