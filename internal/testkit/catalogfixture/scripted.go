package catalogfixture

import (
	"context"
	"strings"
	"sync"

	"github.com/loomarr/loomarr/internal/library/mediaserver"
)

// SearchStub is one in-library title a ScriptedLibrary returns. Terms are the query substrings it
// answers (case-insensitive), so a test can drive a themed intent through real in-library titles.
type SearchStub struct {
	Terms          []string // query substrings this title answers (e.g. "cartoon", "90s")
	LibraryItemID  string
	Name           string
	Type           string // "Movie" | "Series"
	Year           int
	TMDBID         int
	TVDBID         int
	Genres         []string
	OfficialRating string
	// RunTimeTicks is the title's runtime in 100ns ticks; the result carries it in whole minutes.
	RunTimeTicks int64
}

func (s SearchStub) matches(term string) bool {
	lt := strings.ToLower(term)
	for _, t := range s.Terms {
		if strings.Contains(lt, strings.ToLower(t)) || strings.Contains(strings.ToLower(t), lt) {
			return true
		}
	}
	return false
}

func (s SearchStub) result() mediaserver.SearchResult {
	return mediaserver.SearchResult{
		LibraryItemID: s.LibraryItemID, Name: s.Name, Year: s.Year, MediaType: mediaserver.MediaType(s.Type),
		TMDBID: s.TMDBID, TVDBID: s.TVDBID, Genres: s.Genres, OfficialRating: s.OfficialRating,
		RuntimeMinutes: int(s.RunTimeTicks / 600_000_000),
	}
}

// ScriptedLibrary is a library-scope searcher over scripted titles: a search returns every title
// whose Terms match, in order, and records the term. Err, when set, fails every search.
type ScriptedLibrary struct {
	Err      error
	mu       sync.Mutex
	items    []SearchStub
	searches []string
}

// NewScriptedLibrary holds items.
func NewScriptedLibrary(items ...SearchStub) *ScriptedLibrary {
	return &ScriptedLibrary{items: items}
}

// SetSearchItems replaces what the library holds.
func (l *ScriptedLibrary) SetSearchItems(items ...SearchStub) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.items = items
}

func (l *ScriptedLibrary) Search(_ context.Context, term string, limit int) ([]mediaserver.SearchResult, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.searches = append(l.searches, term)
	if l.Err != nil {
		return nil, l.Err
	}
	var out []mediaserver.SearchResult
	for _, s := range l.items {
		if limit > 0 && len(out) == limit {
			break
		}
		if s.matches(term) {
			out = append(out, s.result())
		}
	}
	return out, nil
}

// Searches lists the terms searched, in order.
func (l *ScriptedLibrary) Searches() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]string(nil), l.searches...)
}
