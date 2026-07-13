package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"

	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/store"
)

// registerMiddleware resolves each request's role (§7) before the operation
// runs and stores it on the context for per-op authorization checks.
func (s *Server) registerMiddleware(api huma.API) {
	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		role := RoleAnonymous
		if s.auth != nil {
			r, _ := humago.Unwrap(ctx)
			role = s.auth.Authorize(r)
		}
		next(huma.WithValue(ctx, roleCtxKey{}, role))
	})
}

// --- DTOs (schemas are generated from these — §7.1) ---

// TitleDTO is the API view of a provisioning record.
type TitleDTO struct {
	Key       string `json:"key" example:"movie:tmdb:1111867" doc:"Stable identity key"`
	MediaType string `json:"mediaType" enum:"movie,series" example:"movie"`
	TMDBID    int    `json:"tmdbId,omitempty" example:"1111867"`
	TVDBID    int    `json:"tvdbId,omitempty"`
	Name      string `json:"name,omitempty" example:"In Flames"`
	Year      int    `json:"year,omitempty" example:"2023"`
	State     string `json:"state" enum:"wanted,requested,downloading,available,unavailable" doc:"Provisioning state (§4)"`
	LibraryID string `json:"libraryId,omitempty"`
}

func toDTO(r provision.Record) TitleDTO {
	return TitleDTO{
		Key: string(r.Key), MediaType: string(r.Title.MediaType),
		TMDBID: r.Title.TMDBID, TVDBID: r.Title.TVDBID,
		Name: r.Title.Name, Year: r.Title.Year,
		State: string(r.State), LibraryID: r.LibraryID,
	}
}

// registerTitles mounts /v1/titles* (§7). Reads are visible to any authenticated
// user; POST/DELETE require admin (§7: enqueuing an acquisition is the approval
// gate's concern).
func (s *Server) registerTitles(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "enqueue-title", Method: http.MethodPost, Path: "/v1/titles",
		Summary: "Enqueue/ensure a title", Description: "Idempotent by external id (§4). Admin only.",
		Tags: []string{"titles"},
	}, s.enqueueTitle)

	huma.Register(api, huma.Operation{
		OperationID: "get-title", Method: http.MethodGet, Path: "/v1/titles/{key}",
		Summary: "Get a title's provisioning state", Tags: []string{"titles"},
	}, s.getTitle)

	huma.Register(api, huma.Operation{
		OperationID: "list-titles", Method: http.MethodGet, Path: "/v1/titles",
		Summary: "List titles, optionally filtered by state", Tags: []string{"titles"},
	}, s.listTitles)

	huma.Register(api, huma.Operation{
		OperationID: "delete-title", Method: http.MethodDelete, Path: "/v1/titles/{key}",
		Summary: "Give up / cancel a title", Description: "Admin only.", Tags: []string{"titles"},
		DefaultStatus: http.StatusNoContent,
	}, s.deleteTitle)
}

// --- handlers ---

type enqueueInput struct {
	Body struct {
		MediaType string `json:"mediaType" enum:"movie,series"`
		TMDBID    int    `json:"tmdbId,omitempty"`
		TVDBID    int    `json:"tvdbId,omitempty"`
		Name      string `json:"name,omitempty"`
		Year      int    `json:"year,omitempty"`
		Seasons   []int  `json:"seasons,omitempty"`
	}
}
type titleOutput struct{ Body TitleDTO }

func (s *Server) enqueueTitle(ctx context.Context, in *enqueueInput) (*titleOutput, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	t := provision.Title{
		MediaType: provision.MediaType(in.Body.MediaType),
		TMDBID:    in.Body.TMDBID, TVDBID: in.Body.TVDBID,
		Name: in.Body.Name, Year: in.Body.Year, Seasons: in.Body.Seasons,
	}
	key, err := t.Key()
	if err != nil {
		return nil, huma.Error422UnprocessableEntity("invalid title identity", err)
	}
	// Idempotent enqueue (§4 inv. 3): only create if absent, else return current.
	if existing, err := s.store.GetTitle(ctx, key); err == nil {
		return &titleOutput{Body: toDTO(existing)}, nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	rec := provision.Record{Key: key, Title: t, State: provision.Wanted}
	if err := s.store.UpsertTitle(ctx, rec); err != nil {
		return nil, err
	}
	return &titleOutput{Body: toDTO(rec)}, nil
}

type keyInput struct {
	Key string `path:"key" example:"movie:tmdb:1111867"`
}

func (s *Server) getTitle(ctx context.Context, in *keyInput) (*titleOutput, error) {
	rec, err := s.store.GetTitle(ctx, provision.Key(in.Key))
	if errors.Is(err, store.ErrNotFound) {
		return nil, huma.Error404NotFound("no such title")
	}
	if err != nil {
		return nil, err
	}
	return &titleOutput{Body: toDTO(rec)}, nil
}

type listInput struct {
	State string `query:"state" enum:"wanted,requested,downloading,available,unavailable" doc:"Filter by state"`
}
type listOutput struct {
	Body struct {
		Titles []TitleDTO `json:"titles"`
	}
}

func (s *Server) listTitles(ctx context.Context, in *listInput) (*listOutput, error) {
	if in.State == "" {
		return nil, huma.Error400BadRequest("state query param is required")
	}
	recs, err := s.store.ListTitlesByState(ctx, provision.State(in.State))
	if err != nil {
		return nil, err
	}
	out := &listOutput{}
	out.Body.Titles = make([]TitleDTO, 0, len(recs))
	for _, r := range recs {
		out.Body.Titles = append(out.Body.Titles, toDTO(r))
	}
	return out, nil
}

type deleteOutput struct{}

func (s *Server) deleteTitle(ctx context.Context, in *keyInput) (*deleteOutput, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	rec, err := s.store.GetTitle(ctx, provision.Key(in.Key))
	if errors.Is(err, store.ErrNotFound) {
		return nil, huma.Error404NotFound("no such title")
	}
	if err != nil {
		return nil, err
	}
	// Give up: mark unavailable (terminal) rather than hard-delete, preserving
	// the audit trail (§4). The reconciler's Cancel path handles downstream.
	rec.State = provision.Unavailable
	rec.LastError = "cancelled via API"
	if err := s.store.UpsertTitle(ctx, rec); err != nil {
		return nil, err
	}
	return &deleteOutput{}, nil
}

// requireAdmin returns a 403 unless the caller resolved to admin (§7).
func requireAdmin(ctx context.Context) error {
	if roleFromHuma(ctx) != RoleAdmin {
		return huma.Error403Forbidden("admin role required")
	}
	return nil
}
