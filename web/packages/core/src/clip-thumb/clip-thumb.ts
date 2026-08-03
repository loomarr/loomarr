// The URLs that serve a clip's own bytes: the still frame (V30/V17b), the animated hover preview
// and the clip itself (V39).
//
// A clip's identity is its PATH relative to FILLER_DIR, which contains slashes and may contain
// spaces, `#`, `?` or any other character a filesystem allows. That makes building these URLs by
// concatenation a bug waiting to happen: a clip called `back in 5?.mp4` would truncate at the `?`
// and request a different file, and one called `a#b.mp4` would drop everything after the hash
// before the request was even sent.
//
// So each SEGMENT is encoded and the separators are re-joined — not the whole path in one go,
// which would escape the slashes into `%2F` and stop the routes' `{path...}` wildcard matching.
//
// ⚠ `ClipDTO.thumbnail` / `.preview` are deliberately NOT what these take. Those fields are the
// ASSET's own path (`80s/toys/intro.jpg`) and are useful only as a "has one been generated" flag;
// each route derives its own extension from the clip path, so passing the asset path would ask
// for `intro.jpg.jpg`. Pass `clip.path`.

// clipAssetURL is the shared rule, factored out when the hover preview became the third route to
// need it. All three had identical encoding and pass-through requirements, and three copies of a
// security-relevant rule is three places for it to drift.
//
// ⚠ A `data:` value is returned UNCHANGED, and this is what makes the clip components storyable.
// Stories must render offline and deterministically (frontend-design §5.2), so they cannot fetch
// `/v1/filler/…` — the visual suite runs against `storybook-static` with no server behind it.
// Passing an inline data URI as the clip's path is how the gallery shows a card WITH a frame,
// matching how channel-icon-field's stories already supply posters.
//
// Deliberately narrow: only `data:`, never `http(s):`. A clip path is server-controlled, but
// treating any absolute URL as pass-through would turn one into a resource loaded from an
// arbitrary origin — a beacon that leaks who is browsing the catalog and when.
const clipAssetURL = (route: string, clipPath: string): string =>
  clipPath.startsWith("data:")
    ? clipPath
    : `/v1/filler/${route}/${clipPath.split("/").map(encodeURIComponent).join("/")}`;

// The extracted still, rendered on every catalog card.
const clipThumbURL = (clipPath: string): string => clipAssetURL("thumb", clipPath);

// The animated hover preview (V39): a few seconds of silent motion, so a grid of stills can
// answer "is this actually the advert it says it is?" without opening anything.
//
// ⚠ Called `hover`, not `preview`, matching the route. "Preview" already means "the pod pool a
// channel would get" twice over in this API, and the server-side naming note explains why a third
// meaning was refused.
const clipHoverURL = (clipPath: string): string => clipAssetURL("hover", clipPath);

// The clip's own bytes, range-capable so a <video> can seek rather than downloading the whole
// file to play the last five seconds.
const clipMediaURL = (clipPath: string): string => clipAssetURL("media", clipPath);

export { clipHoverURL, clipMediaURL, clipThumbURL };
