import { warmableAssets } from "../warmable-assets";

/** Playlists must be fresh; assets are immutable per signed URL, so a transport may cache them. */
type WarmGet = (url: string, kind: "playlist" | "asset") => Promise<Response>;

interface WarmResult {
  /** Every step landed. */
  warmed: boolean;
  /**
   * The absolute signed URL of the master's only variant, when the master names exactly one. A
   * native player may start from it and skip the master fetch (#1037). Absent when the master
   * offers a choice (a premium beside the baseline), which is the player's to make.
   */
  mediaUri?: string;
}

/** The URIs of a multivariant playlist's variants, in order; empty for a media playlist. */
const variantUris = (manifest: string): string[] => {
  const uris: string[] = [];
  let variantNext = false;
  for (const raw of manifest.replaceAll("\r\n", "\n").split("\n")) {
    const line = raw.trim();
    if (line.startsWith("#EXT-X-STREAM-INF:")) {
      variantNext = true;
    } else if (variantNext && line && !line.startsWith("#")) {
      uris.push(line);
      variantNext = false;
    }
  }
  return uris;
};

/**
 * Warms one channel's stream on the server under an already-minted, absolute signed URL and reports whether
 * every step landed. `get` is the caller's own transport (native authenticated fetch, or the
 * browser's with its cache policy), so this owns the protocol and never the network stack:
 *   1. `mode=warm` makes the server start the channel's packager under speculative admission (it
 *      never reclaims another channel's session, 503 when full) and holds the response until the
 *      first segment is listed;
 *   2. variant playlists are read with the same speculative hint; only already admitted renditions
 *      answer, without promoting them or recording a premium choice;
 *   3. each rendition's init map and newest fragment are fetched and drained so they are hot server-side.
 * A warm is only ever for a neighbour of the channel being watched, and the packager stops after
 * its grace if the viewer never arrives (#1512 G1: no encoder runs while nobody is watching).
 * Every body is drained: an unread response can hold its connection and stall the real tune. The
 * signed URL the caller keeps for the real tune is untouched, so its asset URLs match what was warmed.
 */
const warmSource = async (uri: string, get: WarmGet): Promise<WarmResult> => {
  const warmUrl = new URL(uri);
  warmUrl.searchParams.set("mode", "warm");
  const manifestUrl = warmUrl.toString();
  const response = await get(manifestUrl, "playlist");
  if (!response.ok) {
    await response.text();
    return { warmed: false };
  }
  const manifest = await response.text();
  const base = response.url || manifestUrl;
  const variants = variantUris(manifest);
  const warmMedia = async (body: string, mediaBase: string): Promise<boolean> => {
    // A playlist is never a media asset. Malformed/nested masters are a harmless warm miss.
    if (!body.trimStart().startsWith("#EXTM3U") || variantUris(body).length > 0) return false;
    const assets = warmableAssets(body);
    const fetched = await Promise.all(
      assets.map(async (asset) => {
        const result = await get(new URL(asset, mediaBase).toString(), "asset");
        await result.arrayBuffer();
        return result.ok;
      }),
    );
    return assets.length > 0 && fetched.every(Boolean);
  };
  const fetched =
    variants.length > 0
      ? await Promise.all(
          variants.map(async (variant) => {
            const url = new URL(variant, base);
            url.searchParams.set("mode", "warm");
            const media = await get(url.toString(), "playlist");
            const body = await media.text();
            return media.ok && warmMedia(body, media.url || url.toString());
          }),
        )
      : [await warmMedia(manifest, base)];
  const warmed = fetched.every(Boolean);
  const [only, ...others] = variants;
  return warmed && only !== undefined && others.length === 0
    ? { mediaUri: new URL(only, base).toString(), warmed }
    : { warmed };
};

export { type WarmResult, warmSource };
