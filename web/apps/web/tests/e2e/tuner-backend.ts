import { createReadStream, promises as fs } from "node:fs";
import { createServer, type IncomingMessage, type ServerResponse } from "node:http";
import { extname, join, normalize, resolve } from "node:path";
import { pathToFileURL } from "node:url";
import type { Page } from "@playwright/test";

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
  delayNextActiveManifest: (channelId: string, delayMs: number) => Promise<void>;
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
  page.on("request", (request) => {
    const url = new URL(request.url());
    const path = url.pathname;
    const master = path.match(/^\/v1\/playout\/hls\/(ch-\d+)\/master\.m3u8$/);
    if (master) {
      const id = master[1] ?? "";
      if (url.searchParams.get("mode") === "prepared") state.preparedProbes.push(id);
      else state.activeManifests.push(id);
    }
    const asset = path.match(/^\/v1\/playout\/hls\/(ch-\d+)\/(init\.mp4|segment\.m4s)$/);
    if (asset) state.assetRequests.push(`${asset[1]}/${asset[2]}`);
    const playURL = path.match(/^\/v1\/channels\/(ch-\d+)\/play-url$/);
    if (playURL && request.method() === "POST") state.playURLMints.push(playURL[1] ?? "");
  });

  return {
    delayNextActiveManifest: async (id, delayMs) => {
      const response = await fetch(`${TUNER_ORIGIN}/__tuner/delay`, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ channelId: id, delayMs }),
      });
      if (!response.ok) throw new Error(`fixture delay control failed: ${response.status}`);
    },
    state,
  };
};

const PORT = 6009;
const TUNER_ORIGIN = `http://127.0.0.1:${PORT}`;

const send = (
  response: ServerResponse,
  status: number,
  contentType: string,
  body: string | Buffer,
  headers: Record<string, string> = {},
) => {
  response.writeHead(status, { "content-type": contentType, ...headers });
  response.end(body);
};

const sendJSON = (response: ServerResponse, body: unknown, status = 200) =>
  send(response, status, "application/json", JSON.stringify(body));

const readJSON = async (request: IncomingMessage): Promise<Record<string, unknown>> => {
  const chunks: Buffer[] = [];
  for await (const chunk of request) chunks.push(Buffer.from(chunk));
  return JSON.parse(Buffer.concat(chunks).toString("utf8")) as Record<string, unknown>;
};

const contentType = (path: string): string =>
  ({
    ".css": "text/css",
    ".html": "text/html",
    ".js": "text/javascript",
    ".json": "application/json",
    ".woff2": "font/woff2",
  })[extname(path)] ?? "application/octet-stream";

const startTunerServer = () => {
  const activeManifestDelays = new Map<string, number>();
  const now = Date.now();
  const staticRoot = resolve(process.cwd(), "../../../internal/web/dist");
  const server = createServer(async (request, response) => {
    try {
      const url = new URL(request.url ?? "/", TUNER_ORIGIN);
      const path = url.pathname;

      if (path === "/__tuner/delay" && request.method === "POST") {
        const body = await readJSON(request);
        activeManifestDelays.set(String(body.channelId), Number(body.delayMs));
        return sendJSON(response, {});
      }
      if (path === "/v1/events") return send(response, 204, "text/plain", "");

      const master = path.match(/^\/v1\/playout\/hls\/(ch-\d+)\/master\.m3u8$/);
      if (master) {
        const id = master[1] ?? "";
        if (url.searchParams.get("mode") !== "prepared") {
          const delay = activeManifestDelays.get(id);
          if (delay) {
            activeManifestDelays.delete(id);
            await new Promise((resolve) => setTimeout(resolve, delay));
          }
        }
        return send(
          response,
          200,
          "application/vnd.apple.mpegurl",
          manifest(url.searchParams.get("sig") ?? id),
          { "cache-control": "no-store" },
        );
      }

      const asset = path.match(/^\/v1\/playout\/hls\/(ch-\d+)\/(init\.mp4|segment\.m4s)$/);
      if (asset) {
        return send(
          response,
          200,
          "video/mp4",
          Buffer.from(asset[2] === "init.mp4" ? INIT_MP4 : MEDIA_FRAGMENT, "base64"),
          { "cache-control": "private, max-age=31536000, immutable" },
        );
      }

      if (path === "/v1/auth/me") return sendJSON(response, ADMIN);
      if (path === "/v1/setup/state") return sendJSON(response, { bootstrapped: true });
      if (path === "/v1/settings") return sendJSON(response, { features: {}, settings: [] });
      if (path === "/v1/channels") return sendJSON(response, { channels });
      if (path === "/v1/channels/now-next") {
        return sendJSON(response, {
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
      if (playURL && request.method === "POST") {
        const id = playURL[1] ?? "";
        return sendJSON(response, {
          url: "",
          relativeUrl: `/v1/playout/hls/${id}/master.m3u8?sig=${id}`,
          expiresAt: new Date(Date.now() + 60 * 60_000).toISOString(),
        });
      }
      if (/^\/v1\/channels\/ch-\d+\/timeline$/.test(path)) return sendJSON(response, { airings: [] });
      if (/^\/v1\/channels\/ch-\d+\/tracks$/.test(path)) {
        return sendJSON(response, { audio: [], subtitles: [] });
      }
      const detail = path.match(/^\/v1\/channels\/(ch-\d+)$/);
      if (detail) {
        const found = channels.find((channel) => channel.id === detail[1]);
        return found ? sendJSON(response, found) : sendJSON(response, { title: "Not found" }, 404);
      }
      if (path.startsWith("/v1/")) return sendJSON(response, {});

      const relative = normalize(path).replace(/^\/+/, "");
      const candidate = join(staticRoot, relative || "index.html");
      const file = candidate.startsWith(staticRoot) ? candidate : join(staticRoot, "index.html");
      const target = await fs
        .stat(file)
        .then((info) => (info.isFile() ? file : join(staticRoot, "index.html")))
        .catch(() => join(staticRoot, "index.html"));
      response.writeHead(200, { "content-type": contentType(target), "cache-control": "no-store" });
      createReadStream(target).pipe(response);
    } catch (error) {
      sendJSON(response, { error: error instanceof Error ? error.message : String(error) }, 500);
    }
  });
  server.listen(PORT, "127.0.0.1");
};

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) startTunerServer();

export type { TunerBackend };
export { channelId, installTunerBackend };
