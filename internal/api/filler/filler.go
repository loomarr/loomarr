package filler

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/api/httpcore"
	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/filler/acquisition"
	"github.com/loomarr/loomarr/internal/filler/asset"
	"github.com/loomarr/loomarr/internal/filler/cliplibrary"
	"github.com/loomarr/loomarr/internal/filler/clips"
	"github.com/loomarr/loomarr/internal/filler/enrichment"

	"github.com/loomarr/loomarr/internal/filler/operator"
	"github.com/loomarr/loomarr/internal/filler/pipelinestate"
	"github.com/loomarr/loomarr/internal/filler/pod"

	"github.com/loomarr/loomarr/internal/filler/taxonomy"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
)

// registerFiller mounts /v1/filler* (§7/§10). List is visible to any
// authenticated user; tag edit, sync, and the AI-tagging job require admin
// (filler ingestion is an admin concern, §7).
func (s *Handlers) registerFiller(api huma.API) {
	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "list-filler", Method: http.MethodGet, Path: "/v1/filler",
		Summary: "List filler clips",
		Description: "Filter by kind/era/audience/category, search across name/brand/on-screen-text/tags, sort, and page (§10 V51d). " +
			"⚠ **Paged by default — `limit` is 100 and caps at 500.** The unbounded read this replaced hard-fails past ~65k clips on " +
			"Postgres (one bind parameter per clip in the tag batch, against a 65535-parameter statement cap). `total` is the number " +
			"MATCHING THE FILTER, counted in SQL through the same predicate as the rows, so a pager can say 'showing 61-120 of 1,204' " +
			"without the two disagreeing. " +
			"Held clips and composites are excluded unless asked for, because neither is in the playable catalog.",
		Tags: []string{"filler"},
	}, httpcore.RoleMember), s.listFiller)

	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "filler-pool", Method: http.MethodGet, Path: "/v1/filler/pool",
		Summary: "Catalog-wide filler health",
		Description: "How well the catalog can fill breaks across the whole install (§10 V35) — the Filler page's pool strip. " +
			"Counts what exists (clips, commercials, duration-eligible, untagged) and lists every live channel's coverage, WORST FIRST. " +
			"The per-channel answers are the SAME `Coverage` computation `/v1/channels/{id}/filler/coverage` returns, called once per channel, " +
			"so this page and the channel page cannot disagree — there is no aggregate ladder to drift from the real one. " +
			"Read-only, so any authenticated user may call it.",
		Tags: []string{"filler"},
	}, httpcore.RoleMember), s.fillerPool)

	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "filler-readiness", Method: http.MethodGet, Path: "/v1/filler/readiness",
		Summary:     "Filler readiness and next action",
		Description: "One server-owned operational projection across acquisition limits, durable runs, pipeline ownership, playable inventory, and worst-first channel coverage (§10 V59). Clients render the typed next action rather than reconstructing priorities from counters.",
		Tags:        []string{"filler"},
	}, httpcore.RoleMember), s.fillerReadiness)

	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "preview-filler-storage-cleanup", Method: http.MethodGet, Path: "/v1/filler/storage/cleanup",
		Summary:     "Preview disposable filler staging",
		Description: "Admin only. Counts terminal private download staging older than the safety grace that no recovery evidence still needs. It never selects catalog clips, repair evidence, symlinks, or unknown files.",
		Tags:        []string{"filler"},
	}, httpcore.RoleAdmin), s.previewFillerStorageCleanup)

	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "cleanup-filler-storage", Method: http.MethodPost, Path: "/v1/filler/storage/cleanup",
		Summary:     "Remove disposable filler staging",
		Description: "Admin only. Revalidates and removes only the private terminal download staging returned by the preview contract, then reports a fresh remaining preview.",
		Tags:        []string{"filler"},
	}, httpcore.RoleAdmin), s.cleanupFillerStorage)

	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "get-filler-acquisition", Method: http.MethodGet, Path: "/v1/filler/acquisitions/{jobId}",
		Summary:     "Read one clip download",
		Description: "Admin only (§10). Durable reconnect state for one filler acquisition; SSE only reduces visible latency.",
		Tags:        []string{"filler"},
	}, httpcore.RoleAdmin), s.getFillerAcquisition)

	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "tag-filler-clip", Method: http.MethodPatch, Path: "/v1/filler/tags",
		Summary: "Edit a clip's classification", Description: "Admin only. Corrects name, kind, era, audience, brand, and directly asserted taxonomy tags. The clip is identified by content `hash` in the body (§10 V45a). A name is the operator's curated name for viewers (#1990).", Tags: []string{"filler"},
	}, httpcore.RoleAdmin), s.patchFillerClip)

	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "sync-filler", Method: http.MethodPost, Path: "/v1/filler/sync",
		Summary: "Sync the clip catalog from the media server", Description: "Admin only (§10).",
		Tags: []string{"filler"},
	}, httpcore.RoleAdmin), s.syncFiller)

	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "rewind-filler-clip", Method: http.MethodPost, Path: "/v1/filler/rewind",
		Summary: "Re-run a clip's ingest pipeline from one stage",
		Description: "Admin only. Resets the selected stage and every later stage, durably forces the selected stage to run, and invalidates only derived data safe for that stage to replace. " +
			"Re-running transcode requires force because the original bytes are no longer available and another encode loses quality.",
		Tags: []string{"filler"},
	}, httpcore.RoleAdmin), s.rewindFillerClip)

	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "ingest-filler", Method: http.MethodPost, Path: "/v1/filler/ingest",
		Summary:     "Download clips into the drop-folder (admin; needs the vendored yt-dlp + ffmpeg)",
		Description: httpcore.OperatorAPI,
		Tags:        []string{"filler"},
	}, httpcore.RoleAdmin), s.ingestFiller)

	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "discover-filler", Method: http.MethodGet, Path: "/v1/filler/discover",
		Summary: "Search archive.org for clips to add",
		Description: "Admin only (§10, V33). Searches archive.org by keyword and returns candidates " +
			"WITHOUT downloading anything — the operator is deciding what to fetch, and making them " +
			"download to find out would defeat the point. Results carry only what the source's search " +
			"index knows: no tags, no licence (see below), and no duration or quality — those cost a " +
			"metadata call per row, so ask GET /v1/filler/discover/stats for the rows you are showing. " +
			"Feed an id to POST /v1/filler/ingest to actually fetch it. Synchronous: one upstream " +
			"request, no job to poll.",
		Tags: []string{"filler"},
	}, httpcore.RoleAdmin), s.discoverFiller)

	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "discover-filler-stats", Method: http.MethodGet, Path: "/v1/filler/discover/stats",
		Summary: "Runtime and quality for specific search results",
		Description: "Admin only (§10, V35). Fills in the duration and quality a search deliberately " +
			"omits. ⚠ This is ONE UPSTREAM CALL PER ID and archive.org is slow (median 1.8s, and it " +
			"gets slower if asked for more at once), so send only the ids currently on screen — a " +
			"full page of 25 measured 22.6s. An id archive.org cannot answer for is ABSENT from the " +
			"response rather than zero: 0 would be indistinguishable from a genuinely empty clip.",
		Tags: []string{"filler"},
	}, httpcore.RoleAdmin), s.discoverFillerStats)

	huma.Register(api, httpcore.WithRole(huma.Operation{
		// ⚠ The clip is a QUERY parameter, not a path segment, and that is forced rather than
		// stylistic: `/v1/filler/{id}/fit` collides with every literal segment under /v1/filler.
		// Go's ServeMux refuses to register it beside `/v1/filler/splits/{proposalId}` ("both match
		// some paths, like /v1/filler/splits/fit"), and it panics at ROUTE REGISTRATION, i.e. at
		// boot.
		OperationID: "clip-channel-fit", Method: http.MethodGet, Path: "/v1/filler/fit",
		Summary: "Where this clip lands on each channel's ladder",
		Description: "The override picker's per-channel note (§10 V35). For every channel: which rung " +
			"of the §10 ladder this clip would be drawn from, or WHY it would never be picked, plus the " +
			"operator's pin/exclude state. Computed from the SAME predicates assembly uses, through the " +
			"same per-channel selection as the pod preview — a note that disagrees with what airs is a " +
			"confident wrong answer about why a channel looks the way it does. Read-only. ⚠ A PINNED clip " +
			"reports no rung and no reason: a pin is placed ahead of the ladder, so it has none. ⚠ Every " +
			"channel is listed, including paused ones — pinning to a paused channel is a decision that " +
			"takes effect when it resumes.",
		Tags: []string{"filler", "channels"},
	}, httpcore.RoleMember), s.clipChannelFit)

	// Compilation splitting (§10, V34). Propose is a job (detection runs minutes
	// per file); the proposal read and the confirm are synchronous. All admin:
	// these routes write the catalog, which is the same trust level as ingest.
	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "split-filler", Method: http.MethodPost, Path: "/v1/filler/split",
		Summary: "Propose splits for a compilation clip",
		Description: "Admin only (§10, V34). The clip is identified by `path` in the body (§10 V45a). Runs " +
			"detection — chapters → blackdetect/silencedetect → transcript rescue for over-long segments — " +
			"as a background job (progress on /v1/events as filler_split frames), producing a PERSISTED " +
			"split proposal. Nothing enters the catalog: review is not optional, because detection quality " +
			"is a property of the source.",
		Tags: []string{"filler"},
	}, httpcore.RoleAdmin), s.splitFiller)

	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "get-filler-split", Method: http.MethodGet, Path: "/v1/filler/splits/{proposalId}",
		Summary: "Read a split proposal",
		Description: "Admin only (§10, V34). The review surface's source of truth on SSE reconnect — " +
			"the same pattern as /v1/proposals/{id}.",
		Tags: []string{"filler"},
	}, httpcore.RoleAdmin), s.getFillerSplit)

	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "confirm-filler-split", Method: http.MethodPost, Path: "/v1/filler/splits/{proposalId}/confirm",
		Summary: "Commit a reviewed split",
		Description: "Admin only (§10, V34). The body is the operator's confirmed cut list — the " +
			"proposal as returned, possibly edited (cuts moved/merged/dropped; era suggestions accepted " +
			"or rejected; dedup-flagged segments kept or skipped). Only now do segments become catalog " +
			"clips: cut with ffmpeg stream copy, and the original compilation row removed — its identity " +
			"is a path that now means twenty clips, not one.",
		Tags: []string{"filler"},
	}, httpcore.RoleAdmin), s.confirmFillerSplit)

	// The clip byte route. Member-visible, matching `list-filler`: the catalog listing is
	// visible to any authenticated user, and an asset is strictly less information than the row
	// it decorates. NOT public like the channel icon — that one is open so Tunarr can fetch it
	// machine-to-machine, a reason that does not apply here.
	//
	// ⚠ `media`, never `preview`: /v1/channels/{id}/filler/coverage and pod.Adapter.Preview
	// already mean "the pod pool a channel would get", as JSON.
	//
	// ⚠ **Only the clip's OWN bytes are served here.** A clip's still and hover loop are
	// image-service images (§22): content-addressed, with a srcset and a ThumbHash, served by
	// /v1/images/{hash} under an honest immutable cache. A second route serving them from a second
	// store would be the duplication §22 exists to remove; the clip file is not an image and the
	// image service has no opinion about it.
	//
	// It keeps the stdlib (w, r) signature because http.ServeContent answers Range,
	// If-Modified-Since and 206 in one call — see httpcore.RawOp.
	//
	// ⚠ Media type ranges, not a single type: this route serves whatever the clip's extension
	// maps to in the `mediaTypes` allowlist (video/mp4, video/x-matroska, audio/mpeg, …). Naming
	// one of them would be a documented lie, and enumerating eleven says less than the range does.
	httpcore.RawOp[httpcore.HashInput](api, httpcore.BytesResponse(huma.Operation{
		OperationID: "filler-media", Method: http.MethodGet, Path: "/v1/filler/media/{hash}",
		Summary: "A clip's own bytes",
		Description: "Any authenticated user. The clip itself, so an operator can watch one before " +
			"deciding about it. Range-capable, so a <video> element can seek. The exact media type " +
			"comes from a short server-side allowlist keyed by extension.",
		Tags: []string{"filler"},
	}, "The clip's own bytes.", "video/*", "audio/*"), httpcore.RoleMember, s.serveFillerMedia)
}

type rewindFillerClipInput struct {
	Body struct {
		Hash  string `json:"hash" minLength:"1"`
		From  string `json:"from" enum:"probe,transcode,split,screen,defects,language,transcribe,vision,finish"`
		Force bool   `json:"force,omitempty"`
	}
}

func (s *Handlers) rewindFillerClip(ctx context.Context, in *rewindFillerClipInput) (*struct{}, error) {
	rewinder, ok := s.filler.(Rewinder)
	if !ok {
		return nil, huma.Error501NotImplemented("the filler pipeline is not configured")
	}
	if _, err := s.store.GetClip(ctx, in.Body.Hash); errors.Is(err, store.ErrNotFound) {
		return nil, httpcore.NotFound("Clip not found", "That clip is no longer in the catalog.")
	} else if err != nil {
		return nil, err
	}
	err := rewinder.Rewind(ctx, in.Body.Hash, pipelinestate.StageID(in.Body.From), in.Body.Force)
	switch {
	case errors.Is(err, filler.ErrUnknownStage):
		return nil, httpcore.Unprocessable("Unknown pipeline stage", "Choose a stage from the clip's pipeline ladder.")
	case errors.Is(err, filler.ErrTranscodeNeedsForce):
		return nil, httpcore.Conflict("Re-encoding needs confirmation", err.Error())
	case errors.Is(err, filler.ErrFilesDeleted):
		return nil, httpcore.Conflict("This clip's files are gone", err.Error())
	case err != nil:
		return nil, err
	default:
		return &struct{}{}, nil
	}
}

func fillerSourceLabel(source string, labels map[string]string) string {
	if label := strings.TrimSpace(labels[source]); label != "" {
		return label
	}
	switch source {
	case "filler-dir", "folder":
		return "Your clip folder"
	case "library":
		return "Media server library"
	case "manual":
		return "Manually added"
	case acquisition.OpenSearchSourceID:
		return "Search for nearby stations"
	case "":
		return ""
	default:
		return "Imported source"
	}
}

type listFillerInput struct {
	Role            string `query:"role" enum:"unclassified,commercial,bumper,station_id,psa,trailer,interstitial,political,news"`
	Era             int    `query:"era"`
	Audience        string `query:"audience" enum:"kids,family,general,late_night"`
	Category        string `query:"category"`
	GeographicScope string `query:"geographicScope" enum:"unknown,national,local"`
	Country         string `query:"country" minLength:"2" maxLength:"2"`
	Taxon           string `query:"taxon" doc:"Match clips carrying this taxon directly or through a descendant rollup"`
	Unclassified    bool   `query:"unclassified" doc:"Only playable clips with no directly asserted taxonomy tags on any axis"`
	WithoutAxis     string `query:"withoutAxis" enum:"product,format,seasonal,audience-cue,presentation" doc:"Only playable clips without a directly asserted taxonomy tag on this axis; absence may be valid for sparse cue axes"`
	Untagged        bool   `query:"untagged" doc:"Only commercials missing match tags"`
	// Q is the clip corpus's search box (§7.2). Clip search lives here rather than on
	// /v1/search because a clip is not a provisionable title (§10) and cannot be a
	// federated Candidate without leaking a non-title into the LLM grounding path.
	Q string `query:"q" doc:"Case-insensitive substring match across name, brand, on-screen text and tags (§10 V51d)"`
	// SearchTranscript widens Q to the persisted transcript (§10 V51d) — opt-in because it is the
	// one long column (megabytes over a 500-row page) and the one noisy one ("ford" matches
	// "afford", with no ranking to explain the hit).
	SearchTranscript bool `query:"searchTranscript" doc:"Also search the transcript. Opt-in: it is kilobytes per clip and matches inside words."`
	// Limit / Offset page the catalog (§10 V51d).
	//
	// ⚠ **The default lives HERE, never in the store.** `ClipFilter.Limit == 0` means no LIMIT
	// clause because pod assembly loads the catalog through the zero filter — a store-side default
	// would silently cut every channel's break pool to the first hundred clips.
	//
	// ⚠ `minimum:"1"` matters as much as the default: it stops a client sending `limit=0` and
	// reaching the store's "unbounded" sentinel by accident.
	Limit  int `query:"limit" minimum:"1" maximum:"500" default:"100" doc:"Page size (§10 V51d). Capped at 500 — the unbounded read this replaced hard-fails past ~65k clips on Postgres."`
	Offset int `query:"offset" minimum:"0" doc:"Rows to skip. Use with total to render 'showing 61-120 of 1,204'."`
	// Sort / Order. ⚠ A closed enum, mapped to a column by a fixed switch in the store; an unknown
	// value is rejected rather than silently ignored.
	Sort  string `query:"sort" enum:"name,duration,added,plays" doc:"Sort key; default is the historical path order. Every ordering is made total by a hash tie-break (§10 V51d)."`
	Order string `query:"order" enum:"asc,desc" doc:"Sort direction; default ascending."`
	// IncludeHeld / HeldOnly surface the review queue's clips (§10 V38).
	//
	// ⚠ Opt-in on the wire exactly as in the store, and for the same reason: a held clip is not in
	// the playable catalog. `ClipDTO.held` ships with this so a client can tell them apart.
	IncludeHeld bool `query:"includeHeld" doc:"Include clips awaiting review (§10 V38). They are excluded by default because they are not in the playable catalog."`
	// IncludeComposites / TopLevel are how the catalog listing renders breaks as CONTAINERS.
	//
	// ⚠ Both opt-in, and NOT defaults for this route, because /v1/filler is also the add-search
	// behind the channel pin/exclude editor and the ⌘K palette. Those want AIRABLE clips —
	// segments, not the 16-minute break they came out of — so flipping either default would put a
	// non-airable container into a channel's pin list.
	IncludeComposites bool `query:"includeComposites" doc:"Include composites — recorded breaks, which are NOT airable (§10 V45). The catalog listing pairs this with topLevel to render them as containers."`
	TopLevel          bool `query:"topLevel" doc:"Only clips with no parent, so a break paginates as one container row instead of the twenty adverts inside it (§10 V51d)."`
	// ParentHash loads the SEGMENTS of one composite — what expanding a break asks for.
	ParentHash string `query:"parentHash" doc:"Load the segments split out of this composite (§10 V45). Takes precedence over topLevel, which it contradicts by construction."`
	// Hashes is the batch read: resolve these specific clips.
	//
	// ⚠ It exists because the catalog is paged: mapping N saved ids to names by loading the
	// catalog would be truncated at a 100-row page. Hashes resolves exactly these clip hashes —
	// the batch read behind "name these N pinned clips".
	//
	// ⚠ **`explode`, and it is load-bearing.** Huma turns explode OFF by default for query fields
	// ("parsing is *much* easier if we use comma-separated values"), which means `?hashes=a&hashes=b`
	// binds ONE element — the rest are silently dropped and the request still returns 200. The
	// generated TypeScript client sends exactly that repeated form, so without the tag every pinned
	// clip after the first fails to resolve and the channel page reports real clips as "no longer
	// in your catalog". Neither side's tests can see it if the Go tests hand-write `?hashes=a,b`
	// against a client generated from a spec that agrees with them.
	Hashes []string `query:"hashes,explode" maxItems:"500" doc:"Resolve exactly these clip hashes (§10 V51d) — the batch read behind 'name these N pinned clips'. Repeat the parameter per hash, at most 500: the page size, so every hash asked for can come back. Unknown hashes are simply absent."`
}
type listFillerOutput struct {
	Body struct {
		Clips []ClipDTO `json:"clips"`
		// Total is how many clips MATCH THE FILTER, not how many are on this page (§10 V51d).
		//
		// ⚠ Counted in SQL through the same WHERE builder the rows come from, so a page's total can
		// never disagree with its contents — including under a search, which widens both together.
		Total int `json:"total" doc:"Total matching the filter, ignoring limit/offset — the 'of 1,204' in a pager (§10 V51d)"`
	}
}

func (s *Handlers) listFiller(ctx context.Context, in *listFillerInput) (*listFillerOutput, error) {
	f := clipstore.ClipFilter{
		Role:                clips.Role(in.Role),
		Era:                 in.Era,
		Audience:            clips.Audience(in.Audience),
		Category:            in.Category,
		GeographicScope:     pod.GeographicScope(in.GeographicScope),
		Country:             in.Country,
		Taxon:               in.Taxon,
		WithoutTaxonomyTags: in.Unclassified,
		WithoutTaxonomyAxis: taxonomy.Axis(in.WithoutAxis),
		UntaggedOnly:        in.Untagged,
		Query:               in.Q,
		QueryTranscript:     in.SearchTranscript,
		IncludeHeld:         in.IncludeHeld,
		IncludeComposites:   in.IncludeComposites,
		TopLevelOnly:        in.TopLevel,
		ParentHash:          in.ParentHash,
		Hashes:              in.Hashes,
		Sort:                clipstore.ClipSort(in.Sort),
		Desc:                in.Order == "desc",
		Limit:               in.Limit,
		Offset:              in.Offset,
	}
	clips, err := s.store.ListClips(ctx, f)
	if err != nil {
		// ⚠ Defence in depth: huma's enum already rejects an unknown sort at validation, so this
		// can only fire if the two vocabularies drift. It is a 422 rather than a 500 because it is
		// the caller's value that is wrong — and never a silent fall-back to the default order.
		if errors.Is(err, clipstore.ErrUnknownClipSort) {
			return nil, huma.Error422UnprocessableEntity("unknown sort", err)
		}
		return nil, err
	}
	// ⚠ Counted with the SAME filter, deliberately including limit/offset — CountClips ignores
	// them by construction (they live outside the WHERE builder), so passing the whole filter is
	// what guarantees the count and the rows share every predicate.
	total, err := s.store.CountClips(ctx, f)
	if err != nil {
		return nil, huma.Error500InternalServerError("count filler clips", err)
	}
	out := &listFillerOutput{}
	out.Body.Total = total
	out.Body.Clips = make([]ClipDTO, 0, len(clips))
	// Resolved for the whole page BEFORE the loop — see clipArtworkResolver on why a lookup
	// inside the loop would be two queries per tile.
	img := s.clipArtworkResolver(ctx, clips)
	sources, err := s.store.ListFillerSources(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("list filler sources", err)
	}
	sourceLabels := make(map[string]string, len(sources))
	for _, source := range sources {
		sourceLabels[source.ID] = source.Label
	}
	stations := s.labelStations(ctx)
	for _, c := range clips {
		d := clipToDTO(c, s.playsCounted(), img)
		d.SourceLabel = fillerSourceLabel(c.Source, sourceLabels)
		d.StationCallSign, d.StationLabel = stationNames(stations, c.Station, c.AirDate)
		if len(in.Hashes) == 1 {
			states, err := s.store.ListFillerEnrichment(ctx, c.Hash)
			if err != nil {
				return nil, httpcore.ProblemWithCause(http.StatusInternalServerError, "Couldn't read clip details", "The clip's background details could not be read. Try again.", err)
			}
			d.Enrichment = clipEnrichmentDTO(states)
			report, err := s.store.LatestFillerResearchReport(ctx, c.Hash)
			if err == nil {
				d.ContextSuggestion = clipContextSuggestionDTO(report, c.Era, c.Country)
			} else if !errors.Is(err, store.ErrNotFound) {
				return nil, httpcore.ProblemWithCause(http.StatusInternalServerError, "Couldn't read likely context", "The clip's likely context could not be read. Try again.", err)
			}
			provenanceClip := c
			if c.ParentHash != "" {
				parent, err := s.store.GetClip(ctx, c.ParentHash)
				if err == nil {
					provenanceClip = parent
				}
			}
			artifact, found, err := s.store.AcquisitionArtifactForClip(ctx, provenanceClip.Path, provenanceClip.Hash)
			if err != nil {
				return nil, httpcore.ProblemWithCause(http.StatusInternalServerError, "Couldn't read clip source", "The recorded source could not be read. Try again.", err)
			}
			// A reused filename does not prove this clip came from the newer acquisition.
			if found && artifact.ClipHash == provenanceClip.Hash {
				if u, err := url.Parse(artifact.SourceURL); err == nil && u.User == nil && u.Host != "" && (u.Scheme == "https" || u.Scheme == "http") {
					d.SourceURL = u.String()
				}
			}
			if files := s.fillerLayout.FS(); files != nil {
				if tags, ok := asset.ReadSidecarTagsFS(files, c.Path); ok {
					d.Media = clipMediaDTO(tags)
				}
			}
		}
		out.Body.Clips = append(out.Body.Clips, d)
	}
	return out, nil
}

type patchClipInput struct {
	Body struct {
		// Hash is the clip's identity (its content hash, V38c). ⚠ In the BODY (§10 V45a): the wire
		// identity is the hash — hex, no slashes — so it needs no encoding and no {id} URL segment.
		// Required.
		Hash string `json:"hash" doc:"The clip's content hash (its identity)"`
		// Name is an operator's rename (#1990). Pointer semantics: omission leaves the name alone,
		// and an empty name is rejected rather than read as "clear", since a clip always has one.
		// Its length is checked after trimming, in the handler, so no schema maxLength.
		Name     *string `json:"name,omitempty" doc:"A name for viewers: 1–120 characters after trimming. Recorded as curated and written to the clip's sidecar, so a rescan keeps it. Omit to leave unchanged; an empty name is rejected (#1990)."`
		Era      int     `json:"era,omitempty"`
		Audience string  `json:"audience,omitempty" enum:"kids,family,general,late_night,"`
		// Tags is the operator's chosen taxonomy tag set (§10 V45a) — REPLACES the old single
		// `category` string. Each is grounded against the live taxonomy on write (an unknown slug is
		// rejected), and `category` is DERIVED from the accepted set (the primary product leaf). A nil
		// Tags means "leave the tags alone"; an explicit empty array clears them.
		Tags *[]string `json:"tags,omitempty" maxItems:"64" doc:"The operator's taxonomy tags (leaf slugs, at most 64). Grounded on write; category is derived. Omit to leave unchanged, send [] to clear (§10 V45a)."`
		// Brand is a grounded advertiser name, deliberately separate from the taxonomy. Pointer
		// semantics distinguish omission (leave it alone) from an explicit empty string (clear a
		// bad model guess). The operator is a valid grounding source.
		Brand *string `json:"brand,omitempty" maxLength:"120" doc:"Grounded advertiser name. Omit to leave unchanged; send an empty string to clear."`
		// Role is correctable by hand (§10). Detection at sync gets it wrong in one
		// direction often enough to matter — a trailer scanned as a commercial — and
		// kind drives pod ROLE (a bumper bookends a pod, a commercial fills it), so a
		// wrong kind yields structurally wrong pods rather than just a mis-tagged clip.
		// Empty means "leave it alone", so a tag-only edit never rewrites kind.
		Role string `json:"role,omitempty" enum:"commercial,bumper,station_id,psa,trailer,interstitial,political,news,"`
		// Geography is a complete, grounded replacement. Omit to leave it unchanged; use scope
		// unknown to clear a bad classification. Operator edits are recorded as their own evidence.
		Geography *ClipGeographyDTO `json:"geography,omitempty"`
	}
}

type ClipGeographyDTO struct {
	Scope   string `json:"scope" enum:"unknown,national,local"`
	Country string `json:"country,omitempty" maxLength:"2"`
	Network string `json:"network,omitempty" maxLength:"120"`
	Station string `json:"station,omitempty" maxLength:"120" doc:"A call sign (WEWS, WABC-TV) or station id; required for a local clip and resolved against the broadcast station directory on the air date"`
	AirDate string `json:"airDate,omitempty" doc:"YYYY-MM-DD when known"`
}
type clipOutput struct{ Body ClipDTO }

func (s *Handlers) patchFillerClip(ctx context.Context, in *patchClipInput) (*clipOutput, error) {
	now := time.Now()
	var operatorLeaves []string
	var operatorForest *taxonomy.Forest
	var operatorGeography *enrichment.Value
	if in.Body.Hash == "" {
		return nil, httpcore.Unprocessable("Missing clip", "A clip tag edit must name the clip by its hash.")
	}
	name := ""
	if in.Body.Name != nil {
		name = strings.TrimSpace(*in.Body.Name)
		if name == "" {
			return nil, httpcore.Unprocessable("Name is empty", "A clip needs a name. Enter one, or leave the name out to keep the current one.")
		}
		if utf8.RuneCountInString(name) > 120 {
			return nil, httpcore.Unprocessable("Name is too long", "A clip name can be at most 120 characters.")
		}
	}
	// ⚠ Identity is the HASH (§10 V45a), so the store lookups (`GetClip`/classification update/
	// `UpdateClipRole`, all keyed `WHERE hash = ?`) take it directly — no path resolution. Fetch once
	// to 404 a missing clip synchronously and to read the pre-edit tags below.
	clip, err := s.store.GetClip(ctx, in.Body.Hash)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpcore.NotFound("Clip not found", "That filler clip doesn't exist — it may have been removed by a catalog sync.")
		}
		return nil, err
	}
	// ⚠ EVERY refusal is decided before the first write. The writes below are separate store calls,
	// so a tag or station rejected after the rename would leave the clip renamed and retagged
	// behind a 422 — the operator would see "not saved" for an edit that had half-saved.
	//
	// Resolve the operator's tag edit against the LIVE taxonomy (§10 V45a). Tags nil ⇒ leave the
	// clip's tags alone (an era/audience/kind-only edit); an explicit [] ⇒ clear them. Each slug is
	// grounded exactly as the tagger grounds the model — an unknown slug is a 422, never silently
	// persisted. The store derives `category` from the accepted set on read, against its current
	// graph; the client never supplies it.
	if in.Body.Tags != nil {
		if operatorLeaves, operatorForest, err = s.groundOperatorTags(ctx, *in.Body.Tags); err != nil {
			return nil, err
		}
	}
	var geography *enrichment.Geography
	if in.Body.Geography != nil {
		g, err := s.resolveOperatorGeography(ctx, *in.Body.Geography)
		if err != nil {
			return nil, err
		}
		geography = &g
	}
	// The rename goes before every other write, so a sidecar that cannot be updated refuses the
	// whole edit rather than saving half of it.
	if name != "" {
		// The sidecar first: a name only the database holds would be lost to the next scan, while a
		// sidecar ahead of its row is healed by that scan. A clip with no file in the drop-folder is
		// never rescanned, so the row alone is enough for it.
		if full, ok := s.clipFileOnDisk(clip.Path); ok {
			if err := cliplibrary.WriteCuratedName(full, name); err != nil {
				return nil, httpcore.ProblemWithCause(http.StatusInternalServerError, "Couldn't save the name",
					"The clip's metadata file could not be updated, so nothing was changed. Try again.", err)
			}
		}
		if err := s.store.RenameClip(ctx, clip.Hash, name, clips.NameFromCurated, now); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, httpcore.NotFound("Clip not found", "That filler clip doesn't exist — it may have been removed by a catalog sync.")
			}
			return nil, err
		}
	}
	if in.Body.Tags != nil {
		if err := s.store.SetClipTags(ctx, clip.Hash, operatorLeaves); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, httpcore.NotFound("Clip not found", "That filler clip doesn't exist — it may have been removed by a catalog sync.")
			}
			if errors.Is(err, clipstore.ErrTaxonConflict) {
				return nil, httpcore.Conflict("Taxonomy changed", "The tag vocabulary changed while this clip was being edited. Review the tags and try again.")
			}
			return nil, err
		}
	}
	// Setting an era makes any era guess moot: the Context suggestion hides its era once the clip
	// has a verified one (clipContextSuggestionDTO).
	if err := s.store.UpdateClipClassification(ctx, clip.Hash, in.Body.Era, in.Body.Audience, now); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, httpcore.NotFound("Clip not found", "That filler clip doesn't exist — it may have been removed by a catalog sync.")
		}
		return nil, err
	}
	// Kind is a separate write because the AI tagging job shares UpdateClipClassification and must
	// never touch kind. Both are idempotent single-row updates, so the same PATCH is
	// safe to retry if the second fails.
	if in.Body.Role != "" {
		if err := s.store.UpdateClipRole(ctx, clip.Hash, in.Body.Role, now); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, httpcore.NotFound("Clip not found", "That filler clip doesn't exist — it may have been removed by a catalog sync.")
			}
			return nil, err
		}
	}
	if geography != nil {
		g := *geography
		if err := s.store.UpdateClipGeography(ctx, clip.Hash, g.Scope, g.Country, g.Network, g.Station, g.AirDate, "operator", now); err != nil {
			return nil, err
		}
		operatorGeography = &enrichment.Value{Geography: g}
	}
	if in.Body.Brand != nil {
		brand := strings.TrimSpace(*in.Body.Brand)
		if err := s.store.SetClipBrand(ctx, clip.Path, brand, now); err != nil {
			return nil, err
		}
	}
	evidence := operatorEvidence{s: s, clipHash: clip.Hash, reference: "operator.clip_edit", at: now}
	if err := evidence.record(ctx, enrichment.AxisEra, enrichment.Value{Year: in.Body.Era}); err != nil {
		return nil, err
	}
	if err := evidence.record(ctx, enrichment.AxisAudience, enrichment.Value{Text: in.Body.Audience}); err != nil {
		return nil, err
	}
	if in.Body.Role != "" {
		if err := evidence.record(ctx, enrichment.AxisRole, enrichment.Value{Text: in.Body.Role}); err != nil {
			return nil, err
		}
	}
	if in.Body.Brand != nil {
		if err := evidence.record(ctx, enrichment.AxisBrand, enrichment.Value{Text: strings.TrimSpace(*in.Body.Brand)}); err != nil {
			return nil, err
		}
	}
	if operatorGeography != nil {
		if err := evidence.record(ctx, enrichment.AxisGeography, *operatorGeography); err != nil {
			return nil, err
		}
	}
	if in.Body.Tags != nil {
		if err := evidence.recordTags(ctx, operatorForest, operatorLeaves); err != nil {
			return nil, err
		}
	}
	c, err := s.store.GetClip(ctx, clip.Hash)
	if err != nil {
		return nil, err
	}
	s.reconcileChannelsForFillerChange(ctx, clip.Clip, c.Clip)
	out := clipToDTO(c, s.playsCounted(), s.clipArtworkResolver(ctx, []clipstore.Clip{c}))
	stations := s.labelStations(ctx)
	out.StationCallSign, out.StationLabel = stationNames(stations, c.Station, c.AirDate)
	return &clipOutput{Body: out}, nil
}

type syncFillerOutput struct {
	Body struct {
		Total   int `json:"total"`
		Added   int `json:"added"`
		Updated int `json:"updated"`
		Pruned  int `json:"pruned"`
	}
}

func (s *Handlers) syncFiller(ctx context.Context, _ *struct{}) (*syncFillerOutput, error) {
	if s.filler == nil || s.featureOff(ctx, "filler") {
		return nil, httpcore.NotImplemented("Filler isn't set up", "Enable filler in Settings to sync a commercial and bumper catalog.")
	}
	total, added, updated, pruned, err := s.filler.Sync(ctx)
	// ⚠ A switched-off drop-folder is not a failure, and must not be reported as one. Left as a
	// 502 it reads "your media server is broken" — sending the operator to check a connection
	// that is fine — when the true answer is a switch they flipped themselves, on this page.
	if errors.Is(err, cliplibrary.ErrSourceDisabled) {
		return nil, SourceDisabledError()
	}
	if err != nil {
		return nil, httpcore.ProblemWithCause(http.StatusBadGateway, "Couldn't sync filler",
			"Loomarr couldn't sync the filler catalog from your media server. Check its connection in Settings and try again.", err)
	}
	out := &syncFillerOutput{}
	out.Body.Total, out.Body.Added, out.Body.Updated, out.Body.Pruned = total, added, updated, pruned
	return out, nil
}

type ingestFillerInput struct {
	Body struct {
		// URLs are supplied per request by an admin rather than configured globally:
		// there is no unattended crawler (§10), so ingestion is always a deliberate act
		// with a person attached to it.
		URLs []string `json:"urls" minItems:"1" maxItems:"50" doc:"YouTube playlist/video or Archive.org collection/item URLs, at most 50 per request (each is a playlist or collection, not a video)"`
	}
}
type ingestFillerOutput struct {
	Body struct {
		JobID string `json:"jobId" doc:"Watch /v1/events for filler_ingest frames carrying this id"`
	}
}

// ingestFiller starts a download job and returns immediately. Downloads run for minutes
// to hours, so the response carries a job id and progress arrives on the SSE bus — the
// same contract as the §8.1 model pull.
func (s *Handlers) ingestFiller(ctx context.Context, in *ingestFillerInput) (*ingestFillerOutput, error) {
	if s.filler == nil {
		return nil, httpcore.NotImplemented("Filler isn't set up", "Enable filler in Settings to sync a commercial and bumper catalog.")
	}
	// ⚠ `IngestAsked`, not `Ingest`: this is the one path where an OPERATOR named the target, so
	// it is the one path that registers a source. Auto-fetch and approved pulls carry the items
	// inside an already-registered collection and must not add a row per clip.
	jobID, err := s.filler.IngestAsked(ctx, in.Body.URLs)
	if errors.Is(err, acquisition.ErrProviderPaused) {
		return nil, httpcore.Conflict("That provider is paused",
			"Resume it under Filler → Sources before downloading from it.")
	}
	if errors.Is(err, operator.ErrIngestUnavailable) {
		// NOT feature_not_configured: no setting can assert that a binary RUNS. The single image
		// always ships the tooling (§16), so unavailable means a degraded install, and the message
		// says what an operator can actually check — never a remedy that is a dead end (§7).
		return nil, httpcore.Conflict("Downloads aren't available here",
			"This install can't run the download tooling. The official image ships it; a custom build may be missing it, or INGEST_YTDLP_PATH may point somewhere wrong.")
	}
	if err != nil {
		return nil, err
	}
	out := &ingestFillerOutput{}
	out.Body.JobID = jobID
	return out, nil
}

// --- discovery (§10, V33) ---

// ChannelFitDTO is one channel's answer for one clip (§10, V35 item 1.7).
type ChannelFitDTO struct {
	ChannelID string `json:"channelId"`
	Name      string `json:"name"`
	Number    int    `json:"number"`
	// Level is the ladder rung this clip would be drawn from on this channel.
	//
	// ⚠ `bumper_card` here means "this clip is not a candidate", NOT "this channel falls back to
	// the card" — the same word carries both meanings because it is the bottom of one ladder.
	// `reason` disambiguates: a rejected clip always carries one, a PINNED clip never does.
	Level string `json:"level" enum:"exact,widened,audience,bumper_card" doc:"The rung this clip would be drawn from; bumper_card means it is not a candidate (see reason)"`
	// Reason names the predicate that rejected the clip, absent when it was not rejected.
	//
	// ⚠ A CODE, never a sentence: the frontend owns the wording (the §11 refusal-code
	// precedent), because a server sentence cannot be translated and cannot link to the setting
	// that caused it.
	Reason string `json:"reason,omitempty" enum:"role,duration,quality,category,audience,geography,excluded,season,language" doc:"Why the clip is not a candidate; absent when it is one"`
	// Pinned / Excluded are the operator's explicit override for this clip on this channel.
	//
	// ⚠ Both can be true, and EXCLUDED WINS — assembly seeds excluded ids into the used-set
	// before pins are placed. Reported as stored rather than normalised, or the picker would
	// show a state the database does not hold.
	Pinned   bool `json:"pinned"`
	Excluded bool `json:"excluded"`
}

type clipFitInput struct {
	// Clip is the clip's content HASH (its identity, §10) — a query parameter because a path
	// segment collides with the literal segments; see the route registration.
	Clip string `query:"clip" required:"true" doc:"The clip's content hash (its identity, §10)"`
}

type clipFitOutput struct {
	Body struct {
		// Channels is EVERY channel, paused ones included — see the route description.
		Channels []ChannelFitDTO `json:"channels"`
	}
}

// clipChannelFit answers "where does this clip land on each channel's ladder" (§10, V35 1.7).
func (s *Handlers) clipChannelFit(ctx context.Context, in *clipFitInput) (*clipFitOutput, error) {
	if s.pods == nil {
		return nil, httpcore.NotImplemented("Filler isn't set up",
			"Set up commercials and filler before choosing which channels use a clip.")
	}

	fits, err := s.pods.ClipFit(ctx, in.Clip)
	if errors.Is(err, store.ErrNotFound) {
		return nil, httpcore.NotFound("Clip not found", "That clip isn't in the catalog — it may have been removed.")
	} else if err != nil {
		return nil, err
	}

	chans, err := s.store.ListChannels(ctx)
	if err != nil {
		return nil, err
	}

	out := &clipFitOutput{}
	// ⚠ Built from the CHANNEL list, not by ranging the fit map: Go map order is randomised, so
	// the picker's rows would shuffle on every render. The store's order is number-sorted, which
	// is the order an operator reads their channels in.
	out.Body.Channels = make([]ChannelFitDTO, 0, len(chans))
	for _, ch := range chans {
		fit, ok := fits[ch.ID]
		if !ok {
			continue // a channel created between the two reads; absent beats a fabricated answer
		}
		out.Body.Channels = append(out.Body.Channels, ChannelFitDTO{
			ChannelID: ch.ID, Name: ch.Name, Number: ch.Number,
			Level:    string(fit.Level),
			Reason:   string(fit.Reason),
			Pinned:   fit.Pinned,
			Excluded: fit.Excluded,
		})
	}
	return out, nil
}

// --- compilation splitting (§10, V34) ---
