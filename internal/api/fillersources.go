package api

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"path"
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
	// Switchable is false for a row with no work to stop — a switch there would dim a row and
	// change nothing.
	//
	// ⚠ The remaining case is the legacy `library` PROVENANCE row, which says where clips
	// catalogued under the pre-§9.1 model came from. An operator-ADDED library is switchable like
	// any other source: V38c restored library scanning (§10), so there is real work to stop.
	Switchable bool `json:"switchable"`
	// Removable is true only for rows that can be forgotten — the registered remotes. The
	// derived rows describe configuration, which is changed in Settings, not deleted here.
	Removable bool `json:"removable"`
	// Kind is what sort of source this is (V37: one flat vocabulary, one row per source).
	//
	// ⚠ `remote` is GONE. It was the container the registered archive collections nested under;
	// the flat list has no container, so a collection carries `archive` and a playlist carries
	// `youtube` directly. An old client reading `remote` finds no such row rather than an empty
	// one — the retired-identifier check guards the prose, and the enum guards the wire.
	//
	// ⚠ `packs` is deliberately absent: there is no pack index to read (no URL, no manifest, no
	// fetcher), and §10 forbids a row that dims and changes nothing.
	Kind string `json:"kind" enum:"folder,library,archive,youtube"`
	// Searchable marks a source whose catalog can be searched in place — the Sources tab's
	// per-row search expander. Only `archive` today; the mock draws the same restriction.
	Searchable bool `json:"searchable"`
	// Target is the thing itself — a path, a library name, a URL.
	Target string `json:"target"`
	// URI is the machine-facing source reference. It is deliberately separate from Target:
	// Target may be an operator label, while a collection search must send the actual Archive
	// identifier. Absent on derived provider/provenance rows that have no source to address.
	URI string `json:"uri,omitempty"`
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
	// LastFetchedAt is absent when never fetched — rendered as "never" rather than as an epoch
	// date nobody meant. Empty on the config-backed rows, which are scanned rather than fetched.
	LastFetchedAt string `json:"lastFetchedAt,omitempty" doc:"RFC3339; absent if never fetched"`
	// License is what the source DECLARED about its material — the mock's per-row licence chip.
	//
	// ⚠ **Absent means UNKNOWN, never "public domain".** ~92% of archive.org items declare no
	// licence at all (667 of 8362 measured in `classic_tv_commercials`, 2026-07-31), so absence is
	// the common case and carries no permission whatsoever. The UI must render nothing rather than
	// a reassuring default — a chip that said "public domain" by omission would be Loomarr
	// asserting a legal fact nobody checked.
	//
	// ⚠ Sent here for the first time in V38c. The column has existed since V33 and reached no
	// surface, so the operator could not see what a source claimed even though Loomarr had
	// recorded it.
	License string `json:"license,omitempty" doc:"Licence the source declared; absent means unknown, NOT public domain"`
	// Group marks a PROVIDER node — one service, with the targets an operator added beneath it
	// (§10 V51c). Three archive.org collections stop being three sibling rows and become one
	// Archive.org row that twirls down.
	//
	// ⚠ **A separate axis from `Kind`, deliberately** — the same call §10 V45 makes for
	// `IsComposite`. Adding `provider` to the kind enum would force every switch on kind to grow
	// a case, which is how a container leaks into a place that expects a source: a pull planner
	// or a fetch loop that handled the new value by falling through its `default`.
	Group bool `json:"group,omitempty"`
	// ParentID is the provider node this row sits under, or empty for a top-level row.
	//
	// ⚠ **Derived from `Kind` at read time — there is no `parent_id` column and no migration.**
	// The grouping being asked for IS already a column: every `archive` row belongs under
	// Archive.org, and there is no representable case where it belongs anywhere else. A stored
	// parent would be a second encoding of that fact, and second encodings make illegal states
	// representable (`kind='archive', parentId='provider:youtube'`).
	//
	// ⚠ **Never accepted on a request body.** `POST /v1/filler/sources` takes a kind and derives
	// the parent; accepting one would let a client assert a parent that contradicts the kind.
	ParentID string `json:"parentId,omitempty"`
}

// providerIDPrefix namespaces the derived group ids (`provider:archive`, `provider:youtube`) so
// they can never collide with an operator-produced source id, and so ONE prefix test guards every
// route that addresses a row.
const providerIDPrefix = "provider:"

// isProviderID reports whether an id addresses a derived group node rather than a real source.
func isProviderID(id string) bool { return strings.HasPrefix(id, providerIDPrefix) }

// providerGroups are the kinds that roll up, in the order their groups appear.
//
// ⚠ **`folder` and `library` deliberately do NOT group.** A twirl-down exists because ONE SERVICE
// offers many targets; two watched folders are two unrelated directories with no service in
// common, so a "Folders" container would be a row that dims and changes nothing — §10's own
// forbidden shape. It also leaves the config-backed projection in `listFillerSources` untouched.
var providerGroups = []struct {
	kind, id, label, detail string
}{
	{"archive", providerIDPrefix + "archive", "Archive.org", "collections you've added — searchable here, downloaded when you queue or approve"},
	{"youtube", providerIDPrefix + "youtube", "YouTube", "playlists you've added — titles and descriptions are kept for tagging"},
}

// ⚠ `Remotes` is GONE from this DTO (V37). It carried the archive collections nested under the
// `remote` container row, and the flat list has no container — each collection is now a peer row
// with its own `kind`, `enabled` and `lastFetchedAt`.
//
// The nesting existed for a real reason, recorded here because the reason OUTLIVED the shape:
// the derived rows described CONFIGURATION, including "you could set up a library but have not",
// which a table of things-that-exist cannot express. The flat list carries that property itself
// — `configured` is still a per-row fact, and `folder`/`library` are singleton rows materialised
// even when unset (migration 00029). Flattening without preserving it would have deleted the
// answer to "why is my catalog empty?".

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
		Summary: "Switch a source on or off, and tune how often it is fetched",
		Description: "Admin only (§10 V35, extended V38c). Off means Loomarr stops scanning, searching and " +
			"downloading from this source. ⚠ It does NOT remove clips already in the catalog, and it is not " +
			"a delete: the source keeps its licence and fetch history, so switching it back on resumes rather " +
			"than restarts. `folder` writes the drop-folder setting; any other id writes that source's own row. " +
			"⚠ `fetchEverySeconds` is three-state — omit or send null to inherit the global, 0 to never " +
			"auto-fetch this source, or a positive number of seconds. Library sources became switchable in " +
			"V38c, when §10 restored them to being scanned.",
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

// canFetchRow reports whether this row's "Fetch now" has anything to do.
//
// ⚠ It used to be a bare `s.filler != nil` — "does the ingest route exist" — which is a fact
// about the INSTALL, not about the row. The seeded YouTube source ships with an empty uri (§10
// forbids Loomarr recommending a playlist, so the operator brings one), and that row offered a
// Fetch-now button with nothing to enumerate. A control that cannot work is worse than no
// control.
//
// The two kinds fail for different reasons and are answered separately:
//   - SCANNED (folder, library) read local/LAN storage, so they need no ingest tooling at all —
//     "Fetch now" runs the same sync the scheduler does.
//   - DOWNLOADED (archive, youtube) need BOTH a target to fetch (the store's `Fetchable()`,
//     which is false on an empty uri) and the ingest route to hand it to.
func canFetchRow(src store.FillerSource, ingestAvailable bool) bool {
	if src.Scannable() {
		return true
	}
	return src.Fetchable() && ingestAvailable
}

// addFillerSourceInput registers a source (V37: any addable kind, not only archive).
type addFillerSourceInput struct {
	Body struct {
		// ⚠ Required, and validated PER KIND below. Before V37 this was absent and the kind was
		// hardcoded "archive", so `archiveIdentifier` — which rejects anything containing a dot —
		// was applied to every URI, and a YouTube playlist URL 400'd on a message about
		// archive.org collections. One validator cannot serve two vocabularies.
		Kind  string `json:"kind" enum:"archive,youtube,folder,library" doc:"Which kind of source this is"`
		URI   string `json:"uri" minLength:"1" doc:"An archive.org collection identifier/URL, a YouTube playlist or channel URL, a folder path Loomarr can read, or a media-server library name"`
		Label string `json:"label,omitempty" doc:"Operator-facing name; falls back to the identifier"`
	}
}

// normalizeSourceURI validates a URI against its OWN kind's rules and returns the canonical
// identity to store, or "" if the kind cannot use it.
//
// ⚠ The two kinds disagree about what a valid URI even looks like: an archive identifier is a
// plain slug and is REJECTED if it contains a dot, while a YouTube playlist URL is nothing but
// dots and slashes. That is why validation is per kind rather than one shared check — the
// pre-V37 code applied the archive rule to everything, which is precisely why YouTube could not
// be registered.
//
// ⚠ `folder` and `library` are ADDABLE since V38c. They were singleton rows materialised from
// configuration (migration 00029) and refused here; V38c allows many of each, and the maintainer's
// 2026-08-02 decision returned `library` to being scanned for real (§10). The singleton index is
// gone (migration 00032), so the guard that used to 409 them is gone with it.
func normalizeSourceURI(kind, raw string) string {
	switch kind {
	case "archive":
		return archiveIdentifier(raw)
	case "youtube":
		return youtubeTarget(raw)
	case "folder":
		return folderPath(raw)
	case "library":
		return libraryName(raw)
	default:
		return ""
	}
}

// folderPath validates a watched-folder path an operator typed.
//
// ⚠ **ABSOLUTE ONLY, and that is a correctness rule rather than a style preference.** A relative
// path resolves against Loomarr's working directory, which is an implementation detail the
// operator cannot see and which differs between a container and a `go run`. "ads" would silently
// mean different directories in dev and in production, and the failure — an empty catalog — says
// nothing about why.
//
// ⚠ `..` is refused even inside an absolute path. It cannot escape anything by itself here (the
// scan is confined to whatever this resolves to), but a path that needs cleaning is a path the
// operator did not mean to type, and storing the pre-cleaned form makes the row disagree with the
// directory it names.
func folderPath(raw string) string {
	p := strings.TrimSpace(raw)
	if p == "" || !strings.HasPrefix(p, "/") {
		return ""
	}
	if strings.Contains(p, "..") {
		return ""
	}
	// Cleaned so `/data/filler/` and `/data//filler` are ONE source rather than two rows watching
	// one directory — the duplicate the dropped singleton index no longer prevents.
	cleaned := path.Clean(p)
	if cleaned == "/" {
		// Scanning the filesystem root would walk the entire machine. Refused as an obvious
		// mis-type rather than accepted as an heroic instruction.
		return ""
	}
	return cleaned
}

// libraryName validates a media-server library name (§10 V38c).
//
// ⚠ Deliberately permissive: this is a NAME the operator reads off their own media server
// ("Commercials", "TV Ads (80s)"), not an identifier with a grammar. Over-validating it would
// reject real libraries for containing a bracket. The scan resolves it against the media server,
// which is the only thing that can actually say whether it exists — an unmatched name surfaces
// there as a scan that finds nothing, with the library named.
func libraryName(raw string) string {
	return strings.TrimSpace(raw)
}

type addFillerSourceOutput struct {
	Body RemoteSourceDTO
}

// addFillerSource registers a remote collection. Downloading is a separate, gated act.
func (s *Server) addFillerSource(ctx context.Context, in *addFillerSourceInput) (*addFillerSourceOutput, error) {
	if s.store == nil {
		return nil, huma.Error501NotImplemented("no store configured")
	}
	kind := strings.TrimSpace(in.Body.Kind)
	// ⚠ `folder` and `library` used to be refused here with a 409 — they were singletons
	// materialised from configuration, guarded by a partial unique index (00029). V38c allows
	// many of each and drops that index (00032), so both the guard and the index are gone
	// together. Leaving the guard would have made the UI's own "Add a source" dialog 409 against
	// two of the three kinds it offers.
	id := normalizeSourceURI(kind, in.Body.URI)
	if id == "" {
		// The message names the shape THIS kind accepts. A shared "invalid URI" would have been
		// the pre-V37 behaviour that told a YouTube user about archive.org collections.
		switch kind {
		case "youtube":
			return nil, errBadRequest("That doesn't look like a YouTube playlist or channel",
				"Paste a playlist URL (youtube.com/playlist?list=…) or a channel URL.")
		case "folder":
			return nil, errBadRequest("That doesn't look like a folder Loomarr can watch",
				"Give a full path starting with / — for example /data/filler. "+
					"A relative path would mean different directories depending on how Loomarr was started.")
		case "library":
			return nil, errBadRequest("That library needs a name",
				"Use the library's name as it appears on your media server — for example Commercials.")
		default:
			return nil, errBadRequest("That doesn't look like an archive.org collection",
				"Paste a collection identifier (like classic_tv_commercials) or its archive.org URL.")
		}
	}
	label := strings.TrimSpace(in.Body.Label)
	if label == "" {
		label = id
	}
	// ⚠ The row id is namespaced by kind. Before V37 every row was an archive collection, so the
	// bare identifier was unique; now a YouTube URL and an archive slug share one table, and an
	// un-namespaced id lets one kind's row silently UPSERT over another's.
	rowID := kind + ":" + id
	// NewFillerSource, never a struct literal: `Enabled` is a bool, so a literal that omits it
	// registers the collection SWITCHED OFF (see the store).
	src := store.NewFillerSource(rowID, kind, id, label, time.Now().UTC())
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
		// FetchEverySeconds overrides `filler.fetch.every` for THIS source (§10 V38c).
		//
		// ⚠ **A POINTER, and the three states are all distinct and all reachable:**
		//   - omitted / `null` ⇒ clear the override, inherit the global
		//   - `0`              ⇒ NEVER auto-fetch this source
		//   - `n > 0`          ⇒ poll every n seconds
		//
		// A plain int could not express this: `0` is already meaningful ("never"), so "unset"
		// would have to share an encoding with it and every source would read as switched off.
		// This mirrors the store column, which is nullable for exactly the same reason.
		FetchEverySeconds *int `json:"fetchEverySeconds,omitempty" minimum:"0" maximum:"604800" doc:"Seconds between automatic fetches of this source. 0 means never; omit or null to inherit the global setting."`
		// FetchMaxPerRun overrides `filler.fetch.max_per_run` for this source. Omit/null inherits.
		//
		// ⚠ Minimum 1, NOT 0. "Fetch nothing per run" is what FetchEverySeconds=0 already says,
		// and letting it be said twice invites the two to disagree — a source that is scheduled
		// to poll but capped at nothing looks enabled and does nothing.
		FetchMaxPerRun *int `json:"fetchMaxPerRun,omitempty" minimum:"1" maximum:"1000" doc:"Most clips to take from this source in one run. Omit or null to inherit the global setting."`
	}
}

type setFillerSourceEnabledOutput struct {
	Body struct {
		ID      string `json:"id"`
		Enabled bool   `json:"enabled"`
		// Echoed back so the UI renders what was STORED rather than what it hoped it sent — the
		// difference matters here because null and 0 mean different things and a client that
		// muddles them would show "never fetch" as "inherit".
		FetchEverySeconds *int `json:"fetchEverySeconds,omitempty"`
		FetchMaxPerRun    *int `json:"fetchMaxPerRun,omitempty"`
	}
}

// setFillerSourceEnabled flips a source's switch.
//
// ⚠ The dispatch below is the one place the storage asymmetry surfaces, and it is deliberately
// here rather than in the client: `folder` is derived from configuration so its switch is a
// SETTING, while a collection is a row so its switch is a COLUMN. A UI that had to know which
// is which would encode a persistence decision in a button handler.
func (s *Server) setFillerSourceEnabled(ctx context.Context, in *setFillerSourceEnabledInput) (*setFillerSourceEnabledOutput, error) {
	// ⚠ A derived GROUP node carries no state, so there is nothing here to write (§10 V51c). One
	// prefix test rather than a case per provider: the ids are namespaced precisely so a single
	// guard covers every group that exists now and every one added later.
	if isProviderID(in.ID) {
		return nil, errConflict("That source has no switch",
			"This row groups the sources you've added — switch them off individually.")
	}

	out := &setFillerSourceEnabledOutput{}
	out.Body.ID, out.Body.Enabled = in.ID, in.Body.Enabled
	out.Body.FetchEverySeconds, out.Body.FetchMaxPerRun = in.Body.FetchEverySeconds, in.Body.FetchMaxPerRun

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
	// ⚠ `case "remote"` was here and is gone (§10 V51c). It guarded the V37-retired container —
	// an id nothing could produce for two phases, so it read as protection while protecting
	// nothing, exactly what the DELETE handler's own comment says about removing its twin. The
	// prefix guard above now covers the ids that DO exist, and inherits the copy verbatim,
	// because the sentence was right all along: the row groups sources, and the switches are on
	// the children.
	//
	// ⚠ `library` used to be refused here too, on the grounds that nothing scanned a
	// media-server library. V38c reverses that (§10) — a library source is scanned like any
	// other, so it has a real switch and lands in the default branch below.
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
		// ⚠ The overrides are written UNCONDITIONALLY, including when both are nil — because nil
		// means "clear this back to inheriting the global", which is a real action an operator
		// takes and must be expressible. Writing only when non-nil would make the override a
		// one-way door: settable, never removable.
		if err := s.store.SetFillerSourceFetchPolicy(ctx, in.ID,
			in.Body.FetchEverySeconds, in.Body.FetchMaxPerRun); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, errNotFound("Source not found", "That source isn't registered — it may have been removed.")
			}
			return nil, huma.Error500InternalServerError("set filler source fetch policy", err)
		}
		return out, nil
	}
}

type deleteFillerSourceInput struct {
	ID string `path:"id"`
}

// deleteFillerSource forgets a registered remote. Its clips stay.
func (s *Server) deleteFillerSource(ctx context.Context, in *deleteFillerSourceInput) (*struct{}, error) {
	if s.store == nil {
		return nil, huma.Error501NotImplemented("no store configured")
	}
	// ⚠ A derived GROUP node is not a registration either (§10 V51c). Deleting Archive.org would
	// have to mean deleting every collection beneath it — a bulk act an operator should perform
	// deliberately, one row at a time, seeing exactly what goes.
	if isProviderID(in.ID) {
		return nil, errConflict("That source can't be removed here",
			"This row groups the sources you've added — remove them individually.")
	}
	switch in.ID {
	case "folder", "library":
		// ⚠ The config-backed SINGLETONS are not registrations, so there is nothing to
		// un-register. Deleting one would have to mean "unset filler.dir", which belongs in
		// Settings where the consequence is legible — and since migration 00029 materialises
		// these rows, a delete here would be undone by the next migrate anyway.
		//
		// ⚠ `remote` was in this list until V37 and is deliberately gone: it was the CONTAINER
		// row that archive collections nested under, and the flat list has no container. Leaving
		// it would guard an id nothing can produce, which reads as protection and is noise.
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
	if s.store == nil {
		return nil, huma.Error501NotImplemented("no store configured")
	}
	// Count by provenance. The `source` column is free text written by whatever discovered the
	// clip, so an unrecognized value is possible and must not vanish — see the Total field.
	//
	// ⚠ Counted in SQL. This loaded EVERY column of EVERY clip row to build a map of integers
	// and a total, which on a large catalog was the dominant cost of rendering this page. The
	// filter is still the zero filter, so the default exclusions (removed, held) are unchanged
	// and the numbers mean exactly what they did.
	bySource, err := s.store.CountClipsBySource(ctx, store.ClipFilter{})
	if err != nil {
		return nil, huma.Error500InternalServerError("count clips", err)
	}
	totalClips := 0
	for _, n := range bySource {
		totalClips += n
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

	// The operator-registered sources — archive collections and YouTube playlists — as PEER rows
	// (V37). A read failure is NOT fatal: the two config-backed rows below are the answer to "why
	// is my catalog empty", and losing that whole page because the registry did not load would be
	// a poor trade.
	//
	// ⚠ The SEEDED singletons are skipped here — not the kinds. Migration 00029 materialises one
	// `folder` and one `library` row so the list is complete on a fresh install, but their live
	// state comes from configuration (the folder's path from `filler.dir`, its switch from
	// `filler.source.folder.enabled`), so emitting the stored copy too would list the drop-folder
	// TWICE with the stale one free to disagree with the setting.
	//
	// ⚠ **V38c narrowed this from "skip the kind" to "skip the seeded row".** Many folders and
	// libraries are now addable (§10), and skipping the whole kind would have hidden every one of
	// them — the operator adds a source, gets a 200, and never sees it again. The seeded folder is
	// the one whose URI IS the configured directory; a folder pointing anywhere else is a genuine
	// peer. The seeded library row carries a blank URI (nothing was ever configured into it) and
	// is skipped on that basis, with the provenance row below covering old clips.
	// ⚠ Split by whether the kind ROLLS UP (§10 V51c): ungrouped rows stay top-level peers, and
	// the provider kinds are collected per-provider so the group node can be emitted above its
	// own children in one pre-ordered pass.
	var registered []FillerSourceDTO
	byProvider := map[string][]FillerSourceDTO{}
	if srcs, srcErr := s.store.ListFillerSources(ctx); srcErr != nil {
		s.log.Warn("list filler sources", "err", srcErr)
	} else {
		for _, src := range srcs {
			if src.Kind == "folder" && (!src.Configured() || src.URI == dir) {
				continue // the config-backed drop-folder; rendered from configuration above
			}
			if src.Kind == "library" && !src.Configured() {
				continue // the seeded placeholder, never configured into
			}
			// ⚠ The seeded blank-URI `youtube` row (migration 00034) is skipped for the same
			// reason, and V51c is what makes that correct rather than a loss. It was shaped like
			// a provider root all along — no URI, nothing to fetch — and rendered as a peer with
			// a blank target. The DERIVED group node is now that row, so skipping the stored one
			// removes a duplicate instead of removing YouTube from the list.
			if src.Kind == "youtube" && !src.Configured() {
				continue
			}
			label := src.Label
			if label == "" {
				label = src.URI
			}
			row := FillerSourceDTO{
				ID:         src.ID,
				Kind:       src.Kind,
				Enabled:    src.Enabled,
				Switchable: true,
				Removable:  true,
				Target:     label,
				URI:        src.URI,
				Detail:     sourceDetail(src.Kind, src.URI),
				Count:      bySource[src.Kind],
				Configured: true,
				// ⚠ SCANNED sources are refreshed by the sync, DOWNLOADED ones by ingest — and
				// both are reachable through this row's "Fetch now", so both report fetchable.
				// A scanned folder whose button did nothing would read as broken; the store's
				// `Fetchable()`/`Scannable()` pair is what keeps a folder out of a PULL plan,
				// which is the distinction that actually matters.
				Fetchable: canFetchRow(src, s.filler != nil),
				// ⚠ Only archive can be searched in place. A "search" box on a YouTube playlist
				// would have nothing to query: yt-dlp enumerates a playlist, it does not search
				// YouTube, and offering the box would be a control that returns nothing forever.
				// A folder or library is not searchable for the same reason.
				Searchable: src.Kind == "archive" && s.filler != nil,
			}
			// ⚠ Counted by SOURCE ID for the addable kinds, not by kind. Two watched folders both
			// count `bySource["folder"]`, so a kind-keyed count would show each of them the
			// catalog's whole folder total — every row claiming every clip.
			if src.Scannable() {
				row.Count = bySource[src.ID]
			}
			if !src.LastFetchedAt.IsZero() {
				row.LastFetchedAt = src.LastFetchedAt.UTC().Format(time.RFC3339)
			}
			// ⚠ Sent through UNCHANGED, including empty. Empty means UNKNOWN and the client
			// renders nothing — never a reassuring default. Substituting "public domain" here
			// would have Loomarr asserting a legal fact about ~92% of archive.org items that
			// declare no licence at all.
			row.License = src.License
			// ⚠ The parent is DERIVED from the kind here, at the one place a row is built. Any
			// other arrangement — a stored column, a second pass that assigns parents — is a
			// place the parent could disagree with the kind.
			if parent, grouped := providerFor(src.Kind); grouped {
				row.ParentID = parent
				byProvider[src.Kind] = append(byProvider[src.Kind], row)
				continue
			}
			registered = append(registered, row)
		}
	}

	out := &fillerSourcesOutput{}
	out.Body.Total = totalClips
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
	}
	// The legacy media-server PROVENANCE row (§10). ⚠ Shown only when clips actually carry that
	// source, which is why it is appended conditionally rather than declared above.
	//
	// It describes where OLD clips came from — those catalogued under the pre-§9.1 model — and it
	// is not a source doing work: `enabled:true` says "these clips count", not "a scan is
	// running". V38c restored library SCANNING (§10), but a scanned library is an operator-added
	// row appended below with its own target, switch and counts. Rendering this one unconditionally
	// alongside those would put two different things called "library" in one list, one of them
	// inert — which is the competing-descriptions problem this file's header exists to prevent.
	if n := bySource["library"]; n > 0 {
		out.Body.Sources = append(out.Body.Sources, FillerSourceDTO{
			ID:         "library",
			Kind:       "library",
			Enabled:    true,
			Switchable: false, // provenance, not a scan — there is no work to stop
			Target:     "media server (previously scanned)",
			Detail:     "where these clips originally came from",
			Count:      n,
			Configured: true,
		})
	}
	// ⚠ The `remote` CONTAINER row was here and is deliberately gone (V37). It existed to hold
	// the nested collections; the flat list has no container, so each registered source is a peer
	// appended below. Its clip count is not lost — each peer counts its own kind, and clips whose
	// provenance is the older `ingest` string are counted by the folder row they physically live
	// in. Keeping an empty container would show a source that nothing can be.
	out.Body.Sources = append(out.Body.Sources, registered...)
	// The provider roll-up (§10 V51c), appended last and PRE-ORDERED: each group node immediately
	// followed by its own children.
	//
	// ⚠ **Flat and pre-ordered, not nested.** A nested `children: []` would generate a recursive
	// type through orval, which it handles badly, and the frontend has no tree primitive — a
	// twirl-down renders from exactly this shape by hiding rows whose parent is collapsed. It is
	// also purely ADDITIVE, so a client that knows nothing about `group`/`parentId` renders the
	// same flat list it always did rather than breaking.
	for _, g := range providerGroups {
		out.Body.Sources = append(out.Body.Sources, providerNode(g.id, g.kind, g.label, g.detail, byProvider[g.kind]))
		out.Body.Sources = append(out.Body.Sources, byProvider[g.kind]...)
	}
	return out, nil
}

// providerFor maps a source kind to its group id, and reports whether that kind rolls up at all.
func providerFor(kind string) (string, bool) {
	for _, g := range providerGroups {
		if g.kind == kind {
			return g.id, true
		}
	}
	return "", false
}

// providerNode builds one derived group row from the children beneath it.
//
// ⚠ **Emitted even with ZERO children**, which is what keeps the top-level row count stable when
// an operator deletes their last collection: the service does not vanish from the page, it becomes
// an empty state inviting them to add another. It is also the answer to "where did Archive.org
// go?" — the same reasoning that keeps the unconfigured drop-folder row visible.
func providerNode(id, kind, label, detail string, children []FillerSourceDTO) FillerSourceDTO {
	node := FillerSourceDTO{
		ID: id, Kind: kind, Group: true,
		Target: label, Detail: detail,
		// ⚠ **No group switch, and this is the opinionated call of the phase.** Cascade-on-write
		// destroys each child's own choice, which the store forbids in as many words ("Disabling
		// is not deleting… switching it back on restores what was there"). A computed
		// `effective = parent && child` is worse: it adds a fifth thing four call sites must all
		// remember, and the direction it fails is *fetching from a provider the operator switched
		// off*. The row reports what its children are doing and offers no lever; if a master
		// switch is ever wanted, it ships as a VISIBLE bulk write over the children.
		Switchable: false,
		// Not removable: there is no registration to forget. Deleting Archive.org would have to
		// mean deleting every collection under it, which is a bulk act an operator should perform
		// deliberately, one row at a time, seeing what goes.
		Removable: false,
		// Nothing to fetch or search AT the provider: a group has no URI. Its children each carry
		// their own "Fetch now" and, for archive, their own search box.
		Fetchable:  false,
		Searchable: false,
		// ⚠ **Configured tracks whether anything is BEHIND it**, which is the distinction
		// `Configured()` was extracted for: a provider with no children is an invitation, not a
		// fault, and the tab renders those differently.
		Configured: len(children) > 0,
	}
	// ⚠ Enabled is a REPORT, not a control: true when any child is doing work, so a provider
	// whose collections are all switched off reads as dormant rather than as running. The "2 of 3
	// on" summary is the frontend's to render — it already has every child row in this same array,
	// so sending a count would be a second copy of a number that could disagree with the rows
	// beside it.
	//
	// The clip count is the honest SUM of what the children claim. ⚠ It is not an estimate and
	// never invents attribution: `sync.go` writes `filler-dir` for everything the folder scan
	// finds and nothing records which SOURCE a downloaded clip came from, so most children
	// legitimately report 0 — and the group reports 0 too, rather than a plausible-looking total.
	for _, c := range children {
		if c.Enabled {
			node.Enabled = true
		}
		node.Count += c.Count
		// LastFetchedAt is a read-only MAX over the children, computed HERE so no column can
		// disagree with it. Absent when no child has ever fetched, so the row renders "never"
		// rather than an epoch date nobody meant.
		//
		// ⚠ Compared as STRINGS, which is correct only because every one of them was formatted
		// by the same `.UTC().Format(time.RFC3339)` two hundred lines up: fixed width, fixed
		// offset, so lexical order is chronological order. A local-time or variable-offset
		// timestamp would break that silently, which is why the formatting has one home.
		if c.LastFetchedAt > node.LastFetchedAt {
			node.LastFetchedAt = c.LastFetchedAt
		}
	}
	return node
}

// sourceDetail is the operator-facing sentence under a registered source's name. Per KIND rather
// than per row: it explains how that sort of source BEHAVES, which is a property of the kind.
func sourceDetail(kind, uri string) string {
	switch kind {
	case "archive":
		return "an archive.org collection — searchable here, downloaded when you queue or approve"
	case "youtube":
		// ⚠ Names the operator's own act. §10 records that Loomarr never recommends YouTube
		// content itself; the playlist is one the operator supplied, and the copy should not
		// imply Loomarr chose it.
		return "a playlist you added — titles and descriptions are kept for tagging"
	default:
		return uri
	}
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
// youtubeTarget validates a YouTube playlist or channel URL for REGISTRATION (V37), returning
// the canonical URL to store or "" if it is not one.
//
// ⚠ Deliberately STRICTER than `clipfetch.KindForURL`, which defaults an unknown host to YouTube
// because yt-dlp handles the widest set of sites. That default is right for INGEST — an admin
// pasting one URL deliberately, watching it work. It is wrong here: a registered source persists
// and is fetched unattended, so a typo'd host would become a permanent row that can never fetch,
// with the failure surfacing much later and far from the mistake. Registration is the stricter
// act because nobody is watching the second time.
//
// Accepts the shapes an operator actually pastes — a playlist, a channel, a @handle, a /c/ or
// /user/ URL — on youtube.com (with or without a subdomain) or youtu.be.
func youtubeTarget(raw string) string {
	v := strings.TrimSpace(raw)
	if v == "" {
		return ""
	}
	// Scheme is optional in what people paste; normalise so the host check is uniform.
	probe := v
	if !strings.Contains(probe, "://") {
		probe = "https://" + probe
	}
	u, err := url.Parse(probe)
	if err != nil {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	host = strings.TrimPrefix(host, "www.")
	// ⚠ Exact host match, never `strings.Contains`: "youtube.com.evil.test" contains the string
	// and is a different site. A substring check here would register an attacker-chosen host as
	// a trusted source.
	if host != "youtube.com" && host != "m.youtube.com" && host != "music.youtube.com" && host != "youtu.be" {
		return ""
	}
	// A bare host with no path identifies nothing to fetch.
	if strings.Trim(u.Path, "/") == "" && u.RawQuery == "" {
		return ""
	}
	u.Scheme = "https"
	return u.String()
}

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
