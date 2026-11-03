import * as channelsApi from "@loomarr/api/endpoints/channels";
import { toProblem } from "@loomarr/api/mutator";
import { unwrap } from "@loomarr/api/unwrap";
import {
  type DevicePlaybackProfile,
  type PlayerChannel,
  type PlayerSource,
  type PlayerSourcePort,
  type WarmedSource,
  warmSource,
} from "@loomarr/player";
import { markAdjacentWarm } from "../tuner-timing";

// The browser's source port: how the shared player controller mints and warms a channel's signed
// stream address. The stream authenticates by a short-lived signed URL, not the device token (§11), so
// the source comes from POST /v1/channels/{id}/play-url, minted per session; the session cookie never
// reaches the stream.

// qualityHint caps what the server offers; the player still adapts among those renditions.
const qualityHint = (): "auto" | "720" | "480" => {
  const conn = (navigator as Navigator & { connection?: { saveData?: boolean; effectiveType?: string } })
    .connection;
  if (conn?.saveData) return "480";
  if (conn?.effectiveType === "2g" || conn?.effectiveType === "slow-2g") return "480";
  if (conn?.effectiveType === "3g") return "720";
  const px = Math.max(window.screen.width, window.screen.height) * (window.devicePixelRatio || 1);
  if (px <= 640) return "480";
  if (px <= 1280) return "720";
  return "auto";
};

// mint is shared by the tuned channel and the neighbour warmer, so a warmed neighbour's exact signed
// URL can be reused for the real tune. A fresh signature would produce different asset cache keys
// even though the publication bytes are identical.
const mint = async (
  channel: PlayerChannel,
  profile: DevicePlaybackProfile,
  signal: AbortSignal,
): Promise<PlayerSource> => {
  const request = async () =>
    unwrap(await channelsApi.channelPlayUrl(channel.id, profile, { quality: qualityHint() }, { signal }));
  let body: Awaited<ReturnType<typeof request>>;
  try {
    body = await request();
  } catch (error) {
    if (signal.aborted) throw error;
    const problem = toProblem(error);
    throw new Error(problem.detail ?? problem.title ?? "Couldn't start this channel.");
  }
  // The browser prefers the relative same-origin form, which avoids CORS and works when
  // server.public_url is unset.
  const uri = body?.relativeUrl || body?.url;
  if (!body || !uri) throw new Error("Couldn't get a stream for this channel.");
  const stillUri = body.relativeStillUrl || body.stillUrl;
  return {
    expiresAt: new Date(body.expiresAt).getTime(),
    ...(Number.isFinite(body.serverTimeMs) ? { serverTimeMs: body.serverTimeMs } : {}),
    ...(stillUri ? { stillUri } : {}),
    uri,
  };
};

// warm never creates a player, MediaSource, or decoder. The warm protocol (a bounded live warm, then
// init + newest fragment) is the shared player package's `warmSource`, so the browser and the TV warm
// identically; this adds only the browser's transport (same-origin credentials, HTTP cache for the
// immutable assets) and keeps the exact normal signed URL for the real tune. Capacity and errors
// remain harmless speculative misses.
const warm = async (
  channel: PlayerChannel,
  profile: DevicePlaybackProfile,
  signal: AbortSignal,
): Promise<WarmedSource | undefined> => {
  const source = await mint(channel, profile, signal);
  const { warmed } = await warmSource(new URL(source.uri, window.location.href).toString(), (url, kind) =>
    fetch(url, {
      cache: kind === "playlist" ? "no-store" : "force-cache",
      credentials: "same-origin",
      signal,
    }),
  );
  if (source.stillUri) {
    // The server decodes a cold channel's still on demand. Asking now puts that decode and the
    // bytes behind us, so a surf to this neighbour paints its picture from the browser cache.
    await fetch(source.stillUri, { credentials: "same-origin", signal })
      .then((response) => response.arrayBuffer())
      .catch(() => undefined);
  }
  if (warmed) markAdjacentWarm(channel.id);
  return { ...source, warmed };
};

const channelSourcePort: PlayerSourcePort = { mint, warm };

export { channelSourcePort };
