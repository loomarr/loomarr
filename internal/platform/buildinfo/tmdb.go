package buildinfo

// tmdbKey is the TMDB API key Loomarr ships with (#2151 decision 51), stamped by the release
// build: -ldflags "-X github.com/loomarr/loomarr/internal/platform/buildinfo.tmdbKey=...". The
// repository is public, so the key lives only in the release secret, never in source; a source or
// dev build has none and needs the household's own key.
var tmdbKey = ""

// TMDBKey is the key this build ships with, "" when it was built without one. A household's own
// key, from Settings or TMDB_API_KEY, always wins over it.
func TMDBKey() string { return tmdbKey }
