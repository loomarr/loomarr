package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"

	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
)

// registerMiddleware resolves each request's role + user (§7) before the
// operation runs and stores them on the context for per-op authorization.
func (s *Server) registerMiddleware(api huma.API) {
	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		r, _ := humago.Unwrap(ctx)
		role := RoleAnonymous
		var user *store.User
		var deviceID string
		if s.auth != nil {
			if ia, ok := s.auth.(identityAuthorizer); ok {
				identity := ia.AuthorizeIdentity(r)
				role, user, deviceID = identity.role, identity.user, identity.deviceID
			} else if ua, ok := s.auth.(UserAuthorizer); ok {
				role, user = ua.AuthorizeUser(r)
			} else {
				role = s.auth.Authorize(r)
			}
		}
		// CSRF (§11): mutating requests authenticated by a *cookie* must carry
		// X-Loomarr-Csrf: 1. Combined with SameSite=Strict this closes form-based
		// CSRF cheaply. Bearer-token (machine) callers are exempt — they don't send
		// cookies, so they aren't a CSRF vector. Login is exempt (no session yet).
		//
		// ⚠ The exemption tests for a BEARER credential, not for the absence of a user. Those were
		// the same thing while API_TOKEN was the only bearer path (it resolves to admin with no
		// user), but a paired device (§11, Shield P1) authenticates by bearer AND carries identity.
		// Keying on `user != nil` would demand a CSRF header from a client that cannot be
		// cross-site-forged, so the condition now says what the paragraph above always meant.
		if isMutating(r.Method) && user != nil && !hasBearer(r) && r.URL.Path != "/v1/auth/login" &&
			r.Header.Get("X-Loomarr-Csrf") != "1" {
			_ = huma.WriteErr(api, ctx, http.StatusForbidden, "missing X-Loomarr-Csrf header")
			return
		}

		// ⚠ **AUTHORIZATION, from the route's own declaration** (§11, routeauth.go). The
		// role a route needs rides on its operation, so this enforces it for every route at
		// once — rather than depending on each handler body remembering to ask. An
		// operation that declared nothing requires admin, so forgetting fails CLOSED.
		//
		// Runs after CSRF: a cookie-authenticated request missing the header is refused as
		// CSRF regardless of role, which keeps that refusal from reading as "wrong role".
		if want := roleForOperation(ctx.Operation()); !authorizeRole(want, role) {
			// Goes through WriteErr so the request-id stamping and cause logging in
			// installUserFacingErrors applies here too — a refusal an operator cannot
			// correlate to a request is a refusal they cannot debug.
			if role == RoleAnonymous {
				_ = huma.WriteErr(api, ctx, http.StatusUnauthorized, "You need to sign in to do this.")
			} else {
				_ = huma.WriteErr(api, ctx, http.StatusForbidden, "This action needs an admin account.")
			}
			return
		}

		c := huma.WithValue(ctx, roleCtxKey{}, role)
		c = huma.WithValue(c, reqCtxKey{}, r) // raw request for handlers (login IP, cookies)
		if user != nil {
			c = huma.WithValue(c, userCtxKey{}, *user)
		}
		if deviceID != "" {
			c = huma.WithValue(c, deviceCtxKey{}, deviceID)
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
	// EpisodesHave of EpisodesWanted is Home's On the way "8 of 36 episodes" (#1667).
	EpisodesHave   int `json:"episodesHave,omitempty" doc:"A downloading series' episodes on disk (arr poll); absent for a movie or when unknown"`
	EpisodesWanted int `json:"episodesWanted,omitempty" doc:"The episodes the arr wants for this series (monitored and aired); absent for a movie or when unknown"`
	// LastError is why the reconciler gave up on an `unavailable` title (e.g. "deadline
	// exceeded"), so a client can say so instead of describing it as queued.
	LastError string `json:"lastError,omitempty" example:"deadline exceeded" doc:"Why the title was given up on (unavailable only)"`
	// AvailableAtMs is the title's arrival (Home's New this week, #1663): when the library
	// confirmed a title Loomarr acquired. Absent for a title a channel picked from the library.
	AvailableAtMs int64 `json:"availableAtMs,omitempty" doc:"When the library confirmed this acquired title (Unix ms); absent if it was already in the library"`
	// Channels are the channels whose lineup holds the title, by number, on the list and get
	// reads. Joined server-side from one channel read so no client walks every lineup.
	Channels []TitleChannelDTO `json:"channels,omitempty" doc:"Channels whose lineup holds this title, by number (detached channels excluded)"`
}

// TitleChannelDTO names a channel a title plays on.
type TitleChannelDTO struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Number int    `json:"number"`
}

func toDTO(r provision.Record) TitleDTO {
	var availableAtMs int64
	if !r.AvailableAt.IsZero() {
		availableAtMs = r.AvailableAt.UnixMilli()
	}
	return TitleDTO{
		Key: string(r.Key), MediaType: string(r.Title.MediaType),
		TMDBID: r.Title.TMDBID, TVDBID: r.Title.TVDBID,
		Name: r.Title.Name, Year: r.Title.Year,
		State: string(r.State), LibraryID: r.LibraryID,
		Progress: r.Progress, ETAText: r.ETAText, DownloadStatus: r.DownloadStatus,
		EpisodesHave: r.EpisodesHave, EpisodesWanted: r.EpisodesWanted,
		LastError: r.LastError, AvailableAtMs: availableAtMs,
	}
}

// registerTitles mounts /v1/titles* (§7). Reads are visible to any authenticated
// user; POST/DELETE require admin (§7: enqueuing an acquisition is the approval
// gate's concern).
func (s *Server) registerTitles(api huma.API) {
	huma.Register(api, withRole(huma.Operation{
		OperationID: "enqueue-title", Method: http.MethodPost, Path: "/v1/titles",
		Summary: "Enqueue/ensure a title", Description: "Idempotent by external id (§4). Admin only.",
		Tags: []string{"titles"},
	}, RoleAdmin), s.enqueueTitle)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "get-title", Method: http.MethodGet, Path: "/v1/titles/{key}",
		Summary: "Get a title's provisioning state", Tags: []string{"titles"},
	}, RoleMember), s.getTitle)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "list-titles", Method: http.MethodGet, Path: "/v1/titles",
		Summary: "List titles, optionally filtered by state", Tags: []string{"titles"},
	}, RoleMember), s.listTitles)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "delete-title", Method: http.MethodDelete, Path: "/v1/titles/{key}",
		Summary: "Give up / cancel a title", Description: "Admin only.", Tags: []string{"titles"},
		DefaultStatus: http.StatusNoContent,
	}, RoleAdmin), s.deleteTitle)
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
	on, err := s.titleChannels(ctx)
	if err != nil {
		return nil, err
	}
	dto := toDTO(rec)
	dto.Channels = on[rec.Key]
	return &titleOutput{Body: dto}, nil
}

// titleChannels maps each title key to the channels whose lineup holds it, by number, from one
// channel read. A detached channel no longer plays anything.
func (s *Server) titleChannels(ctx context.Context) (map[provision.Key][]TitleChannelDTO, error) {
	channels, err := s.store.ListChannels(ctx) // ordered by number
	if err != nil {
		return nil, err
	}
	out := map[provision.Key][]TitleChannelDTO{}
	for _, ch := range channels {
		if ch.Status == schedule.StatusDetached {
			continue
		}
		seen := map[provision.Key]bool{}
		for _, e := range ch.Lineup {
			if !seen[e.Key] {
				seen[e.Key] = true
				out[e.Key] = append(out[e.Key], TitleChannelDTO{ID: ch.ID, Name: ch.Name, Number: ch.Number})
			}
		}
	}
	return out, nil
}

type listInput struct {
	State string `query:"state" enum:"wanted,requested,downloading,available,unavailable" doc:"Filter by state"`
	Since int64  `query:"since" minimum:"0" doc:"Unix ms. Lists the titles that arrived at or after this time, newest first (Home's New this week). state may be omitted or 'available'."`
}
type listOutput struct {
	Body struct {
		Titles []TitleDTO `json:"titles"`
	}
}

func (s *Server) listTitles(ctx context.Context, in *listInput) (*listOutput, error) {
	var (
		recs []provision.Record
		err  error
	)
	switch {
	case in.Since > 0 && in.State != "" && provision.State(in.State) != provision.Available:
		return nil, errBadRequest("Arrivals are available titles",
			"Only an available title has arrived. Leave out the state, or ask for available ones.")
	case in.Since > 0:
		recs, err = s.store.ListTitlesAvailableSince(ctx, time.UnixMilli(in.Since))
	case in.State == "":
		return nil, errBadRequest("State required", "Choose a state to filter titles by.")
	default:
		recs, err = s.store.ListTitlesByState(ctx, provision.State(in.State))
	}
	if err != nil {
		return nil, err
	}
	on, err := s.titleChannels(ctx)
	if err != nil {
		return nil, err
	}
	out := &listOutput{}
	out.Body.Titles = make([]TitleDTO, 0, len(recs))
	for _, r := range recs {
		dto := toDTO(r)
		dto.Channels = on[r.Key]
		out.Body.Titles = append(out.Body.Titles, dto)
	}
	return out, nil
}

type deleteOutput struct{}

func (s *Server) deleteTitle(ctx context.Context, in *keyInput) (*deleteOutput, error) {
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

// (`requireAdmin` lived here until 2026-08-10. It returned a 403 unless the caller resolved to
// admin, and 80 handler bodies opened by calling it — every one of them inside an operation that
// ALREADY declared RoleAdmin, which `registerMiddleware` above enforces for every route at once.
// Deleting the function rather than leaving it unused is the point: while it exists, the next
// handler can call it and re-create the two-places-nothing-connects shape routeauth.go documents,
// where intent lived in a Description and enforcement 250 lines away. The role now rides the
// operation, and an operation that declares nothing fails CLOSED.)
