package filler

import (
	"context"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/api/httpcore"
)

type discoverFillerInput struct {
	// Query is what the operator typed. An empty search would return archive.org's entire
	// movies corpus ranked by nothing, which is not an answer to any question — so a search
	// needs at least two characters. Not `required`, because `collection` is the other way to
	// ask; at least one of the two must be given (checked in the handler).
	Query string `query:"q" minLength:"2" doc:"Words to search for, e.g. \"1980s cereal commercial\". When collection is also present, searches within that collection."`
	// Collection lists ONE named archive.org collection instead of searching. This is what a
	// starter pack is (§10, V17d): a curated collection an operator keeps or excludes from
	// before anything is fetched. Deliberately the same endpoint as the keyword search — a
	// separate route would be a second implementation of "list clips, download nothing".
	Collection string `query:"collection" doc:"An archive.org collection to list or search: a URL, a /details/<id> path, or a bare identifier. May be combined with q to search within it."`
	// Limit caps the page. A listing is for DECIDING — an operator judges a source from a
	// handful of titles — so the ceiling is low on purpose.
	Limit int `query:"limit" minimum:"1" maximum:"25" doc:"Max results (default 25)"`
}

type discoverFillerOutput struct {
	Body struct {
		Items []DiscoveredClip `json:"items"`
		// Total is how many the SOURCE matched, not how many were returned. An operator
		// judging "is this search any good" needs the real number: 54 hits shown 5 at a
		// time is a different situation from 5 hits total.
		Total int `json:"total"`
		// ⚠ Stated ONCE here rather than as a per-item field, and that is deliberate.
		// archive.org declares a licence on only ~8% of items, and yt-dlp reports none at
		// all, so a per-result licence chip would read "unknown" on nearly every row —
		// implying a check that never happened. See the build plan §6.3.
		LicenceNote string `json:"licenceNote"`
	}
}

// discoverFiller searches for clips the operator could add, downloading nothing.
func (s *Handlers) discoverFiller(ctx context.Context, in *discoverFillerInput) (*discoverFillerOutput, error) {
	if s.filler == nil {
		return nil, httpcore.NotImplemented("Filler isn't set up",
			"Enable filler in Settings before searching for clips to add.")
	}
	if err := s.requireFillerProvider(ctx, "archive"); err != nil {
		return nil, err
	}

	// At least one mode. With both present, the keyword is scoped to the named collection — the
	// request made by a source row's search box. Neither is the empty search the minLength above
	// exists to prevent.
	query, collection := strings.TrimSpace(in.Query), strings.TrimSpace(in.Collection)
	if query == "" && collection == "" {
		return nil, httpcore.Problem(http.StatusUnprocessableEntity, "Ask for one thing",
			"Send q to search Archive.org, collection to list one collection, or both to search within it.")
	}

	var (
		items []DiscoveredClip
		total int
		err   error
	)
	if collection != "" {
		items, total, err = s.filler.DiscoverCollection(ctx, collection, query, in.Limit)
	} else {
		items, total, err = s.filler.Discover(ctx, query, in.Limit)
	}
	if err != nil {
		// A listing failure is upstream (archive.org unreachable or refusing), not the
		// caller's fault — so it is a 502-shaped problem, and the message says which side
		// broke rather than blaming the query.
		return nil, httpcore.ProblemWithCause(http.StatusBadGateway, "Couldn't search for clips",
			"archive.org didn't answer. Try again in a moment.", err)
	}

	out := &discoverFillerOutput{}
	// Non-nil so a client can iterate without guarding; "nothing matched" is a real answer.
	out.Body.Items = items
	if out.Body.Items == nil {
		out.Body.Items = []DiscoveredClip{}
	}
	for i := range out.Body.Items {
		out.Body.Items[i].EmbedURL = httpcore.EmbedURL(s.sourceProviders, "archive", out.Body.Items[i].URL)
	}
	out.Body.Total = total
	out.Body.LicenceNote = "Licence information isn't available for most results. Check before reusing."
	return out, nil
}

func (s *Handlers) requireFillerProvider(ctx context.Context, kind string) error {
	if s.store == nil {
		return huma.Error501NotImplemented("no store configured")
	}
	providers, err := s.store.ListFillerProviders(ctx)
	if err != nil {
		return huma.Error500InternalServerError("read filler provider policy", err)
	}
	for _, provider := range providers {
		if provider.Kind == kind {
			if provider.Enabled {
				return nil
			}
			return httpcore.Conflict("That provider is paused",
				"Resume it under Filler → Sources before searching or downloading from it.")
		}
	}
	return httpcore.NotFound("Provider not found", "That filler provider isn't available on this install.")
}

type discoverStatsInput struct {
	// IDs are the search-result identifiers to describe, as REPEATED params (`?id=a&id=b&id=c`).
	//
	// ⚠ **`explode` is load-bearing.** Huma disables it for query fields by default ("parsing is
	// *much* easier if we use comma-separated values"), so `?id=a&id=b` would bind ONE element.
	// The generated TypeScript client sends the repeated form, so the contract is repeated
	// parameters, matching the caller.
	//
	// ⚠ **`explode` is also what lets `maxItems` fire.** Each id is one upstream call, so the cap
	// exists to stop an uncapped list making Loomarr hammer archive.org on someone else's behalf;
	// with a comma-joined value the slice is never long and the cap never trips.
	IDs []string `query:"id,explode" required:"true" maxItems:"25" doc:"Result ids to describe. Repeat the parameter per id (?id=a&id=b)."`
}

type discoverStatsOutput struct {
	Body struct {
		// Stats is keyed by result id. ⚠ An id archive.org could not answer for is ABSENT,
		// never present-with-zeros: 0 renders as "0:00", which claims a clip is empty, and
		// "unknown" is the only honest answer for an item it never probed.
		Stats map[string]DiscoveredClipStats `json:"stats"`
	}
}

// discoverFillerStats fills in the duration + quality a search omits (§10, V35).
//
// ⚠ **A separate route because it is a separate COST, measured against the live API.** A
// listing is one Solr request; these two fields are one `/metadata/<id>` call each — 22.6s for
// a page of 25, with a per-call median of 1.78s. Widening the fan-out makes it worse (12 and 25
// concurrent both measured 25s against 6's 15.6s), so archive.org is throttling and the latency
// is not ours to tune away. Asking only for rows a person is looking at is the fix.
func (s *Handlers) discoverFillerStats(ctx context.Context, in *discoverStatsInput) (*discoverStatsOutput, error) {
	if s.filler == nil {
		return nil, httpcore.NotImplemented("Filler isn't set up",
			"Enable filler in Settings before searching for clips to add.")
	}
	if len(in.IDs) == 0 {
		return nil, httpcore.Problem(http.StatusUnprocessableEntity, "Nothing to describe",
			"Send at least one id.")
	}
	if err := s.requireFillerProvider(ctx, "archive"); err != nil {
		return nil, err
	}

	stats, err := s.filler.EnrichDiscovered(ctx, in.IDs)
	if err != nil {
		return nil, httpcore.ProblemWithCause(http.StatusBadGateway, "Couldn't read clip details",
			"archive.org didn't answer. Try again in a moment.", err)
	}

	out := &discoverStatsOutput{}
	// Non-nil so a client can index without guarding; "nothing known" is a real answer.
	out.Body.Stats = stats
	if out.Body.Stats == nil {
		out.Body.Stats = map[string]DiscoveredClipStats{}
	}
	return out, nil
}
