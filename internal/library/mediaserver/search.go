package mediaserver

// SearchResult is one Library title as the catalog, the availability scan and channel ideas read
// it: its library item id, the external ids grounding and dedupe need, and the facts audience
// enforcement and theme scoring read. Since #2151 step 16 it is filled from the Library's tables
// (titlesearch.LibrarySearchResult), never from the media server.
type SearchResult struct {
	LibraryItemID string
	Name          string
	Year          int
	MediaType     MediaType // Movie | Series
	TMDBID        int
	TVDBID        int
	IMDBID        string
	// Genres + Overview enrich the catalog candidate for theme reasoning/scoring (§8). Display
	// only.
	Genres   []string
	Overview string
	// RuntimeMinutes is the title's runtime rounded down to whole minutes. Zero is unavailable,
	// never a zero-length title.
	RuntimeMinutes int
	// OfficialRating is the content rating ("TV-Y7", "PG-13", …), the input to ChannelPolicy
	// audience enforcement (programming-design §4). Display/enforcement metadata only — never
	// identity.
	OfficialRating string
	// SizeBytes is the playing file's size. Zero is unavailable (a series has no file of its
	// own), never an empty file.
	SizeBytes int64
}
