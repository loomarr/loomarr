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
}

// ClipDTO is the API view of a filler clip (§10).
type ClipDTO struct {
	LibraryItemID string `json:"libraryItemId"`
	Name          string `json:"name"`
	Kind          string `json:"kind" enum:"commercial,bumper,station_id,psa,trailer,interstitial"`
	Era           int    `json:"era,omitempty"`
	Audience      string `json:"audience,omitempty" enum:"kids,family,general,late_night,"`
	Category      string `json:"category,omitempty"`
	DurationMs    int64  `json:"durationMs"`
	Source        string `json:"source,omitempty"`
	AITagged      bool   `json:"aiTagged"`
	Tagged        bool   `json:"tagged" doc:"Whether the clip has all match tags (era+audience+category)"`
}

func clipToDTO(c store.Clip) ClipDTO {
	return ClipDTO{
		LibraryItemID: c.LibraryItemID, Name: c.Name, Kind: string(c.Kind),
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
	}
}
type clipOutput struct{ Body ClipDTO }

func (s *Server) patchFillerClip(ctx context.Context, in *patchClipInput) (*clipOutput, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	// A manual edit clears the AI flag (a human tagged it).
	if err := s.store.UpdateClipTags(ctx, in.ID, in.Body.Era, in.Body.Audience, in.Body.Category, false, time.Now()); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return nil, huma.Error404NotFound("no such clip")
		}
		return nil, err
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
	if s.filler == nil {
		return nil, huma.Error501NotImplemented("filler not configured")
	}
	total, added, updated, pruned, err := s.filler.Sync(ctx)
	if err != nil {
		return nil, huma.Error502BadGateway("filler sync failed", err)
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
	if s.filler == nil {
		return nil, huma.Error501NotImplemented("filler not configured")
	}
	considered, tagged, partial, skipped, err := s.filler.Tag(ctx)
	if err != nil {
		return nil, huma.Error502BadGateway("filler tagging failed", err)
	}
	out := &tagFillerOutput{}
	out.Body.Considered, out.Body.Tagged, out.Body.Partial, out.Body.Skipped = considered, tagged, partial, skipped
	return out, nil
}
