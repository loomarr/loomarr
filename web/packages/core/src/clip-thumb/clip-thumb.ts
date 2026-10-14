// The URL that serves a clip's own bytes (V39).
//
// ⚠ A clip's wire identity is its content HASH (V45a) — hex, no slashes, no special characters — so
// the URL is a plain `/v1/filler/{route}/{hash}`. This REPLACES the old path-based form, which had to
// per-segment-encode a slash-and-`?`-and-`#`-bearing path; the hash removes that entire hazard class
// (and the dev-proxy `%2F`-decoding 404 it caused). `encodeURIComponent` stays as belt-and-braces —
// a hash needs no escaping, but it costs nothing and defends against a non-hash value slipping in.
//
// ⚠ **`clipThumbURL` and `clipHoverURL` were RETIRED in V52 phase 8, with the routes behind them.**
// A clip's still and hover loop are image-service images now (§22): the DTO carries `thumbImage` /
// `hoverImage` records and the client renders them through the <Image> primitive, which brings a
// width ladder, a ThumbHash placeholder and honest immutable caching — none of which a bare
// `/v1/filler/thumb/{hash}` could provide. `clipMediaURL` stays: a clip's own bytes are not an
// image, and the image service has no opinion about them.

// clipAssetURL is the shared rule, kept as its own function because the pass-through below is a
// rule about VALUES rather than about any one route.
//
// ⚠ A `data:` value is returned UNCHANGED, and this is what makes the clip components storyable.
// Stories must render offline and deterministically (frontend-design §5.2), so they cannot fetch
// `/v1/filler/…` — the visual suite runs against `storybook-static` with no server behind it.
//
// Deliberately narrow: only `data:`, never `http(s):`. Treating any absolute URL as pass-through
// would turn one into a resource loaded from an arbitrary origin — a beacon that leaks who is
// browsing the catalog and when. That reasoning is what §22 generalised across the whole product.
const clipAssetURL = (route: string, clipHash: string): string =>
  clipHash.startsWith("data:") ? clipHash : `/v1/filler/${route}/${encodeURIComponent(clipHash)}`;

// The clip's own bytes, range-capable so a <video> can seek rather than downloading the whole
// file to play the last five seconds.
const clipMediaURL = (clipHash: string): string => clipAssetURL("media", clipHash);

export { clipMediaURL };
