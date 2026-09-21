import { createSocket } from "node:dgram";
import { createReadStream, existsSync } from "node:fs";
import { createServer } from "node:http";
import { extname, join, normalize } from "node:path";

const port = Number.parseInt(process.argv[2] ?? "18777", 10);
const mediaDirectory = process.argv[3];
const listenHost = process.argv[4] ?? "127.0.0.1";

if (!Number.isInteger(port) || port < 1 || port > 65_535 || !mediaDirectory) {
  console.error("usage: node tv-emulator-fixture-server.mjs PORT MEDIA_DIRECTORY [LISTEN_HOST]");
  process.exit(2);
}

const deviceToken = "journey-device-token";
const discoveryPort = 51_029;
const discoveryRequest = "LOOMARR_DISCOVER/1";
const state = {
  eventConnections: 0,
  eventDisconnects: 0,
  guideLoads: 0,
  pairPolls: 0,
  pairStarts: 0,
  playUrlChannels: [],
  revocations: 0,
  channelLoads: 0,
  guideEpochMs: null,
  schedulePhase: "programme-before",
};

const phaseServerTime = () => {
  const epoch = state.guideEpochMs ?? Date.now();
  if (state.schedulePhase === "filler") return epoch + 5 * 60_000 + 10_000;
  if (state.schedulePhase === "programme-after") return epoch + 10 * 60_000 + 10_000;
  return epoch;
};

const writeJson = (response, status, body, headers = {}) => {
  response.writeHead(status, { "Content-Type": "application/json", ...headers });
  response.end(JSON.stringify(body));
};

const channel = (id, name, number) => ({
  breakCount: 0,
  id,
  inAppPlayable: true,
  lineup: [],
  name,
  number,
  pendingCount: 0,
  policy: {},
  programCount: 1,
  revision: 1,
  slotCount: 1,
  status: "live",
  strategy: "sequential",
});

const channels = [
  channel("classic-animation", "Classic Animation", 77),
  channel("science-fiction", "Science Fiction", 120),
];

const guide = () => {
  const now = Date.now();
  state.guideEpochMs ??= now;
  const fromMs = now - 30 * 60_000;
  const toMs = now + 210 * 60_000;
  const airing = (scheduleBlockId, title, series, startMs = fromMs, stopMs = toMs) => ({
    description: `${title} emulator journey fixture`,
    episode: 2,
    kind: "program",
    scheduleBlockId,
    season: 7,
    series,
    startMs,
    stopMs,
    title,
  });
  const fillerStartMs = state.guideEpochMs + 5 * 60_000;
  const fillerStopMs = state.guideEpochMs + 10 * 60_000;
  return {
    channels: [
      {
        airings: [
          airing("before-break", "Before the break", "The Simpsons", fromMs, fillerStartMs),
          {
            kind: "filler",
            pod: {
              entries: [
                {
                  brand: "Acme Household",
                  durationMs: 5 * 60_000,
                  era: 1980,
                  hash: "raw-fixture-hash-must-not-render",
                  isFallbackCard: false,
                  kind: "commercial",
                  name: "Friendly Sponsor Spot",
                  path: "/raw/fixture/path-must-not-render.mp4",
                  quality: "1080p",
                },
              ],
              matchLevel: "exact",
              totalMs: 5 * 60_000,
            },
            scheduleBlockId: "commercial-break",
            startMs: fillerStartMs,
            stopMs: fillerStopMs,
            title: "",
          },
          airing("after-break", "After the break", "The Simpsons", fillerStopMs, toMs),
        ],
        channelId: "classic-animation",
        name: "Classic Animation",
        number: 77,
        pendingCount: 0,
        status: "live",
      },
      {
        airings: [airing("neutral-zone", "The Neutral Zone", "Star Trek")],
        channelId: "science-fiction",
        name: "Science Fiction",
        number: 120,
        pendingCount: 0,
        status: "live",
      },
    ],
    fromMs,
    timezone: "America/New_York",
    toMs,
  };
};

const authorized = (request) => request.headers.authorization === `Bearer ${deviceToken}`;

const serveMedia = (pathname, response) => {
  const relative = pathname.slice("/journey/".length);
  const file = normalize(join(mediaDirectory, relative));
  if (!file.startsWith(`${normalize(mediaDirectory)}/`) || !existsSync(file)) return false;
  const contentType =
    extname(file) === ".m3u8"
      ? "application/vnd.apple.mpegurl"
      : extname(file) === ".ts"
        ? "video/mp2t"
        : "application/octet-stream";
  response.writeHead(200, { "Content-Type": contentType });
  createReadStream(file).pipe(response);
  return true;
};

const server = createServer((request, response) => {
  const url = new URL(request.url ?? "/", `http://${request.headers.host ?? "127.0.0.1"}`);
  const pathname = url.pathname;

  if (request.method === "GET" && pathname === "/__journey") {
    writeJson(response, 200, state);
    return;
  }
  const phase = pathname.match(/^\/__journey\/phase\/(programme-before|filler|programme-after)$/);
  if (request.method === "POST" && phase) {
    state.schedulePhase = phase[1];
    writeJson(response, 200, { schedulePhase: state.schedulePhase });
    return;
  }
  if (request.method === "GET" && pathname.startsWith("/journey/")) {
    if (!serveMedia(pathname, response)) writeJson(response, 404, { title: "Fixture media not found" });
    return;
  }
  if (request.method === "POST" && pathname === "/v1/auth/device/start") {
    state.pairStarts += 1;
    writeJson(
      response,
      200,
      {
        deviceCode: "emulator-device-code",
        expiresAt: new Date(Date.now() + 10 * 60_000).toISOString(),
        interval: 1,
        userCode: "2468",
      },
      { Date: new Date().toUTCString() },
    );
    return;
  }
  if (request.method === "POST" && pathname === "/v1/auth/device/poll") {
    state.pairPolls += 1;
    if (state.pairPolls < 4) {
      writeJson(response, 428, { title: "Approval pending" });
      return;
    }
    writeJson(response, 200, { deviceName: "Loomarr TV", token: deviceToken });
    return;
  }
  if (request.method === "GET" && pathname === "/v1/auth/devices") {
    if (!authorized(request)) {
      writeJson(response, 401, { title: "Unauthorized" });
      return;
    }
    writeJson(response, 200, {
      devices: [
        {
          createdAt: new Date().toISOString(),
          deviceName: "Loomarr TV",
          id: "emulator-device",
          lastSeenAt: new Date().toISOString(),
        },
      ],
    });
    return;
  }
  if (request.method === "DELETE" && pathname === "/v1/auth/device") {
    if (!authorized(request)) {
      writeJson(response, 401, { title: "Unauthorized" });
      return;
    }
    state.revocations += 1;
    response.writeHead(204);
    response.end();
    return;
  }
  if (!authorized(request)) {
    writeJson(response, 401, { title: "Unauthorized" });
    return;
  }
  if (request.method === "GET" && pathname === "/v1/channels") {
    state.channelLoads += 1;
    writeJson(response, 200, { channels });
    return;
  }
  if (request.method === "GET" && pathname === "/v1/guide") {
    state.guideLoads += 1;
    writeJson(response, 200, guide());
    return;
  }
  const playUrl = pathname.match(/^\/v1\/channels\/([^/]+)\/play-url$/);
  if (request.method === "POST" && playUrl) {
    const channelId = decodeURIComponent(playUrl[1]);
    if (!channels.some(({ id }) => id === channelId)) {
      writeJson(response, 404, { title: "Channel not found" });
      return;
    }
    state.playUrlChannels.push(channelId);
    const serverTime = phaseServerTime();
    writeJson(
      response,
      200,
      {
        expiresAt: new Date(serverTime + 60_000).toISOString(),
        relativeUrl: "/journey/media.m3u8",
        serverTimeMs: serverTime,
        url: "",
      },
      { Date: new Date(serverTime).toUTCString() },
    );
    return;
  }
  if (request.method === "GET" && pathname === "/v1/system/version") {
    writeJson(response, 200, { ready: true, version: "journey-fixture" });
    return;
  }
  if (request.method === "GET" && pathname === "/v1/events") {
    state.eventConnections += 1;
    response.writeHead(200, {
      "Cache-Control": "no-cache",
      Connection: "keep-alive",
      "Content-Type": "text/event-stream",
    });
    response.write(": emulator journey\n\n");
    request.on("close", () => {
      state.eventDisconnects += 1;
    });
    return;
  }
  writeJson(response, 404, { title: "Fixture route not found" });
});

const discovery = createSocket({ reuseAddr: true, type: "udp4" });
discovery.on("message", (message, sender) => {
  if (message.toString("utf8") !== discoveryRequest) return;
  const payload = Buffer.from(
    JSON.stringify({
      id: "emulator-acceptance",
      name: "Loomarr Emulator Acceptance",
      protocol: 1,
      url: `http://10.0.2.2:${port}`,
    }),
  );
  discovery.send(payload, sender.port, sender.address);
});
discovery.bind(discoveryPort, "0.0.0.0");

server.listen(port, listenHost, () => {
  console.log(`tv-emulator-fixture: listening on http://${listenHost}:${port}`);
});

for (const signal of ["SIGINT", "SIGTERM"]) {
  process.on(signal, () => {
    discovery.close();
    server.close(() => process.exit(0));
    server.closeAllConnections();
    setTimeout(() => process.exit(1), 1_000).unref();
  });
}
