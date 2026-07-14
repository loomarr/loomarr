package tmdb

import "strings"

// TMDB genre-id → name (§8 enrichment). /search/multi and /discover return
// `genre_ids`, not names; this static table maps them so a Candidate carries
// human/model-readable genres for theme reasoning + scoring. The lists are the
// public, stable TMDB movie + TV genre sets (GET /genre/{movie,tv}/list) —
// hardcoded so the core needs no extra network call and no new dependency.
// Movie and TV id spaces overlap but agree where they share an id; the merged
// map below is safe because same-id genres share the same name across both.
var genreByID = map[int]string{
	// Movie genres
	28:    "Action",
	12:    "Adventure",
	16:    "Animation",
	35:    "Comedy",
	80:    "Crime",
	99:    "Documentary",
	18:    "Drama",
	10751: "Family",
	14:    "Fantasy",
	36:    "History",
	27:    "Horror",
	10402: "Music",
	9648:  "Mystery",
	10749: "Romance",
	878:   "Science Fiction",
	10770: "TV Movie",
	53:    "Thriller",
	10752: "War",
	37:    "Western",
	// TV genres not already covered above
	10759: "Action & Adventure",
	10762: "Kids",
	10763: "News",
	10764: "Reality",
	10765: "Sci-Fi & Fantasy",
	10766: "Soap",
	10767: "Talk",
	10768: "War & Politics",
}

// genreNames maps TMDB genre ids to names, dropping unknown ids. Returns nil for
// an empty input so the Candidate's `genres` stays omitted (omitempty).
func genreNames(ids []int) []string {
	if len(ids) == 0 {
		return nil
	}
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if name, ok := genreByID[id]; ok {
			out = append(out, name)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// genreIDByName is the reverse map for /discover (intent genre term → TMDB id),
// keyed by the CANONICAL genre form (lowercased, common synonyms folded) so an
// intent term like "sci-fi" or "scifi" resolves to Science Fiction. Includes a
// few synonym aliases beyond the literal TMDB names.
var genreIDByName = func() map[string]int {
	m := make(map[string]int, len(genreByID))
	for id, name := range genreByID {
		m[canonGenre(name)] = id
	}
	// Common intent synonyms → canonical genre.
	aliases := map[string]string{
		"scifi": "science fiction", "sci fi": "science fiction", "sf": "science fiction",
		"rom com": "romance", "romcom": "romance", "docs": "documentary",
		"kids": "family", "cartoon": "animation", "cartoons": "animation", "anime": "animation",
	}
	for alias, target := range aliases {
		if id, ok := m[canonGenre(target)]; ok {
			m[canonGenre(alias)] = id
		}
	}
	return m
}()

// canonGenre normalizes a genre term for matching: lowercase, trimmed, and
// hyphens folded to spaces ("Sci-Fi" → "sci fi").
func canonGenre(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "-", " ")
	return s
}
