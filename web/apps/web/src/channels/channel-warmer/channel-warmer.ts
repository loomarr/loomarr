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

// warmPreparedChannel never creates a player, MediaSource, decoder, or server-side live session.
// It mints one normal signed URL, adds the least-privilege mode only to the probe, and fetches the
// immutable bytes named by a prepared hit into the browser's HTTP cache.
const warmPreparedChannel = async (
  channelId: string,
  signal: AbortSignal,
): Promise<WarmedChannel | undefined> => {
  const source = await mintChannelPlaySource(channelId, signal);
  if (!source) return undefined;
  const response = await fetch(preparedURL(source.url), {
    signal,
    credentials: "same-origin",
    cache: "no-store",
  });
  if (response.status === 204) return { ...source, warmed: false };
  if (!response.ok) return { ...source, warmed: false };

  const manifestURL = response.url || preparedURL(source.url);
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

export { preparedURL, warmableAssets, warmPreparedChannel };
