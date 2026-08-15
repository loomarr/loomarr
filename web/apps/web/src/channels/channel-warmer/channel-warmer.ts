import { mintChannelPlaySource } from "../channel-play-url";
import type { WarmedChannel } from "./channel-warmer.type";

const preparedURL = (raw: string): string => {
  const url = new URL(raw, window.location.href);
  url.searchParams.set("mode", "prepared");
  return url.toString();
};

// warmableAssets selects the init map and newest media fragment. It tracks the active map across
// discontinuities, so a manifest spanning two programmes warms the init that belongs to the
// fragment the player will join, not merely the first map in the file.
const warmableAssets = (manifest: string): string[] => {
  let currentMap: string | undefined;
  let targetMap: string | undefined;
  let targetMedia: string | undefined;
  for (const raw of manifest.replaceAll("\r\n", "\n").split("\n")) {
    const line = raw.trim();
    if (line.startsWith("#EXT-X-MAP:")) {
      currentMap = line.match(/URI="([^"]+)"/)?.[1];
    } else if (line && !line.startsWith("#")) {
      targetMap = currentMap;
      targetMedia = line;
    }
  }
  return [...new Set([targetMap, targetMedia].filter((value): value is string => Boolean(value)))];
};

// warmChannel never creates a player, MediaSource, or decoder. It tries the durable prepared origin
// first; on a clean miss it fetches one normal HLS snapshot so the server's existing bounded live
// origin can establish the adjacent remux during the current channel's tune-in. The exact normal
// signed URL is retained for the real tune, and capacity/errors remain harmless speculative misses.
const warmChannel = async (channelId: string, signal: AbortSignal): Promise<WarmedChannel | undefined> => {
  const source = await mintChannelPlaySource(channelId, signal);
  if (!source) return undefined;
  let response = await fetch(preparedURL(source.url), {
    signal,
    credentials: "same-origin",
    cache: "no-store",
  });
  let manifestURL = response.url || preparedURL(source.url);
  if (response.status === 204) {
    response = await fetch(source.url, {
      signal,
      credentials: "same-origin",
      cache: "no-store",
    });
    manifestURL = response.url || new URL(source.url, window.location.href).toString();
  }
  if (!response.ok) return { ...source, warmed: false };

  const manifest = await response.text();
  const assets = warmableAssets(manifest);
  const fetched = await Promise.all(
    assets.map((asset) =>
      fetch(new URL(asset, manifestURL), {
        signal,
        credentials: "same-origin",
        cache: "force-cache",
      }).then((result) => result.ok),
    ),
  );
  return { ...source, warmed: assets.length > 0 && fetched.every(Boolean) };
};

export { preparedURL, warmableAssets, warmChannel };
