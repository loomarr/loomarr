package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/mantonx/loomarr/internal/filler"
	"github.com/mantonx/loomarr/internal/store"
)

// registerFiller mounts /v1/filler* (§7/§10). List is visible to any
// authenticated user; tag edit, sync, and the AI-tagging job require admin
// (filler ingestion is an admin concern, §7).
func (s *Server) registerFiller(api huma.API) {
	huma.Register(api, withRole(huma.Operation{
		OperationID: "list-filler", Method: http.MethodGet, Path: "/v1/filler",
		Summary: "List filler clips", Description: "Filter by kind/era/audience/category.",
		Tags: []string{"filler"},
	}, RoleMember), s.listFiller)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "filler-pool", Method: http.MethodGet, Path: "/v1/filler/pool",
		Summary: "Catalog-wide filler health",
		Description: "How well the catalog can fill breaks across the whole install (§10 V35) — the Filler page's pool strip. " +
			"Counts what exists (clips, commercials, duration-eligible, untagged) and lists every live channel's coverage, WORST FIRST. " +
			"The per-channel answers are the SAME `Coverage` computation `/v1/channels/{id}/filler/coverage` returns, called once per channel, " +
			"so this page and the channel page cannot disagree — there is no aggregate ladder to drift from the real one. " +
			"Read-only, so any authenticated user may call it.",
		Tags: []string{"filler"},
	}, RoleMember), s.fillerPool)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "tag-filler-clip", Method: http.MethodPatch, Path: "/v1/filler/{id}",
		Summary: "Edit a clip's tags", Description: "Admin only.", Tags: []string{"filler"},
	}, RoleAdmin), s.patchFillerClip)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "sync-filler", Method: http.MethodPost, Path: "/v1/filler/sync",
		Summary: "Sync the clip catalog from the media server", Description: "Admin only (§10).",
		Tags: []string{"filler"},
	}, RoleAdmin), s.syncFiller)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "tag-filler", Method: http.MethodPost, Path: "/v1/filler/tag",
		Summary: "AI-tag untagged clips", Description: "Admin only. Text-signal classification (§10).",
		Tags: []string{"filler"},
	}, RoleAdmin), s.tagFiller)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "ingest-filler", Method: http.MethodPost, Path: "/v1/filler/ingest",
		Summary: "Download clips into the drop-folder (admin; needs the vendored yt-dlp + ffmpeg)",
		Tags:    []string{"filler"},
	}, RoleAdmin), s.ingestFiller)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "discover-filler", Method: http.MethodGet, Path: "/v1/filler/discover",
		Summary: "Search archive.org for clips to add",
		Description: "Admin only (§10, V33). Searches archive.org by keyword and returns candidates " +
			"WITHOUT downloading anything — the operator is deciding what to fetch, and making them " +
			"download to find out would defeat the point. Results carry only what the source's search " +
			"index knows: no duration, no tags, and no licence (see below). Feed an id to POST " +
			"/v1/filler/ingest to actually fetch it. Synchronous: one upstream request, no job to poll.",
		Tags: []string{"filler"},
	}, RoleAdmin), s.discoverFiller)

	// Compilation splitting (§10, V34). Propose is a job (detection runs minutes
	// per file); the proposal read and the confirm are synchronous. All admin:
	// these routes write the catalog, which is the same trust level as ingest.
	huma.Register(api, withRole(huma.Operation{
		OperationID: "split-filler", Method: http.MethodPost, Path: "/v1/filler/{id}/split",
		Summary: "Propose splits for a compilation clip",
		Description: "Admin only (§10, V34). Runs detection — chapters → blackdetect/silencedetect → " +
			"transcript rescue for over-long segments — as a background job (progress on /v1/events as " +
			"filler_split frames), producing a PERSISTED split proposal. Nothing enters the catalog: " +
			"review is not optional, because detection quality is a property of the source.",
		Tags: []string{"filler"},
	}, RoleAdmin), s.splitFiller)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "get-filler-split", Method: http.MethodGet, Path: "/v1/filler/splits/{proposalId}",
		Summary: "Read a split proposal",
		Description: "Admin only (§10, V34). The review surface's source of truth on SSE reconnect — " +
			"the same pattern as /v1/suggestions/{id}.",
		Tags: []string{"filler"},
	}, RoleAdmin), s.getFillerSplit)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "confirm-filler-split", Method: http.MethodPost, Path: "/v1/filler/splits/{proposalId}/confirm",
		Summary: "Commit a reviewed split",
		Description: "Admin only (§10, V34). The body is the operator's confirmed cut list — the " +
			"proposal as returned, possibly edited (cuts moved/merged/dropped; era suggestions accepted " +
			"or rejected; dedup-flagged segments kept or skipped). Only now do segments become catalog " +
			"clips: cut with ffmpeg stream copy, and the original compilation row removed — its identity " +
			"is a path that now means twenty clips, not one.",
		Tags: []string{"filler"},
	}, RoleAdmin), s.confirmFillerSplit)
}

// ClipDTO is the API view of a filler clip (§10). Identity is the clip's PATH relative to
// FILLER_DIR (§9.1 moved it off the Tunarr program uuid — see Path below).
type ClipDTO struct {
	// Path is the clip's IDENTITY (relative to FILLER_DIR) — what /v1/filler/{id} takes and
	// what a channel's pinned/excluded lists reference (§9.1).
	Path string `json:"path"`
	// TunarrProgramID is informational since §9.1: it exists for Tunarr filler-lists and is
	// empty on an install with no Tunarr. NOT an identity — clients must key on Path.
	TunarrProgramID string `json:"tunarrProgramId,omitempty"`
	Name            string `json:"name"`
	Kind            string `json:"kind" enum:"commercial,bumper,station_id,psa,trailer,interstitial"`
	Era             int    `json:"era,omitempty"`
	Audience        string `json:"audience,omitempty" enum:"kids,family,general,late_night,"`
	Category        string `json:"category,omitempty"`
	DurationMs      int64  `json:"durationMs"`
	Source          string `json:"source,omitempty"`
	// Quality is the resolution label ("1080p", "480p"); "" for an audio-only clip or one
	// scanned before the column existed. Shipped in migration 00014 and surfaced here by V28 —
	// it existed in the store for two phases with no way to see it.
	// ⚠ The `doc` string is a CLIENT-VISIBLE contract (it ships in api/openapi.yaml and the
	// generated TS). It used to say "never affects pod selection"; V17c made that false by
	// adding an opt-in floor, so it was amended in the same PR rather than left as a lie the
	// generated client repeats.
	Quality string `json:"quality,omitempty" doc:"Resolution label from the probed video height (1080p, 480p). Display-only by default; affects selection only when the filler.min_quality floor is set, which is off unless an operator turns it on."`
	// Thumbnail is the extracted frame's path relative to the thumbnail cache; "" when
	// extraction failed or has not run, which renders as no image rather than a broken one.
	Thumbnail string `json:"thumbnail,omitempty"`
	// PlayCount / LastPlayedAt count airings on INTERNAL playout only.
	//
	// ⚠ PlaysCounted is what stops this being a lie. A Tunarr-backed channel airs its filler
	// through Tunarr, which never reports back, so those clips sit at 0 forever — and "0
	// plays" and "we cannot see plays here" are different facts. The UI must render the
	// second as "not counted", never as a zero.
	PlayCount    int64  `json:"playCount"`
	LastPlayedAt string `json:"lastPlayedAt,omitempty" doc:"RFC3339; absent if never played (or not counted)"`
	PlaysCounted bool   `json:"playsCounted" doc:"False when this install cannot observe airings (Tunarr-backed playout) — render as 'not counted', not as 0"`
	AITagged     bool   `json:"aiTagged"`
	Tagged       bool   `json:"tagged" doc:"Whether the clip has all match tags (era+audience+category)"`
	// SuggestedEra is an AI-proposed era the validator refused to persist (§10 era
	// grounding, V34): the year appears nowhere in the clip's text signals, so it is a
	// question for the operator, not a tag. Confirming = PATCHing era (clears this);
	// 0/absent = no suggestion. Nothing in pod matching ever reads it.
	SuggestedEra int `json:"suggestedEra,omitempty" doc:"AI-proposed era NOT persisted as a tag because the year is absent from the clip's text signals (§10). Confirm by PATCHing era; 0 = no suggestion."`
}

// playsCounted reports whether THIS install can observe a filler clip airing.
//
// It is exactly "does internal playout run here": the resolver is what records a play (see
// playoutadapter.airingFiller), and a Tunarr-backed install has none — its filler is aired by
// Tunarr, which never reports back. Deriving it from the same field the program route checks
// means the flag cannot drift from the behaviour it describes.
func (s *Server) playsCounted() bool { return s.playoutResolver != nil }

// clipToDTO maps a stored clip. playsCounted comes from the caller because it is a property
// of the INSTALL (does internal playout run here?), not of the clip.
func clipToDTO(c store.Clip, playsCounted bool) ClipDTO {
	d := ClipDTO{
		Path: c.Path, TunarrProgramID: c.TunarrProgramID, Name: c.Name, Kind: string(c.Kind),
		Era: c.Era, Audience: string(c.Audience), Category: c.Category,
		DurationMs: c.DurationMs, Source: c.Source, Quality: c.Quality, Thumbnail: c.Thumbnail,
		PlayCount: c.PlayCount, PlaysCounted: playsCounted,
		AITagged: c.AITagged, Tagged: c.Tagged(), SuggestedEra: c.SuggestedEra,
	}
	if !c.LastPlayedAt.IsZero() {
		d.LastPlayedAt = c.LastPlayedAt.UTC().Format(time.RFC3339)
	}
	return d
}

type listFillerInput struct {
	Kind     string `query:"kind" enum:"commercial,bumper,station_id,psa,trailer,interstitial"`
	Era      int    `query:"era"`
	Audience string `query:"audience" enum:"kids,family,general,late_night"`
	Category string `query:"category"`
	Untagged bool   `query:"untagged" doc:"Only commercials missing match tags"`
	// Q is the clip corpus's search box (§7.2). Clip search lives here rather than on
	// /v1/search because a clip is not a provisionable title (§10) and cannot be a
	// federated Candidate without leaking a non-title into the LLM grounding path.
	Q string `query:"q" doc:"Case-insensitive substring match on the clip name"`
}
type listFillerOutput struct {
	Body struct {
		Clips []ClipDTO `json:"clips"`
	}
}

func (s *Server) listFiller(ctx context.Context, in *listFillerInput) (*listFillerOutput, error) {
	clips, err := s.store.ListClips(ctx, store.ClipFilter{
		Kind:         filler.Kind(in.Kind),
		Era:          in.Era,
		Audience:     filler.Audience(in.Audience),
		Category:     in.Category,
		UntaggedOnly: in.Untagged,
		Query:        in.Q,
	})
	if err != nil {
		return nil, err
	}
	out := &listFillerOutput{}
	out.Body.Clips = make([]ClipDTO, 0, len(clips))
	for _, c := range clips {
		out.Body.Clips = append(out.Body.Clips, clipToDTO(c, s.playsCounted()))
	}
	return out, nil
}

type patchClipInput struct {
	ID   string `path:"id"`
	Body struct {
		Era      int    `json:"era,omitempty"`
		Audience string `json:"audience,omitempty" enum:"kids,family,general,late_night,"`
		Category string `json:"category,omitempty"`
		// Kind is correctable by hand (§10). Detection at sync gets it wrong in one
		// direction often enough to matter — a trailer scanned as a commercial — and
		// kind drives pod ROLE (a bumper bookends a pod, a commercial fills it), so a
		// wrong kind yields structurally wrong pods rather than just a mis-tagged clip.
		// Empty means "leave it alone", so a tag-only edit never rewrites kind.
		Kind string `json:"kind,omitempty" enum:"commercial,bumper,station_id,psa,trailer,interstitial,"`
	}
}
type clipOutput struct{ Body ClipDTO }

func (s *Server) patchFillerClip(ctx context.Context, in *patchClipInput) (*clipOutput, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	now := time.Now()
	// A manual edit clears the AI flag (a human tagged it). suggestedEra is 0 here —
	// only the tagger writes suggestions — and the store's rule applies: setting era
	// CONFIRMS and clears any suggestion in the same write, while an era-less edit
	// leaves the operator's unanswered question alone (§10, V34).
	if err := s.store.UpdateClipTags(ctx, in.ID, in.Body.Era, in.Body.Audience, in.Body.Category, 0, false, now); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, errNotFound("Clip not found", "That filler clip doesn't exist — it may have been removed by a catalog sync.")
		}
		return nil, err
	}
	// Kind is a separate write because the AI tagging job shares UpdateClipTags and must
	// never touch kind. Both are idempotent single-row updates, so the same PATCH is
	// safe to retry if the second fails.
	if in.Body.Kind != "" {
		if err := s.store.UpdateClipKind(ctx, in.ID, in.Body.Kind, now); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, errNotFound("Clip not found", "That filler clip doesn't exist — it may have been removed by a catalog sync.")
			}
			return nil, err
		}
	}
	c, err := s.store.GetClip(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	return &clipOutput{Body: clipToDTO(c, s.playsCounted())}, nil
}

type syncFillerOutput struct {
	Body struct {
		Total   int `json:"total"`
		Added   int `json:"added"`
		Updated int `json:"updated"`
		Pruned  int `json:"pruned"`
	}
}

func (s *Server) syncFiller(ctx context.Context, _ *struct{}) (*syncFillerOutput, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	if s.filler == nil || s.featureOff(ctx, "filler") {
		return nil, errNotImplemented("Filler isn't set up", "Enable filler in Settings to sync a commercial and bumper catalog.")
	}
	total, added, updated, pruned, err := s.filler.Sync(ctx)
	if err != nil {
		return nil, apiErrWithCause(http.StatusBadGateway, "Couldn't sync filler",
			"Loomarr couldn't sync the filler catalog from your media server. Check its connection in Settings and try again.", err)
	}
	out := &syncFillerOutput{}
	out.Body.Total, out.Body.Added, out.Body.Updated, out.Body.Pruned = total, added, updated, pruned
	return out, nil
}

type tagFillerOutput struct {
	Body struct {
		Considered int `json:"considered"`
		Tagged     int `json:"tagged"`
		Partial    int `json:"partial"`
		Skipped    int `json:"skipped"`
	}
}

func (s *Server) tagFiller(ctx context.Context, _ *struct{}) (*tagFillerOutput, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	if s.filler == nil || s.featureOff(ctx, "filler") {
		return nil, errNotImplemented("Filler isn't set up", "Enable filler in Settings to sync a commercial and bumper catalog.")
	}
	considered, tagged, partial, skipped, err := s.filler.Tag(ctx)
	if err != nil {
		return nil, apiErrWithCause(http.StatusBadGateway, "Couldn't tag filler",
			"Loomarr couldn't AI-tag the filler clips. Check that an AI provider is connected in Settings and try again.", err)
	}
	out := &tagFillerOutput{}
	out.Body.Considered, out.Body.Tagged, out.Body.Partial, out.Body.Skipped = considered, tagged, partial, skipped
	return out, nil
}

type ingestFillerInput struct {
	Body struct {
		// URLs are supplied per request by an admin rather than configured globally:
		// there is no unattended crawler (§10), so ingestion is always a deliberate act
		// with a person attached to it.
		URLs []string `json:"urls" minItems:"1" doc:"YouTube playlist/video or Archive.org collection/item URLs"`
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
func (s *Server) ingestFiller(ctx context.Context, in *ingestFillerInput) (*ingestFillerOutput, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	if s.filler == nil {
		return nil, errNotImplemented("Filler isn't set up", "Enable filler in Settings to sync a commercial and bumper catalog.")
	}
	jobID, err := s.filler.Ingest(ctx, in.Body.URLs)
	if errors.Is(err, ErrIngestUnavailable) {
		// NOT feature_not_configured: no setting can assert that a binary RUNS. ⚠ This
		// used to say "run the loomarr:filler image" — that variant no longer exists (the
		// single image always ships the tooling, §16), so the remedy it named was a dead
		// end of exactly the kind §7 warns about. Unavailable now means a degraded
		// install, and the message says what an operator can actually check.
		return nil, errConflict("Downloads aren't available here",
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

type discoverFillerInput struct {
	// Query is what the operator typed. An empty search would return archive.org's entire
	// movies corpus ranked by nothing, which is not an answer to any question — so a search
	// needs at least two characters. No longer `required`, because `collection` is the other
	// way to ask; exactly one of the two must be given (checked in the handler).
	Query string `query:"q" minLength:"2" doc:"Words to search for, e.g. \"1980s cereal commercial\". Mutually exclusive with the collection parameter."`
	// Collection lists ONE named archive.org collection instead of searching. This is what a
	// starter pack is (§10, V17d): a curated collection an operator keeps or excludes from
	// before anything is fetched. Deliberately the same endpoint as the keyword search — a
	// separate route would be a second implementation of "list clips, download nothing".
	Collection string `query:"collection" doc:"An archive.org collection to list: a URL, a /details/<id> path, or a bare identifier. Mutually exclusive with the q parameter."`
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
func (s *Server) discoverFiller(ctx context.Context, in *discoverFillerInput) (*discoverFillerOutput, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	if s.filler == nil {
		return nil, errNotImplemented("Filler isn't set up",
			"Enable filler in Settings before searching for clips to add.")
	}

	// Exactly one mode. Both would be ambiguous (search WITHIN a collection is a different
	// query archive.org would have to be asked differently), and neither is the empty search
	// the minLength above exists to prevent.
	query, collection := strings.TrimSpace(in.Query), strings.TrimSpace(in.Collection)
	if (query == "") == (collection == "") {
		return nil, apiErr(http.StatusUnprocessableEntity, "Ask for one thing",
			"Send either q (to search) or collection (to list one collection), not both and not neither.")
	}

	var (
		items []DiscoveredClip
		total int
		err   error
	)
	if collection != "" {
		items, total, err = s.filler.DiscoverCollection(ctx, collection, in.Limit)
	} else {
		items, total, err = s.filler.Discover(ctx, query, in.Limit)
	}
	if err != nil {
		// A listing failure is upstream (archive.org unreachable or refusing), not the
		// caller's fault — so it is a 502-shaped problem, and the message says which side
		// broke rather than blaming the query.
		return nil, apiErrWithCause(http.StatusBadGateway, "Couldn't search for clips",
			"archive.org didn't answer. Try again in a moment.", err)
	}

	out := &discoverFillerOutput{}
	// Non-nil so a client can iterate without guarding; "nothing matched" is a real answer.
	out.Body.Items = items
	if out.Body.Items == nil {
		out.Body.Items = []DiscoveredClip{}
	}
	out.Body.Total = total
	out.Body.LicenceNote = "Licence information isn't available for most results. Check before reusing."
	return out, nil
}

// --- compilation splitting (§10, V34) ---

type splitFillerInput struct {
	ID string `path:"id" doc:"The compilation clip's path (its identity, §10)"`
}
type splitFillerOutput struct {
	Body struct {
		JobID string `json:"jobId" doc:"Detection job — progress streams on /v1/events as filler_split frames; the proposal id arrives in the terminal frame"`
	}
}

// splitFiller starts detection on a compilation clip. It answers immediately:
// a full-decode detection pass takes minutes, so the job id's progress rides
// the SSE bus and the proposal is read back when the terminal frame lands.
func (s *Server) splitFiller(ctx context.Context, in *splitFillerInput) (*splitFillerOutput, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	if s.filler == nil {
		return nil, errNotImplemented("Filler isn't set up",
			"Enable filler in Settings before splitting a compilation.")
	}
	jobID, err := s.filler.Split(ctx, in.ID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, errNotFound("Clip not found", "That filler clip doesn't exist — it may have been removed by a catalog sync.")
	}
	if errors.Is(err, ErrSplitUnavailable) {
		return nil, errConflict("Splitting isn't set up",
			"Splitting needs the filler drop-folder (filler.dir). Set it in Settings → Filler.")
	}
	if err != nil {
		return nil, err
	}
	out := &splitFillerOutput{}
	out.Body.JobID = jobID
	return out, nil
}

type getFillerSplitInput struct {
	ProposalID string `path:"proposalId"`
}
type getFillerSplitOutput struct {
	Body filler.SplitProposal
}

// getFillerSplit reads one proposal — the review surface's source of truth.
// Read straight from the store, like list/patch: no service indirection needed.
func (s *Server) getFillerSplit(ctx context.Context, in *getFillerSplitInput) (*getFillerSplitOutput, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	p, err := s.store.GetSplitProposal(ctx, in.ProposalID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, errNotFound("Split proposal not found",
			"That proposal doesn't exist — it may have been confirmed, rejected, or replaced by a re-run of detection.")
	}
	if err != nil {
		return nil, err
	}
	return &getFillerSplitOutput{Body: p}, nil
}

type confirmFillerSplitInput struct {
	ProposalID string `path:"proposalId"`
	Body       struct {
		// Segments is the operator's REVIEWED cut list: the proposal as returned,
		// possibly edited. It is re-validated server-side (§10 — the review gate's
		// teeth live on the write path, not in the UI).
		Segments []filler.SplitSegment `json:"segments" minItems:"1"`
	}
}
type confirmFillerSplitOutput struct {
	Body struct {
		// Clips is how many catalog clips the confirm created — the confirmation
		// the UI toasts before the catalog list refreshes.
		Clips int `json:"clips"`
	}
}

// confirmFillerSplit commits the reviewed cut list. Synchronous: stream-copy
// cuts seek rather than decode, so even twenty segments are seconds — the
// minutes belong to detection, which already ran.
func (s *Server) confirmFillerSplit(ctx context.Context, in *confirmFillerSplitInput) (*confirmFillerSplitOutput, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	if s.filler == nil {
		return nil, errNotImplemented("Filler isn't set up",
			"Enable filler in Settings before confirming a split.")
	}
	if err := s.filler.ConfirmSplit(ctx, in.ProposalID, in.Body.Segments); err != nil {
		switch {
		case errors.Is(err, store.ErrNotFound):
			return nil, errNotFound("Split proposal not found",
				"That proposal doesn't exist — it may have been confirmed already, or replaced by a re-run of detection.")
		case errors.Is(err, filler.ErrSplitValidation):
			return nil, apiErrWithCause(http.StatusUnprocessableEntity, "Invalid cut list",
				"The edited segments are outside the clip, overlap, or too short — fix the cuts and try again.", err)
		default:
			return nil, err
		}
	}
	out := &confirmFillerSplitOutput{}
	out.Body.Clips = len(in.Body.Segments)
	return out, nil
}
