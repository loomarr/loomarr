package httpcore

// SearchCandidate is the API view of a catalog candidate (§7.2). It carries the
// same source-backed editorial and enforcement evidence as the LLM grounding
// tool, so the doc's "humans and the model see identical results" promise (§8)
// holds on the read side too.
type SearchCandidate struct {
	MediaType        string   `json:"mediaType"`
	TMDBID           int      `json:"tmdbId,omitempty"`
	TVDBID           int      `json:"tvdbId,omitempty"`
	Name             string   `json:"name"`
	Year             int      `json:"year,omitempty"`
	InLibrary        bool     `json:"inLibrary"`
	LibraryItemID    string   `json:"libraryItemId,omitempty"`
	Genres           []string `json:"genres,omitempty"`
	Overview         string   `json:"overview,omitempty"`
	OriginalLanguage string   `json:"originalLanguage,omitempty"`
	OriginCountries  []string `json:"originCountries,omitempty"`
	RuntimeMinutes   int      `json:"runtimeMinutes,omitempty"`
	VoteAverage      float64  `json:"voteAverage,omitempty"`
	VoteCount        int      `json:"voteCount,omitempty"`
	Keywords         []string `json:"keywords,omitempty"`
	Networks         []string `json:"networks,omitempty"`
	Cast             []string `json:"cast,omitempty"`
	Creators         []string `json:"creators,omitempty"`
	OfficialRating   string   `json:"officialRating,omitempty"`
}
