import type { Page, Route } from "@playwright/test";

const ADMIN = { id: "u1", name: "Ada", role: "admin", autoApprove: true, disabled: false, quota: 0 };

// One second of flat 160x90 H.264 as CMAF init + media bytes. Keeping the fixture inline lets the
// mocked HLS origin stay deterministic and avoids a second test server, while Chromium still feeds
// the bytes through production hls.js, MSE, its decoder, and requestVideoFrameCallback. Generated:
//
// ffmpeg -f lavfi -i "color=c=0x1A222C:s=160x90:r=24:d=4" -an -c:v libx264 -pix_fmt yuv420p
//   -g 24 -keyint_min 24 -sc_threshold 0 -f hls -hls_time 1 -hls_segment_type fmp4
//   -hls_flags independent_segments -hls_playlist_type vod -hls_fmp4_init_filename init.mp4 stream.m3u8
const INIT_MP4 =
  "AAAAHGZ0eXBpc281AAACAGlzbzVpc282bXA0MQAAAx5tb292AAAAbG12aGQAAAAAAAAAAAAAAAAAAAPoAAAAAAABAAABAAAAAAAAAAAAAAAAAQAAAAAAAAAAAAAAAAAAAAEAAAAAAAAAAAAAAAAAAEAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAACAAACIXRyYWsAAABcdGtoZAAAAAMAAAAAAAAAAAAAAAEAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAQAAAAAAAAAAAAAAAAAAAAEAAAAAAAAAAAAAAAAAAEAAAAAAoAAAAFoAAAAAADBlZHRzAAAAKGVsc3QAAAAAAAAAAgAAAFP/////AAEAAAAAAAAAAAQAAAEAAAAAAY1tZGlhAAAAIG1kaGQAAAAAAAAAAAAAAAAAADAAAAAAAFXEAAAAAAAtaGRscgAAAAAAAAAAdmlkZQAAAAAAAAAAAAAAAFZpZGVvSGFuZGxlcgAAAAE4bWluZgAAABR2bWhkAAAAAQAAAAAAAAAAAAAAJGRpbmYAAAAcZHJlZgAAAAAAAAABAAAADHVybCAAAAABAAAA+HN0YmwAAACsc3RzZAAAAAAAAAABAAAAnGF2YzEAAAAAAAAAAQAAAAAAAAAAAAAAAAAAAAAAoABaAEgAAABIAAAAAAAAAAEUTGF2YzYzLjEuMTAxIGxpYngyNjQAAAAAAAAAAAAAAAAY//8AAAA2YXZjQwFkAAr/4QAZZ2QACqzZQo35MBEAAAMAAQAAAwAwDxIllgEABmjr48siwP34+AAAAAAQcGFzcAAAAAEAAAABAAAAEHN0dHMAAAAAAAAAAAAAABBzdHNjAAAAAAAAAAAAAAAUc3RzegAAAAAAAAAAAAAAAAAAABBzdGNvAAAAAAAAAAAAAAAobXZleAAAACB0cmV4AAAAAAAAAAEAAAABAAAAAAAAAAAAAAAAAAAAYXVkdGEAAABZbWV0YQAAAAAAAAAhaGRscgAAAAAAAAAAbWRpcmFwcGwAAAAAAAAAAAAAAAAsaWxzdAAAACSpdG9vAAAAHGRhdGEAAAABAAAAAExhdmY2My4xLjEwMQ==";

const MEDIA_FRAGMENT =
  "AAAAGHN0eXBtc2RoAAAAAG1zZGhtc2l4AAAANHNpZHgBAAAAAAAAAQAAMAAAAAAAAAAEAAAAAAAAAAAAAAAAAQAABW4AADAAgAAAAAAAAShtb29mAAAAEG1maGQAAAAAAAAAAQAAARB0cmFmAAAAHHRmaGQAAgA4AAAAAQAAAgAAAALmAQEAAAAAABR0ZmR0AQAAAAAAAAAAAAAAAAAA2HRydW4AAAoFAAAAGAAAATACAAAAAAAC5gAABAAAAAAQAAAKAAAAAA0AAAQAAAAADAAAAAAAAAAMAAACAAAAABYAAAoAAAAADwAABAAAAAAMAAAAAAAAAAwAAAIAAAAAFgAACgAAAAAPAAAEAAAAAAwAAAAAAAAADAAAAgAAAAAWAAAKAAAAAA8AAAQAAAAADAAAAAAAAAAMAAACAAAAABYAAAoAAAAADwAABAAAAAAMAAAAAAAAAAwAAAIAAAAAFQAACAAAAAAOAAACAAAAAAwAAAIAAAAERm1kYXQAAAKsBgX//6jcRem95tlIt5Ys2CDZI+7veDI2NCAtIGNvcmUgMTY1IHIzMjIyIGIzNTYwNWEgLSBILjI2NC9NUEVHLTQgQVZDIGNvZGVjIC0gQ29weWxlZnQgMjAwMy0yMDI1IC0gaHR0cDovL3d3dy52aWRlb2xhbi5vcmcveDI2NC5odG1sIC0gb3B0aW9uczogY2FiYWM9MSByZWY9MyBkZWJsb2NrPTE6MDowIGFuYWx5c2U9MHgzOjB4MTEzIG1lPWhleCBzdWJtZT03IHBzeT0xIHBzeV9yZD0xLjAwOjAuMDAgbWl4ZWRfcmVmPTEgbWVfcmFuZ2U9MTYgY2hyb21hX21lPTEgdHJlbGxpcz0xIDh4OGRjdD0xIGNxbT0wIGRlYWR6b25lPTIxLDExIGZhc3RfcHNraXA9MSBjaHJvbWFfcXBfb2Zmc2V0PS0yIHRocmVhZHM9MyBsb29rYWhlYWRfdGhyZWFkcz0xIHNsaWNlZF90aHJlYWRzPTAgbnI9MCBkZWNpbWF0ZT0xIGludGVybGFjZWQ9MCBibHVyYXlfY29tcGF0PTAgY29uc3RyYWluZWRfaW50cmE9MCBiZnJhbWVzPTMgYl9weXJhbWlkPTIgYl9hZGFwdD0xIGJfYmlhcz0wIGRpcmVjdD0xIHdlaWdodGI9MSBvcGVuX2dvcD0wIHdlaWdodHA9MiBrZXlpbnQ9MjQga2V5aW50X21pbj0xMyBzY2VuZWN1dD0wIGludHJhX3JlZnJlc2g9MCByY19sb29rYWhlYWQ9MjQgcmM9Y3JmIG1idHJlZT0xIGNyZj0yMy4wIHFjb21wPTAuNjAgcXBtaW49MCBxcG1heD02OSBxcHN0ZXA9NCBpcF9yYXRpbz0xLjQwIGFxPTE6MS4wMACAAAAAMmWIhAA7//7jq/gU2AngbcRYlNGNPzxSXbPITNxywVQ/PquVu8GW2S0vyAMgABeQf/5BAAAADEGaJGxDv/6plgDmgAAAAAlBnkJ4hf8A84EAAAAIAZ5hdEK/AVMAAAAIAZ5jakK/AVMAAAASQZpoSahBaJlMCHf//qmWAOaBAAAAC0GehkURLC//APOBAAAACAGepXRCvwFTAAAACAGep2pCvwFTAAAAEkGarEmoQWyZTAh3//6plgDmgAAAAAtBnspFFSwv/wDzgQAAAAgBnul0Qr8BUwAAAAgBnutqQr8BUwAAABJBmvBJqEFsmUwId//+qZYA5oEAAAALQZ8ORRUsL/8A84EAAAAIAZ8tdEK/AVMAAAAIAZ8vakK/AVMAAAASQZs0SahBbJlMCHf//qmWAOaAAAAAC0GfUkUVLC//APOBAAAACAGfcXRCvwFTAAAACAGfc2pCvwFTAAAAEUGbd0moQWyZTAhX//44QBoxAAAACkGflUUVLCv/AVMAAAAIAZ+2akK/AVM=";

const manifest = (sig: string) => `#EXTM3U
#EXT-X-VERSION:7
#EXT-X-TARGETDURATION:1
#EXT-X-MEDIA-SEQUENCE:0
#EXT-X-PLAYLIST-TYPE:VOD
#EXT-X-INDEPENDENT-SEGMENTS
#EXT-X-MAP:URI="init.mp4?sig=${sig}"
#EXTINF:1.000000,
segment.m4s?sig=${sig}
#EXT-X-ENDLIST
`;

const json = (route: Route, body: unknown, status = 200) =>
  route.fulfill({ status, contentType: "application/json", body: JSON.stringify(body) });

const channelId = (number: number) => `ch-${String(number).padStart(3, "0")}`;

const channels = Array.from({ length: 100 }, (_, index) => {
  const number = index + 1;
  return {
    id: channelId(number),
    name: `Channel ${number}`,
    number,
    inAppPlayable: true,
    status: "live",
    strategy: "shuffle",
    lineup: [],
    policy: {},
    breakCount: 0,
    pendingCount: 0,
    programCount: 1,
    slotCount: 1,
    tunarrId: `tunarr-${number}`,
  };
});

interface TunerBackend {
  readonly state: {
    activeManifests: string[];
    assetRequests: string[];
    playURLMints: string[];
    preparedProbes: string[];
  };
}

const installTunerBackend = async (page: Page): Promise<TunerBackend> => {
  const state = {
    activeManifests: [] as string[],
    assetRequests: [] as string[],
    playURLMints: [] as string[],
    preparedProbes: [] as string[],
  };
  const now = Date.now();

  await page.route("**/v1/events**", (route) => route.abort());
  await page.route("**/v1/**", async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname;

    const master = path.match(/^\/v1\/playout\/hls\/(ch-\d+)\/master\.m3u8$/);
    if (master) {
      const id = master[1] ?? "";
      if (url.searchParams.get("mode") === "prepared") state.preparedProbes.push(id);
      else state.activeManifests.push(id);
      return route.fulfill({
        status: 200,
        contentType: "application/vnd.apple.mpegurl",
        headers: { "cache-control": "no-store" },
        body: manifest(url.searchParams.get("sig") ?? id),
      });
    }

    const asset = path.match(/^\/v1\/playout\/hls\/(ch-\d+)\/(init\.mp4|segment\.m4s)$/);
    if (asset) {
      state.assetRequests.push(`${asset[1]}/${asset[2]}`);
      return route.fulfill({
        status: 200,
        contentType: "video/mp4",
        headers: { "cache-control": "private, max-age=31536000, immutable" },
        body: Buffer.from(asset[2] === "init.mp4" ? INIT_MP4 : MEDIA_FRAGMENT, "base64"),
      });
    }

    if (path === "/v1/auth/me") return json(route, ADMIN);
    if (path === "/v1/setup/state") return json(route, { bootstrapped: true });
    if (path === "/v1/settings") return json(route, { features: {}, settings: [] });
    if (path === "/v1/channels") return json(route, { channels });
    if (path === "/v1/channels/now-next") {
      return json(route, {
        channels: channels.map((channel) => ({
          channelId: channel.id,
          now: {
            title: `Programme ${channel.number}`,
            gap: false,
            startMs: now - 10 * 60_000,
            stopMs: now + 50 * 60_000,
          },
        })),
      });
    }

    const playURL = path.match(/^\/v1\/channels\/(ch-\d+)\/play-url$/);
    if (playURL && request.method() === "POST") {
      const id = playURL[1] ?? "";
      state.playURLMints.push(id);
      return json(route, {
        url: "",
        relativeUrl: `/v1/playout/hls/${id}/master.m3u8?sig=${id}`,
        expiresAt: new Date(Date.now() + 60 * 60_000).toISOString(),
      });
    }

    const timeline = path.match(/^\/v1\/channels\/(ch-\d+)\/timeline$/);
    if (timeline) return json(route, { airings: [] });
    const tracks = path.match(/^\/v1\/channels\/(ch-\d+)\/tracks$/);
    if (tracks) return json(route, { audio: [], subtitles: [] });
    const detail = path.match(/^\/v1\/channels\/(ch-\d+)$/);
    if (detail) {
      const found = channels.find((channel) => channel.id === detail[1]);
      return found ? json(route, found) : json(route, { title: "Not found" }, 404);
    }

    return json(route, {});
  });

  return { state };
};

export type { TunerBackend };
export { channelId, installTunerBackend };
