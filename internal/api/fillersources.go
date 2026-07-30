package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/mantonx/loomarr/internal/store"
)

// GET /v1/filler/sources — where a clip catalog comes from (§10, V28).
//
// ⚠ A READ-MODEL, deliberately: there is no `sources` table. Filler discovery is driven by
// `filler.dir` (the watched folder) plus the media-server library scan, and adding a table
// would create a second source of truth needing a precedence rule against the setting — "the
// row says /data/filler, the setting says /srv/clips, which wins?" is a question with no good
// answer. So this DESCRIBES the configuration that already exists, deriving each row's live
// clip count from the catalog itself.
//
// V33 owns the persisted registry, when remote sources genuinely need rows to remember (an
// Archive.org collection has state a setting cannot hold: a fetch cursor, a licence, a last
// error). At that point a row and a setting stop competing, because they describe different
// things.

// FillerSourceDTO is one row of the mock's Sources tab.
type FillerSourceDTO struct {
	// Kind is what sort of source this is: folder (the watched drop-folder), library (the
	// media server's own scan), or remote (fetched in by the ingest sidecar).
	Kind string `json:"kind" enum:"folder,library,remote"`
	// Target is the thing itself — a path, a library name, a URL.
	Target string `json:"target"`
	// Detail is operator-facing prose explaining how this source behaves, rendered verbatim.
	Detail string `json:"detail"`
	// Count is how many catalog clips came from this source, counted live.
	Count int `json:"count"`
	// Configured is false for a source the install could use but has not set up. Rendered as
	// an invitation rather than hidden: "no drop-folder configured" is the answer to "why is
	// my catalog empty", and hiding the row leaves that question unanswered.
	Configured bool `json:"configured"`
	// Fetchable marks a source that `POST /v1/filler/sources/fetch` can refresh on demand.
	Fetchable bool `json:"fetchable"`
}

type fillerSourcesOutput struct {
	Body struct {
		Sources []FillerSourceDTO `json:"sources"`
		// Total is the catalog size — the mock's "N sources · M clips" line. Sent rather than
		// summed client-side because a clip whose `source` matches no known row still counts
		// toward the catalog, and a client summing the rows would under-report.
		Total int `json:"total"`
	}
}

// registerFillerSources mounts the sources read-model. Admin-only: it names filesystem paths
// and library targets, which is infrastructure detail a member has no business reading.
func (s *Server) registerFillerSources(api huma.API) {
	huma.Register(api, withRole(huma.Operation{
		OperationID: "list-filler-sources", Method: http.MethodGet, Path: "/v1/filler/sources",
		Summary: "Where the clip catalog comes from",
		Description: "Admin only. Derived from the configured drop-folder and library scan plus live " +
			"per-source clip counts — there is no sources table (§10). A source the install could use " +
			"but has not configured is returned with configured:false rather than omitted.",
		Tags: []string{"filler"},
	}, RoleAdmin), s.listFillerSources)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "fetch-filler-source", Method: http.MethodPost, Path: "/v1/filler/sources/fetch",
		Summary: "Re-scan the catalog now",
		Description: "Admin only. Runs the same sync as POST /v1/filler/sync — the Sources tab's " +
			"per-row `Fetch now`. Separate operation id so the UI's affordance is nameable, same work.",
		Tags: []string{"filler"},
	}, RoleAdmin), s.fetchFillerSource)
}

func (s *Server) listFillerSources(ctx context.Context, _ *struct{}) (*fillerSourcesOutput, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	if s.store == nil {
		return nil, huma.Error501NotImplemented("no store configured")
	}
	clips, err := s.store.ListClips(ctx, store.ClipFilter{})
	if err != nil {
		return nil, huma.Error500InternalServerError("list clips", err)
	}

	// Count by provenance. The `source` column is free text written by whatever discovered the
	// clip, so an unrecognized value is possible and must not vanish — see the Total field.
	bySource := map[string]int{}
	for _, c := range clips {
		bySource[c.Source]++
	}

	// Read live rather than through a dedicated field: filler.dir hot-applies
	// (config-design §3), and a value captured at construction would report the old folder
	// after an operator changed it — on the very screen they would go to check.
	dir := ""
	if s.liveConfig != nil {
		dir = s.liveConfig("filler.dir")
	}

	out := &fillerSourcesOutput{}
	out.Body.Total = len(clips)
	out.Body.Sources = []FillerSourceDTO{
		{
			Kind:   "folder",
			Target: orPlaceholder(dir, "not configured"),
			Detail: "watched directly — new files appear on the next pass",
			// filler-dir is what DirSource writes; the older tunarr-local value is counted
			// here too because those clips also live in the folder — the provenance string
			// changed with §9.1, the files did not.
			Count:      bySource["filler-dir"] + bySource["tunarr-local"],
			Configured: dir != "",
			Fetchable:  dir != "",
		},
		{
			Kind:   "library",
			Target: "media server filler library",
			Detail: "scanned by the media server",
			Count:  bySource["library"],
			// The library connection is configured iff its URL is set — the same signal every
			// other library-dependent surface gates on.
			Configured: s.liveConfig != nil && s.liveConfig("library.url") != "",
			Fetchable:  s.filler != nil,
		},
		{
			Kind:   "remote",
			Target: "ingest sidecar",
			Detail: "fetches into the watched folder — needs the loomarr:filler image",
			Count:  bySource["ingest"] + bySource["youtube"] + bySource["archive"],
			// The sidecar's availability is only knowable by trying (ErrIngestUnavailable), so
			// this reports whether the ROUTE exists rather than claiming the tooling is present.
			Configured: s.filler != nil,
			// Not fetchable from here: a remote fetch needs URLs, which is POST /v1/filler/ingest.
			// A "Fetch now" that silently did nothing would be worse than no button.
			Fetchable: false,
		},
	}
	return out, nil
}

type fetchFillerSourceOutput struct {
	Body struct {
		Total   int `json:"total"`
		Added   int `json:"added"`
		Updated int `json:"updated"`
		Pruned  int `json:"pruned"`
	}
}

func (s *Server) fetchFillerSource(ctx context.Context, _ *struct{}) (*fetchFillerSourceOutput, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	if s.filler == nil {
		return nil, huma.Error501NotImplemented("filler sync is not available on this instance")
	}
	total, added, updated, pruned, err := s.filler.Sync(ctx)
	if err != nil {
		return nil, huma.Error502BadGateway("sync filler catalog", err)
	}
	out := &fetchFillerSourceOutput{}
	out.Body.Total, out.Body.Added, out.Body.Updated, out.Body.Pruned = total, added, updated, pruned
	return out, nil
}

func orPlaceholder(v, placeholder string) string {
	if v == "" {
		return placeholder
	}
	return v
}
