import { createReadStream, promises as fs } from "node:fs";
import { createServer, type IncomingMessage, type ServerResponse } from "node:http";
import { extname, join, normalize, resolve } from "node:path";
import { pathToFileURL } from "node:url";
import type { Page } from "@playwright/test";

const ADMIN = { id: "u1", name: "Ada", role: "admin", autoApprove: true, disabled: false, quota: 0 };

// Moving 160x90 H.264 CMAF keeps decoded frames observable. Channel 1 uses a one-second VOD so
// initial playback reaches ended and certifies disposal of an ended MediaSource. Replacement
// Channels use one genuine four-second segment in an OPEN live manifest, matching PreparedOrigin's
// deliberate no-ENDLIST contract. That is long enough to distinguish playing from a
// decoded-but-paused frame while the warmer and active player share the same newest asset, as they
// do at a live edge.
// Keeping bytes inline leaves the origin deterministic while browsers exercise hls.js, MSE, and
// decoding. Both fragments use the shared init above and this source, changing only -hls_time 1/4:
//
// ffmpeg -f lavfi -i "color=c=0x1A222C:s=160x90:r=24:d=4,drawbox=x=mod(t*40\,140):y=35:w=20:h=20:color=0x00ff88:t=fill"
//   -an -c:v libx264 -pix_fmt yuv420p -g 24 -keyint_min 24 -sc_threshold 0 -f hls
//   -hls_time 4 -hls_segment_type fmp4 -hls_flags independent_segments -hls_playlist_type vod
//   -hls_fmp4_init_filename init.mp4 stream.m3u8
const INIT_MP4 =
  "AAAAHGZ0eXBpc281AAACAGlzbzVpc282bXA0MQAAAx5tb292AAAAbG12aGQAAAAAAAAAAAAAAAAAAAPoAAAAAAABAAABAAAAAAAAAAAAAAAAAQAAAAAAAAAAAAAAAAAAAAEAAAAAAAAAAAAAAAAAAEAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAACAAACIXRyYWsAAABcdGtoZAAAAAMAAAAAAAAAAAAAAAEAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAQAAAAAAAAAAAAAAAAAAAAEAAAAAAAAAAAAAAAAAAEAAAAAAoAAAAFoAAAAAADBlZHRzAAAAKGVsc3QAAAAAAAAAAgAAAFP/////AAEAAAAAAAAAAAQAAAEAAAAAAY1tZGlhAAAAIG1kaGQAAAAAAAAAAAAAAAAAADAAAAAAAFXEAAAAAAAtaGRscgAAAAAAAAAAdmlkZQAAAAAAAAAAAAAAAFZpZGVvSGFuZGxlcgAAAAE4bWluZgAAABR2bWhkAAAAAQAAAAAAAAAAAAAAJGRpbmYAAAAcZHJlZgAAAAAAAAABAAAADHVybCAAAAABAAAA+HN0YmwAAACsc3RzZAAAAAAAAAABAAAAnGF2YzEAAAAAAAAAAQAAAAAAAAAAAAAAAAAAAAAAoABaAEgAAABIAAAAAAAAAAEUTGF2YzYzLjEuMTAxIGxpYngyNjQAAAAAAAAAAAAAAAAY//8AAAA2YXZjQwFkAAr/4QAZZ2QACqzZQo35MBEAAAMAAQAAAwAwDxIllgEABmjr48siwP34+AAAAAAQcGFzcAAAAAEAAAABAAAAEHN0dHMAAAAAAAAAAAAAABBzdHNjAAAAAAAAAAAAAAAUc3RzegAAAAAAAAAAAAAAAAAAABBzdGNvAAAAAAAAAAAAAAAobXZleAAAACB0cmV4AAAAAAAAAAEAAAABAAAAAAAAAAAAAAAAAAAAYXVkdGEAAABZbWV0YQAAAAAAAAAhaGRscgAAAAAAAAAAbWRpcmFwcGwAAAAAAAAAAAAAAAAsaWxzdAAAACSpdG9vAAAAHGRhdGEAAAABAAAAAExhdmY2My4xLjEwMQ==";

const SHORT_MEDIA_FRAGMENT =
  "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";

const LONG_MEDIA_FRAGMENT =
  "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";

const manifest = (sig: string, duration: 1 | 4) => `#EXTM3U
#EXT-X-VERSION:7
#EXT-X-TARGETDURATION:${duration}
#EXT-X-MEDIA-SEQUENCE:0
${duration === 1 ? "#EXT-X-PLAYLIST-TYPE:VOD\n" : ""}#EXT-X-INDEPENDENT-SEGMENTS
#EXT-X-MAP:URI="init.mp4?sig=${sig}"
#EXTINF:${duration}.000000,
segment.m4s?sig=${sig}
${duration === 1 ? "#EXT-X-ENDLIST\n" : ""}
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
    assetCompletions: string[];
    assetRequests: string[];
    playURLMints: string[];
    preparedProbes: string[];
  };
}

const installTunerBackend = async (page: Page): Promise<TunerBackend> => {
  const state = {
    activeManifests: [] as string[],
    assetCompletions: [] as string[],
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
  page.on("requestfinished", (request) => {
    const asset = new URL(request.url()).pathname.match(
      /^\/v1\/playout\/hls\/(ch-\d+)\/(init\.mp4|segment\.m4s)$/,
    );
    if (asset) state.assetCompletions.push(`${asset[1]}/${asset[2]}`);
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
      if (path === "/__tuner/calibration") {
        return send(
          response,
          200,
          "text/html",
          "<!doctype html><html><body><main>Media calibration</main></body></html>",
          { "cache-control": "no-store" },
        );
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
          manifest(url.searchParams.get("sig") ?? id, id === channelId(1) ? 1 : 4),
          { "cache-control": "no-store" },
        );
      }

      const asset = path.match(/^\/v1\/playout\/hls\/(ch-\d+)\/(init\.mp4|segment\.m4s)$/);
      if (asset) {
        const bytes =
          asset[2] === "init.mp4"
            ? INIT_MP4
            : asset[1] === channelId(1)
              ? SHORT_MEDIA_FRAGMENT
              : LONG_MEDIA_FRAGMENT;
        return send(response, 200, "video/mp4", Buffer.from(bytes, "base64"), {
          "cache-control": "private, max-age=31536000, immutable",
        });
      }

      if (path === "/v1/auth/me") return sendJSON(response, ADMIN);
      if (path === "/v1/setup/state") return sendJSON(response, { bootstrapped: true });
      if (path === "/v1/diagnostics/health") {
        return sendJSON(response, {
          generationId: "tuner-e2e-generation",
          generation: 1,
          version: "v0.9.3",
          processStartedAt: now - 60_000,
          generationStartedAt: now - 59_000,
          updatedAt: now,
          nextRefreshAt: now + 60_000,
          state: "healthy",
          checks: [],
        });
      }
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
export { channelId, installTunerBackend, manifest as tunerManifest };
