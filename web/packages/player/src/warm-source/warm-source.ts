import { warmableAssets } from "../warmable-assets";

/** Playlists must be fresh; assets are immutable per signed URL, so a transport may cache them. */
type WarmGet = (url: string, kind: "playlist" | "asset") => Promise<Response>;

/**
 * Warms one channel's stream on the server under an already-minted, absolute signed URL and reports whether
 * every step landed. `get` is the caller's own transport (native authenticated fetch, or the
 * browser's with its cache policy), so this owns the protocol and never the network stack:
 *   1. `mode=prepared` asks for the durable prepared origin (204 = nothing prepared);
 *   2. on a clean miss, `mode=warm` makes the server start the channel's live remux under
 *      speculative admission (it never reclaims another channel's session, 503 when full) and holds
 *      the response until the first segment is listed;
 *   3. the init map and newest fragment are fetched and drained so they are hot server-side.
 * Every body is drained: an unread response can hold its connection and stall the real tune. The
 * signed URL the caller keeps for the real tune is untouched, so its asset URLs match what was warmed.
 */
const warmSource = async (uri: string, get: WarmGet): Promise<boolean> => {
  const withMode = (mode: string) => {
    const url = new URL(uri);
    url.searchParams.set("mode", mode);
    return url.toString();
  };

  let manifestUrl = withMode("prepared");
  let response = await get(manifestUrl, "playlist");
  if (response.status === 204) {
    manifestUrl = withMode("warm");
    response = await get(manifestUrl, "playlist");
  }
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
