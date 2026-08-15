import { beforeEach, describe, expect, it, vi } from "vitest";

const { channelPlayUrl, unwrap, deviceProfile } = vi.hoisted(() => ({
  channelPlayUrl: vi.fn(),
  unwrap: vi.fn((response: unknown) => response),
  deviceProfile: vi.fn(() => ({ video: [], audio: [], video10bit: false })),
}));

vi.mock("@loomarr/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@loomarr/api")>()),
  channelsApi: { channelPlayUrl },
  unwrap,
}));
vi.mock("../use-hls-player/device-profile", () => ({ deviceProfile }));

import { mintChannelPlaySource } from "./channel-play-url";

describe("channel play URL", () => {
  beforeEach(() => {
    channelPlayUrl.mockReset();
    unwrap.mockClear();
  });

  it("mints with the device profile and prefers the browser's same-origin URL", async () => {
    channelPlayUrl.mockResolvedValue({
      relativeUrl: "/v1/playout/hls/ch-2/master.m3u8?sig=signed",
      url: "https://loomarr.example/v1/playout/hls/ch-2/master.m3u8?sig=signed",
      expiresAt: "2030-01-01T00:00:00Z",
    });
    const controller = new AbortController();

    await expect(mintChannelPlaySource("ch-2", controller.signal)).resolves.toEqual({
      url: "/v1/playout/hls/ch-2/master.m3u8?sig=signed",
      expiresAt: new Date("2030-01-01T00:00:00Z").getTime(),
    });
    expect(channelPlayUrl).toHaveBeenCalledWith(
      "ch-2",
      { video: [], audio: [], video10bit: false },
      { quality: expect.stringMatching(/^(auto|720|480)$/) },
      { signal: controller.signal },
    );
  });
});
