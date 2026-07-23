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

// registerMiddleware resolves each request's role + user (§7) before the
// operation runs and stores them on the context for per-op authorization.
func (s *Server) registerMiddleware(api huma.API) {
	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		r, _ := humago.Unwrap(ctx)
		role := RoleAnonymous
		var user *store.User
		if s.auth != nil {
			if ua, ok := s.auth.(UserAuthorizer); ok {
				role, user = ua.AuthorizeUser(r)
			} else {
				role = s.auth.Authorize(r)
			}
		}
		// CSRF (§11): mutating requests authenticated by a *cookie* must carry
		// X-Loomarr-Csrf: 1. Combined with SameSite=Strict this closes form-based
		// CSRF cheaply. Bearer-token (machine) callers are exempt — they don't send
		// cookies, so they aren't a CSRF vector. Login is exempt (no session yet).
		if isMutating(r.Method) && user != nil && r.URL.Path != "/v1/auth/login" &&
			r.Header.Get("X-Loomarr-Csrf") != "1" {
			_ = huma.WriteErr(api, ctx, http.StatusForbidden, "missing X-Loomarr-Csrf header")
			return
		}

		c := huma.WithValue(ctx, roleCtxKey{}, role)
		c = huma.WithValue(c, reqCtxKey{}, r) // raw request for handlers (login IP, cookies)
		if user != nil {
			c = huma.WithValue(c, userCtxKey{}, *user)
		}
		next(c)
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
	// Download progress (§18.1 arr-queue-poll), populated while downloading via the direct
	// arr requester; zero on the Seerr path / once available.
	Progress       float64 `json:"progress,omitempty" doc:"Download completion 0..1 (arr queue poll)"`
	ETAText        string  `json:"etaText,omitempty" doc:"Human time-left from the download client"`
	DownloadStatus string  `json:"downloadStatus,omitempty" doc:"Download-client status (downloading/warning/stalled/…)"`
}

func toDTO(r provision.Record) TitleDTO {
	return TitleDTO{
		Key: string(r.Key), MediaType: string(r.Title.MediaType),
		TMDBID: r.Title.TMDBID, TVDBID: r.Title.TVDBID,
		Name: r.Title.Name, Year: r.Title.Year,
		State: string(r.State), LibraryID: r.LibraryID,
		Progress: r.Progress, ETAText: r.ETAText, DownloadStatus: r.DownloadStatus,
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
		return nil, apiErrWithCause(http.StatusUnprocessableEntity, "Invalid title",
			"That title's identity couldn't be resolved. Check the media type and id, then try again.", err)
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
		return nil, errNotFound("Title not found", "That title doesn't exist — it may have been removed.")
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
		return nil, errBadRequest("State required", "Choose a state to filter titles by.")
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
		return nil, errNotFound("Title not found", "That title doesn't exist — it may have been removed.")
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

// isMutating reports whether a method changes state (needs CSRF, §11).
func isMutating(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

// requireAdmin returns a 403 unless the caller resolved to admin (§7).
func requireAdmin(ctx context.Context) error {
	if roleFromHuma(ctx) != RoleAdmin {
		return errForbidden("Not allowed", "This action needs an admin account.")
	}
	return nil
}
