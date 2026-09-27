import { warmableAssets } from "../warmable-assets";

/** Playlists must be fresh; assets are immutable per signed URL, so a transport may cache them. */
type WarmGet = (url: string, kind: "playlist" | "asset") => Promise<Response>;

/**
 * Warms one channel's stream on the server under an already-minted, absolute signed URL and reports whether
 * every step landed. `get` is the caller's own transport (native authenticated fetch, or the
 * browser's with its cache policy), so this owns the protocol and never the network stack:
 *   1. `mode=warm` makes the server start the channel's packager under speculative admission (it
 *      never reclaims another channel's session, 503 when full) and holds the response until the
 *      first segment is listed;
 *   2. the init map and newest fragment are fetched and drained so they are hot server-side.
 * A warm is only ever for a neighbour of the channel being watched, and the packager stops after
 * its grace if the viewer never arrives (#1512 G1: no encoder runs while nobody is watching).
 * Every body is drained: an unread response can hold its connection and stall the real tune. The
 * signed URL the caller keeps for the real tune is untouched, so its asset URLs match what was warmed.
 */
const warmSource = async (uri: string, get: WarmGet): Promise<boolean> => {
  const warmUrl = new URL(uri);
  warmUrl.searchParams.set("mode", "warm");
  const manifestUrl = warmUrl.toString();
  const response = await get(manifestUrl, "playlist");
  if (!response.ok) {
    await response.text();
    return false;
  }
  const assets = warmableAssets(await response.text());
  const fetched = await Promise.all(
    assets.map(async (asset) => {
      const result = await get(new URL(asset, response.url || manifestUrl).toString(), "asset");
      await result.arrayBuffer();
      return result.ok;
    }),
  );
  return assets.length > 0 && fetched.every(Boolean);
};

export { warmSource };
