/**
 * Picks the init map and newest media fragment from an HLS media playlist. It tracks the active map
 * across discontinuities, so a manifest spanning two programmes yields the init that belongs to the
 * fragment a player will join, not merely the first map in the file.
 */
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

export { warmableAssets };
