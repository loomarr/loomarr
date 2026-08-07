// The URLs that serve a clip's own bytes: the still frame (V30/V17b), the animated hover preview
// and the clip itself (V39).
//
// ⚠ A clip's wire identity is its content HASH (V45a) — hex, no slashes, no special characters — so
// the URL is a plain `/v1/filler/{route}/{hash}`. This REPLACES the old path-based form, which had to
// per-segment-encode a slash-and-`?`-and-`#`-bearing path; the hash removes that entire hazard class
// (and the dev-proxy `%2F`-decoding 404 it caused). `encodeURIComponent` stays as belt-and-braces —
// a hash needs no escaping, but it costs nothing and defends against a non-hash value slipping in.
//
// ⚠ `ClipDTO.thumbnail` / `.preview` are deliberately NOT what these take. Those are the ASSET's own
// path and are useful only as a "has one been generated" flag; each route derives its own file from
// the clip, server-side. Pass `clip.hash`.

// clipAssetURL is the shared rule for all three routes.
//
// ⚠ A `data:` value is returned UNCHANGED, and this is what makes the clip components storyable.
// Stories must render offline and deterministically (frontend-design §5.2), so they cannot fetch
// `/v1/filler/…` — the visual suite runs against `storybook-static` with no server behind it.
// Passing an inline data URI as the clip's identity is how the gallery shows a card WITH a frame,
// matching how channel-icon-field's stories already supply posters.
//
// Deliberately narrow: only `data:`, never `http(s):`. Treating any absolute URL as pass-through
// would turn one into a resource loaded from an arbitrary origin — a beacon that leaks who is
// browsing the catalog and when.
const clipAssetURL = (route: string, clipHash: string): string =>
  clipHash.startsWith("data:") ? clipHash : `/v1/filler/${route}/${encodeURIComponent(clipHash)}`;

// The extracted still, rendered on every catalog card.
const clipThumbURL = (clipHash: string): string => clipAssetURL("thumb", clipHash);

// The animated hover preview (V39): a few seconds of silent motion, so a grid of stills can
// answer "is this actually the advert it says it is?" without opening anything.
//
// ⚠ Called `hover`, not `preview`, matching the route. "Preview" already means "the pod pool a
// channel would get" twice over in this API, and the server-side naming note explains why a third
// meaning was refused.
const clipHoverURL = (clipHash: string): string => clipAssetURL("hover", clipHash);

// The clip's own bytes, range-capable so a <video> can seek rather than downloading the whole
// file to play the last five seconds.
const clipMediaURL = (clipHash: string): string => clipAssetURL("media", clipHash);

export { clipHoverURL, clipMediaURL, clipThumbURL };
