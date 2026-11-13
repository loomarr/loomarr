// Package resolvers answers the questions other areas ask about what the household owns, from the
// Library's own records (#2151 step 16): which titles exist, a title's library item id, its content
// rating, the collections it is in, and its TMDB franchise. The media server is never asked.
package resolvers

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/loomarr/loomarr/internal/library/mediaserver"
	"github.com/loomarr/loomarr/internal/library/provision"
	"github.com/loomarr/loomarr/internal/library/titlesearch"
	"github.com/loomarr/loomarr/internal/library/tmdb"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
)

// Scanner answers the availability scan from the Library's own titles, so a title confirms with
// its library item id.
type Scanner struct {
	titles librarystore.LibraryTitleReader
}

// NewScanner returns the availability scan's title source over the Library's titles.
func NewScanner(titles librarystore.LibraryTitleReader) Scanner { return Scanner{titles: titles} }

func (s Scanner) RecentlyAdded(ctx context.Context, since time.Time) ([]mediaserver.SearchResult, error) {
	return s.scan(ctx, since)
}

func (s Scanner) AllItems(ctx context.Context) ([]mediaserver.SearchResult, error) {
	return s.scan(ctx, time.Time{})
}

func (s Scanner) scan(ctx context.Context, since time.Time) ([]mediaserver.SearchResult, error) {
	titles, err := s.titles.LibraryTitles(ctx, since)
	if err != nil {
		return nil, err
	}
	out := make([]mediaserver.SearchResult, 0, len(titles))
	for _, t := range titles {
		out = append(out, titlesearch.LibrarySearchResult(t))
	}
	return out, nil
}

// OwnedLookup is the provisioning backstop's last look before it gives a title up
// (provisionbackstop.LibraryLookup): a title the Library holds is confirmed with its library item
// id.
type OwnedLookup struct {
	titles librarystore.LibraryTitleReader
}

// NewOwnedLookup returns the backstop's lookup over the Library's titles.
func NewOwnedLookup(titles librarystore.LibraryTitleReader) OwnedLookup {
	return OwnedLookup{titles: titles}
}

func (l OwnedLookup) Lookup(
	ctx context.Context, kind mediaserver.ProviderKind, id string, mt mediaserver.MediaType,
) (string, bool, error) {
	t, ok, err := titlesearch.OwnedTitle(ctx, l.titles, kind, id, mt)
	return t.ID, ok, err
}

// Ratings resolves an approved lineup entry's content rating by the entry's Key, for the
// reconcile-time heal of unrated entries (§389 amendment), from the Library's own title, so the
// rating is never a guess.
type Ratings struct {
	titles librarystore.LibraryTitleReader
}

// NewRatings returns the rating resolver over the Library's titles.
func NewRatings(titles librarystore.LibraryTitleReader) Ratings { return Ratings{titles: titles} }

func (a Ratings) Rating(ctx context.Context, key provision.Key) (string, bool, error) {
	mt, provider, id, ok := provision.ParseKey(key)
	if !ok {
		return "", false, nil // an unparseable key can't be looked up; leave it unrated
	}
	kind := mediaserver.TMDB
	if provider == "tvdb" {
		kind = mediaserver.TVDB
	}
	lmt := mediaserver.Movie
	if mt == provision.Series {
		lmt = mediaserver.Series
	}
	t, ok, err := titlesearch.OwnedTitle(ctx, a.titles, kind, strconv.Itoa(id), lmt)
	if err != nil || !ok {
		return "", false, err
	}
	return t.Rating, true, nil
}

// Franchises resolves a MOVIE entry's TMDB collection (franchise) id from the entry's Key, for the
// reconcile-time heal (§5 franchise ordering). Only a tmdb-keyed movie can be resolved — a
// tvdb-keyed movie or a series returns ok=false (no lookup), so the entry stays ungrouped
// (harmless).
type Franchises struct{ tmdb *tmdb.Client }

// NewFranchises returns the franchise resolver over the TMDB client.
func NewFranchises(client *tmdb.Client) Franchises { return Franchises{tmdb: client} }

func (a Franchises) Collection(ctx context.Context, key provision.Key) (int, bool, error) {
	mt, provider, id, ok := provision.ParseKey(key)
	if !ok || mt == provision.Series || provider != "tmdb" {
		return 0, false, nil // only a tmdb-keyed movie has a resolvable collection
	}
	cid, err := a.tmdb.CollectionID(ctx, mt, id)
	if errors.Is(err, tmdb.ErrAPIKeyRequired) {
		// The resolver is deliberately wired before TMDB is configured. An absent or
		// freshly cleared key means franchise metadata is unavailable, not that the
		// channel reconcile failed. Returning ok=false leaves the entry unresolved so
		// a later configured reconcile can heal it.
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return cid, true, nil // ok=true (resolved); cid 0 means "standalone", a settled answer
}
