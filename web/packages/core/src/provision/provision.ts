// The provisioning-key derivation, mirroring `provision.Title.Key()` in Go
// (internal/provision/title.go:52). ONE title has ONE key regardless of which endpoint
// described it, which is what lets a search result, a proposal item, and a stored lineup entry
// all refer to the same thing.
//
// Extracted here after a FIFTH copy was about to be written. Duplication is a real hazard for
// this particular function rather than a tidiness complaint: the approval edit drops titles **by
// key**, so a derivation that disagrees with Go's by one character does not error — the key
// simply matches nothing and the title the admin removed is acquired anyway. Silent, and on the
// path that spends money.
//
// ⚠ THREE NEAR-COPIES REMAIN, deliberately not converted here (that would be behaviour change
// inside a feature PR). Each differs in a way worth reading before you fold it in:
//   - `channel-lineup-editor.tsx keyOf` — falls back to a `name:<lower>:<year>` key, then
//     FILTERS those out. The fallback exists only so the filter has something to test.
//   - `refine-review.tsx keyOf` — same `name:` fallback, but it KEEPS those rows: a diff needs
//     an identity for every row, including one that could never be provisioned.
//   - `channel-series-scope.tsx seriesKeyOf` — series-only, so a movie yields "" rather than a
//     movie key.
// A shared helper covering all three wants an options argument, and the honest version of that
// is its own change.
//
// Precedence is Go's, exactly:
//   1. series + tvdbId  → "series:tvdb:<id>"   (series prefer TVDB)
//   2. any + tmdbId     → "<mediaType>:tmdb:<id>"
//   3. otherwise        → no usable key
//
// Go returns an error for case 3; here it is "" so callers can filter. A title with no usable
// id cannot be provisioned at all, and `provision.ParseKey` rejects any other shape — so an
// empty result must be treated as "not addable", never lowered to a `name:`-style fallback.

/** The identifying fields of a title, as carried by search candidates and proposal items. */
interface KeyableTitle {
  mediaType?: string;
  tmdbId?: number;
  tvdbId?: number;
}

/**
 * Derives the provisioning key for a title, or "" when it has no usable id.
 * Mirrors Go's `provision.Title.Key()`.
 */
const provisionKey = (t: KeyableTitle): string => {
  if (!t.mediaType) return "";
  if (t.mediaType === "series" && t.tvdbId) return `series:tvdb:${t.tvdbId}`;
  if (t.tmdbId) return `${t.mediaType}:tmdb:${t.tmdbId}`;
  return "";
};

export type { KeyableTitle };
export { provisionKey };
