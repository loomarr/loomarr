package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/mantonx/loomarr/internal/filler"
	"github.com/mantonx/loomarr/internal/store"
)

// registerFiller mounts /v1/filler* (§7/§10). List is visible to any
// authenticated user; tag edit, sync, and the AI-tagging job require admin
// (filler ingestion is an admin concern, §7).
func (s *Server) registerFiller(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "list-filler", Method: http.MethodGet, Path: "/v1/filler",
		Summary: "List filler clips", Description: "Filter by kind/era/audience/category.",
		Tags: []string{"filler"},
	}, s.listFiller)

	huma.Register(api, huma.Operation{
		OperationID: "tag-filler-clip", Method: http.MethodPatch, Path: "/v1/filler/{id}",
		Summary: "Edit a clip's tags", Description: "Admin only.", Tags: []string{"filler"},
	}, s.patchFillerClip)

	huma.Register(api, huma.Operation{
		OperationID: "sync-filler", Method: http.MethodPost, Path: "/v1/filler/sync",
		Summary: "Sync the clip catalog from the media server", Description: "Admin only (§10).",
		Tags: []string{"filler"},
	}, s.syncFiller)

	huma.Register(api, huma.Operation{
		OperationID: "tag-filler", Method: http.MethodPost, Path: "/v1/filler/tag",
		Summary: "AI-tag untagged clips", Description: "Admin only. Text-signal classification (§10).",
		Tags: []string{"filler"},
	}, s.tagFiller)

	huma.Register(api, huma.Operation{
		OperationID: "ingest-filler", Method: http.MethodPost, Path: "/v1/filler/ingest",
		Summary: "Download clips into the drop-folder (admin; loomarr:filler image only)",
		Tags:    []string{"filler"},
	}, s.ingestFiller)
}

// ClipDTO is the API view of a filler clip (§10). Identity is the Tunarr `local`-
// source program uuid (the redesign moved filler off the media server).
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
	AITagged        bool   `json:"aiTagged"`
	Tagged          bool   `json:"tagged" doc:"Whether the clip has all match tags (era+audience+category)"`
}

func clipToDTO(c store.Clip) ClipDTO {
	return ClipDTO{
		Path: c.Path, TunarrProgramID: c.TunarrProgramID, Name: c.Name, Kind: string(c.Kind),
		Era: c.Era, Audience: string(c.Audience), Category: c.Category,
		DurationMs: c.DurationMs, Source: c.Source, AITagged: c.AITagged, Tagged: c.Tagged(),
	}
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
		out.Body.Clips = append(out.Body.Clips, clipToDTO(c))
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
	// A manual edit clears the AI flag (a human tagged it).
	if err := s.store.UpdateClipTags(ctx, in.ID, in.Body.Era, in.Body.Audience, in.Body.Category, false, now); err != nil {
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
	return &clipOutput{Body: clipToDTO(c)}, nil
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
		// NOT feature_not_configured: no setting can open this gate. The message names
		// the actual remedy (a different image), because pointing an operator at
		// Settings for something Settings cannot fix is the dead end §7 warns about.
		return nil, errConflict("Downloads aren't available here",
			"This build has no download tooling. Run the loomarr:filler image to download clips in-app.")
	}
	if err != nil {
		return nil, err
	}
	out := &ingestFillerOutput{}
	out.Body.JobID = jobID
	return out, nil
}
