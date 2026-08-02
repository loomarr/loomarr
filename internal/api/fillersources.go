package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/mantonx/loomarr/internal/filler"
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

// FillerSourceDTO is one row of the Sources tab.
type FillerSourceDTO struct {
	// ID addresses this row on PATCH/DELETE. The two DERIVED rows carry the stable literals
	// `folder` and `library`; a remote collection carries its registry id.
	//
	// ⚠ Giving the derived rows ids does NOT make them table rows — they remain a read-model
	// over configuration (see the file header). It gives the Sources tab ONE way to address a
	// source, so the UI does not have to know that flipping the folder writes a setting while
	// flipping a collection writes a column. That asymmetry is a storage fact, not something an
	// operator should have to hold.
	ID string `json:"id"`
	// Enabled is the row's on/off switch (V35). ⚠ Off means Loomarr stops scanning, searching
	// and downloading from this source — it does NOT remove clips already in the catalog.
	Enabled bool `json:"enabled"`
	// Switchable is false for a row with no work to stop. ⚠ The `library` row is the case:
	// nothing scans a media-server library for clips (§10 took the media server out of the
	// filler path), so a switch there would dim a row and change nothing. It survives as
	// PROVENANCE for clips catalogued under the pre-§9.1 model.
	Switchable bool `json:"switchable"`
	// Removable is true only for rows that can be forgotten — the registered remotes. The
	// derived rows describe configuration, which is changed in Settings, not deleted here.
	Removable bool `json:"removable"`
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
	// Remotes are the specific archive.org items an operator ADDED, nested under the
	// `remote` row (V33). Empty on every other row.
	//
	// ⚠ Nested rather than promoted to peers, deliberately. The three rows above describe
	// CONFIGURATION — including "you could set up a library but have not", which is what
	// `configured:false` is for — and a flat list of things that EXIST cannot express that.
	// Merging the two models would show the drop-folder twice; see the build plan §6.1.
	Remotes []RemoteSourceDTO `json:"remotes,omitempty"`
}

// RemoteSourceDTO is one registered remote source (§10, V33).
type RemoteSourceDTO struct {
	ID string `json:"id"`
	// Enabled is this collection's own switch, stored as a column on its row (V35).
	Enabled bool `json:"enabled"`
	// Label falls back to the URI when a source has none, so a row is never blank.
	Label string `json:"label"`
	URI   string `json:"uri"`
	// LastFetchedAt is absent when never fetched — rendered as "never" rather than as an
	// epoch date nobody meant.
	LastFetchedAt string `json:"lastFetchedAt,omitempty" doc:"RFC3339; absent if never fetched"`
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

	huma.Register(api, withRole(huma.Operation{
		OperationID: "add-filler-source", Method: http.MethodPost, Path: "/v1/filler/sources",
		Summary: "Register a remote collection to pull filler from",
		Description: "Admin only (§10 V35). Registers an archive.org collection so pulls and searches can " +
			"use it. Registering DOWNLOADS NOTHING — it records that this source exists and is allowed; " +
			"fetching is the ingest path, and a composed multi-source pull goes through the approval gate.",
		Tags: []string{"filler"},
	}, RoleAdmin), s.addFillerSource)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "set-filler-source-enabled", Method: http.MethodPatch, Path: "/v1/filler/sources/{id}",
		Summary: "Switch a source on or off",
		Description: "Admin only (§10 V35). Off means Loomarr stops scanning, searching and downloading from " +
			"this source. ⚠ It does NOT remove clips already in the catalog, and it is not a delete: the " +
			"source keeps its licence and fetch history, so switching it back on resumes rather than restarts. " +
			"`folder` writes the drop-folder setting; any other id writes that collection's own row. The " +
			"`library` row is not switchable — nothing scans a media-server library for clips (§10).",
		Tags: []string{"filler"},
	}, RoleAdmin), s.setFillerSourceEnabled)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "delete-filler-source", Method: http.MethodDelete, Path: "/v1/filler/sources/{id}",
		Summary: "Forget a registered remote collection",
		Description: "Admin only (§10 V35). ⚠ Clips it already brought in are NOT deleted — they are real " +
			"files, already tagged and possibly pinned into a channel, and forgetting where something came " +
			"from is not a reason to throw it away. Only the registered remotes can be deleted; the derived " +
			"folder and library rows describe configuration, which is changed in Settings.",
		Tags: []string{"filler"},
	}, RoleAdmin), s.deleteFillerSource)
}

// addFillerSourceInput registers a remote collection.
type addFillerSourceInput struct {
	Body struct {
		URI   string `json:"uri" minLength:"1" doc:"archive.org collection identifier, /details/<id> path, or full URL"`
		Label string `json:"label,omitempty" doc:"Operator-facing name; falls back to the identifier"`
	}
}

type addFillerSourceOutput struct {
	Body RemoteSourceDTO
}

// addFillerSource registers a remote collection. Downloading is a separate, gated act.
func (s *Server) addFillerSource(ctx context.Context, in *addFillerSourceInput) (*addFillerSourceOutput, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	if s.store == nil {
		return nil, huma.Error501NotImplemented("no store configured")
	}
	id := archiveIdentifier(in.Body.URI)
	if id == "" {
		return nil, errBadRequest("That doesn't look like an archive.org collection",
			"Paste a collection identifier (like classic_tv_commercials) or its archive.org URL.")
	}
	label := strings.TrimSpace(in.Body.Label)
	if label == "" {
		label = id
	}
	// NewFillerSource, never a struct literal: `Enabled` is a bool, so a literal that omits it
	// registers the collection SWITCHED OFF (see the store).
	src := store.NewFillerSource(id, "archive", strings.TrimSpace(in.Body.URI), label, time.Now().UTC())
	if err := s.store.UpsertFillerSource(ctx, src); err != nil {
		return nil, huma.Error500InternalServerError("register filler source", err)
	}
	return &addFillerSourceOutput{Body: RemoteSourceDTO{
		ID: src.ID, Label: src.Label, URI: src.URI, Enabled: src.Enabled,
	}}, nil
}

type setFillerSourceEnabledInput struct {
	ID   string `path:"id"`
	Body struct {
		Enabled bool `json:"enabled"`
	}
}

type setFillerSourceEnabledOutput struct {
	Body struct {
		ID      string `json:"id"`
		Enabled bool   `json:"enabled"`
	}
}

// setFillerSourceEnabled flips a source's switch.
//
// ⚠ The dispatch below is the one place the storage asymmetry surfaces, and it is deliberately
// here rather than in the client: `folder` is derived from configuration so its switch is a
// SETTING, while a collection is a row so its switch is a COLUMN. A UI that had to know which
// is which would encode a persistence decision in a button handler.
func (s *Server) setFillerSourceEnabled(ctx context.Context, in *setFillerSourceEnabledInput) (*setFillerSourceEnabledOutput, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	out := &setFillerSourceEnabledOutput{}
	out.Body.ID, out.Body.Enabled = in.ID, in.Body.Enabled

	switch in.ID {
	case "folder":
		if s.settings == nil {
			return nil, huma.Error501NotImplemented("settings are not available on this instance")
		}
		// Through Patch rather than a bespoke setter: it validates, hot-applies and records
		// WHO changed it, so flipping this switch is audited exactly like editing the same key
		// on the Settings page. A private write path would be the one setting with no history.
		results := s.settings.Patch(ctx,
			map[string]string{"filler.source.folder.enabled": strconv.FormatBool(in.Body.Enabled)},
			auditActor(ctx))
		for _, r := range results {
			if r.Status == "pinned" {
				return nil, errConflict("That switch is set by the environment",
					"FILLER_SOURCE_FOLDER_ENABLED is pinned in this deployment's configuration, so it can't be changed here.")
			}
			if r.Status != "saved" {
				return nil, huma.Error500InternalServerError("save the drop-folder switch: "+r.Status, nil)
			}
		}
		return out, nil
	case "library", "remote":
		// Refused rather than silently accepted. Nothing scans a media-server library for
		// clips (§10), and `remote` is a container whose children carry the switches — so
		// storing a flag for either would be a control that changes nothing.
		return nil, errConflict("That source has no switch",
			"This row is here to show where existing clips came from. There's nothing running to turn off.")
	default:
		if s.store == nil {
			return nil, huma.Error501NotImplemented("no store configured")
		}
		if err := s.store.SetFillerSourceEnabled(ctx, in.ID, in.Body.Enabled); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, errNotFound("Source not found", "That source isn't registered — it may have been removed.")
			}
			return nil, huma.Error500InternalServerError("set filler source enabled", err)
		}
		return out, nil
	}
}

type deleteFillerSourceInput struct {
	ID string `path:"id"`
}

// deleteFillerSource forgets a registered remote. Its clips stay.
func (s *Server) deleteFillerSource(ctx context.Context, in *deleteFillerSourceInput) (*struct{}, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	if s.store == nil {
		return nil, huma.Error501NotImplemented("no store configured")
	}
	switch in.ID {
	case "folder", "library", "remote":
		// The derived rows are configuration, not registrations. Deleting one would have to
		// mean "unset filler.dir", which belongs in Settings where the consequence is legible.
		return nil, errConflict("That source can't be removed here",
			"This row describes how Loomarr is configured. Change it in Settings instead.")
	}
	if err := s.store.DeleteFillerSource(ctx, in.ID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, errNotFound("Source not found", "That source isn't registered — it may already have been removed.")
		}
		return nil, huma.Error500InternalServerError("delete filler source", err)
	}
	return nil, nil
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

	// ⚠ Read through the BOOL seam, never liveConfig: settings.String panics on a non-string
	// Kind, so routing this key through the string accessor took the whole route down with an
	// empty reply rather than returning something a comparison could inspect.
	//
	// An unanswerable read is ON, matching the syncer's own gate (`resolved.boolOn`) and the
	// setting's declared default. Treating it as "off" would render the drop-folder switched
	// off on a page whose whole job is telling the operator why their catalog is empty.
	folderEnabled := s.liveConfigBoolOn == nil || s.liveConfigBoolOn("filler.source.folder.enabled")

	// The registered remotes, nested under the `remote` row below. A read failure is NOT
	// fatal: the three config rows are the answer to "why is my catalog empty", and losing
	// that whole page because a secondary list did not load would be a poor trade.
	var remotes []RemoteSourceDTO
	if srcs, srcErr := s.store.ListFillerSources(ctx); srcErr != nil {
		s.log.Warn("list filler sources", "err", srcErr)
	} else {
		for _, src := range srcs {
			r := RemoteSourceDTO{ID: src.ID, Label: src.Label, URI: src.URI, Enabled: src.Enabled}
			if r.Label == "" {
				r.Label = src.URI
			}
			if !src.LastFetchedAt.IsZero() {
				r.LastFetchedAt = src.LastFetchedAt.UTC().Format(time.RFC3339)
			}
			remotes = append(remotes, r)
		}
	}

	out := &fillerSourcesOutput{}
	out.Body.Total = len(clips)
	out.Body.Sources = []FillerSourceDTO{
		{
			ID:   "folder",
			Kind: "folder",
			// The folder's switch is a SETTING, because the folder itself is derived from one.
			// `boolOn` semantics: anything other than an explicit false reads as on, so a
			// settings service that cannot answer does not silently stop the scan.
			Enabled:    folderEnabled,
			Switchable: true,
			Target:     orPlaceholder(dir, "not configured"),
			Detail:     "watched directly — new files appear on the next pass",
			// filler-dir is what DirSource writes; the older tunarr-local value is counted
			// here too because those clips also live in the folder — the provenance string
			// changed with §9.1, the files did not.
			Count:      bySource["filler-dir"] + bySource["tunarr-local"],
			Configured: dir != "",
			Fetchable:  dir != "",
		},
		{
			ID:   "library",
			Kind: "library",
			// ⚠ No switch: nothing scans a library for filler since §10 took the media server
			// out of that path, so this row is provenance for older clips rather than a source
			// doing work. `enabled:true` says "these clips count", not "a scan is running".
			Enabled:    true,
			Switchable: false,
			Target:     "media server filler library",
			Detail:     "scanned by the media server",
			Count:      bySource["library"],
			// The library connection is configured iff its URL is set — the same signal every
			// other library-dependent surface gates on.
			Configured: s.liveConfig != nil && s.liveConfig("library.url") != "",
			Fetchable:  s.filler != nil,
		},
		{
			ID:   "remote",
			Kind: "remote",
			// The `remote` row is a container for the registered collections nested under it;
			// each carries its own switch, so the container has none.
			Enabled:    true,
			Switchable: false,
			// ⚠ Was "ingest sidecar" / "needs the loomarr:filler image" (retired-ok), and neither of those
			// things exists any more. The sidecar was folded into the core and the two-tag split
			// was replaced by the single image (§10 records both reversals), so this label was
			// telling operators to go and find a deployment they cannot get.
			Target: "downloads",
			Detail: "fetches clips into the watched folder from a URL you give it",
			Count:  bySource["ingest"] + bySource["youtube"] + bySource["archive"],
			// Availability is only knowable by trying (ErrIngestUnavailable), so this reports
			// whether the ROUTE exists rather than claiming the tooling is present.
			Configured: s.filler != nil,
			// Not fetchable from here: a remote fetch needs URLs, which is POST /v1/filler/ingest.
			// A "Fetch now" that silently did nothing would be worse than no button.
			Fetchable: false,
			Remotes:   remotes,
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
	// Same reason as the sync route: a source the operator switched off is an answer, not a
	// gateway failure. This is the "Fetch now" on the very row carrying the switch.
	if errors.Is(err, filler.ErrSourceDisabled) {
		return nil, errSourceDisabled()
	}
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

// errSourceDisabled is the shared answer for "you switched this off".
//
// 409 rather than 502 or 501: the request is well-formed and the feature is present — the
// install is simply in a state that refuses it, and the operator can change that state. The
// detail names the exact switch, because "disabled" without a location is a dead end.
func errSourceDisabled() huma.StatusError {
	return errConflict("The drop-folder is switched off",
		"Loomarr isn't scanning the drop-folder because that source is turned off. Switch it back on under Filler → Sources, then sync again.")
}

// archiveIdentifier pulls the collection identifier out of whatever an operator pasted.
//
// Three spellings are accepted because all three are things people actually paste: a bare
// identifier, a `/details/<id>` path, and a full URL. Returns "" for anything else, which the
// caller turns into a message naming the shapes that work — silently registering an
// unparseable source would produce a row that can never fetch.
func archiveIdentifier(raw string) string {
	v := strings.TrimSpace(raw)
	if v == "" {
		return ""
	}
	if i := strings.Index(v, "archive.org/details/"); i >= 0 {
		v = v[i+len("archive.org/details/"):]
	} else if i := strings.Index(v, "/details/"); i >= 0 {
		v = v[i+len("/details/"):]
	}
	if j := strings.IndexAny(v, "/?#"); j >= 0 {
		v = v[:j]
	}
	// An identifier is a plain slug. Anything still carrying a scheme, a dot or a space is a
	// URL we did not recognise rather than an id, and guessing would create a dead row.
	if v == "" || strings.ContainsAny(v, ":. \t") {
		return ""
	}
	return v
}
