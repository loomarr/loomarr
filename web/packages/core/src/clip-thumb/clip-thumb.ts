// The URL that serves a clip's thumbnail frame (V30/V17b).
//
// A clip's identity is its PATH relative to FILLER_DIR, which contains slashes and may contain
// spaces, `#`, `?` or any other character a filesystem allows. That makes building this URL by
// concatenation a bug waiting to happen: a clip called `back in 5?.mp4` would truncate at the
// `?` and request a different file, and one called `a#b.mp4` would drop everything after the
// hash before the request was even sent.
//
// So each SEGMENT is encoded and the separators are re-joined — not the whole path in one go,
// which would escape the slashes into `%2F` and stop the route's `{path...}` wildcard matching.
//
// ⚠ `ClipDTO.thumbnail` is deliberately NOT what this takes. That field is the thumbnail's own
// path (`80s/toys/intro.jpg`) and is useful only as a "has one been extracted" flag; the route
// derives the .jpg name itself from the clip path, so passing the thumbnail path would ask for
// `intro.jpg.jpg`. Pass `clip.path`.
// ⚠ A `data:` value is returned UNCHANGED, and this is what makes ClipCard storyable.
// Stories must render offline and deterministically (frontend-design §5.2), so they cannot
// fetch `/v1/filler/thumb/…` — the visual suite runs against `storybook-static` with no server
// behind it. Passing an inline data URI as the clip's path is how the gallery shows a card
// WITH a frame, matching how channel-icon-field's stories already supply posters.
//
// Deliberately narrow: only `data:`, never `http(s):`. A clip path is server-controlled, but
// treating any absolute URL as pass-through would turn one into an image loaded from an
// arbitrary origin — a beacon that leaks who is browsing the catalog and when.
const clipThumbURL = (clipPath: string): string =>
  clipPath.startsWith("data:")
    ? clipPath
    : `/v1/filler/thumb/${clipPath.split("/").map(encodeURIComponent).join("/")}`;

export { clipThumbURL };
