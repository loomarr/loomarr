package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/auth"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/suggest"
)

// registerUsers mounts /v1/users* (§11). All are admin-only.
func (s *Server) registerUsers(api huma.API) {
	huma.Register(api, withRole(huma.Operation{
		OperationID: "list-users", Method: http.MethodGet, Path: "/v1/users",
		Summary: "List users (admin)", Tags: []string{"users"},
	}, RoleAdmin), s.listUsers)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "patch-user", Method: http.MethodPatch, Path: "/v1/users/{id}",
		Summary: "Update a user's role/quota/disabled (admin)", Tags: []string{"users"},
	}, RoleAdmin), s.patchUser)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "sync-users", Method: http.MethodPost, Path: "/v1/users/sync",
		Summary: "Import/sync users from the media server (admin)", Tags: []string{"users"},
	}, RoleAdmin), s.syncUsers)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "list-user-sessions", Method: http.MethodGet, Path: "/v1/users/{id}/sessions",
		Summary: "List a user's live sessions (admin)", Tags: []string{"users"},
	}, RoleAdmin), s.listUserSessions)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "revoke-session", Method: http.MethodDelete, Path: "/v1/sessions/{hash}",
		Summary: "Revoke one session (admin)", Tags: []string{"users"},
	}, RoleAdmin), s.revokeSession)
}

type syncOutput struct {
	Body struct {
		Synced int `json:"synced"`
	}
}

func (s *Server) syncUsers(ctx context.Context, _ *struct{}) (*syncOutput, error) {
	if s.userSync == nil || s.libraryUnconfigured() {
		return nil, errNotImplemented("Media server not connected",
			"Connect a media server in Settings to sync imported users.")
	}
	n, err := s.userSync.Sync(ctx)
	if err != nil {
		return nil, err
	}
	out := &syncOutput{}
	out.Body.Synced = n
	return out, nil
}

type userBody struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Role     string `json:"role" enum:"admin,member"`
	Disabled bool   `json:"disabled"`
	Quota    int    `json:"quota" doc:"Pending-acquisition cap; 0 ⇒ the suggest.max_acquisitions default (§11)"`
	// PendingAcquisitions is what the cap is measured against — titles this user caused
	// that have not landed. Shipped so the UI can show "3 / 5" HONESTLY; it was withheld
	// until the cap actually bound something, because a denominator implies a limit.
	PendingAcquisitions int `json:"pendingAcquisitions"`
	// EffectiveQuota is Quota, or the configured default when Quota is 0 — so the UI
	// never has to re-derive the fallback and disagree with the server about it.
	EffectiveQuota int  `json:"effectiveQuota"`
	AutoApprove    bool `json:"autoApprove"`
	// Local reports ownership of the credential, independently from whether a
	// linked account has an offline verifier (§11).
	Local bool `json:"local"`
	// OfflineLogin reports that a media-server-linked account has captured a
	// verifier and can authenticate during provider unavailability.
	OfflineLogin bool `json:"offlineLogin"`
}

func toUserBody(u store.User) userBody {
	return userBody{
		ID: u.ID, Name: u.Name, Role: string(u.Role), Disabled: u.Disabled,
		Quota: u.Quota, AutoApprove: u.AutoApprove, Local: !u.MediaServerLinked && u.PasswordHash != "",
		OfflineLogin: u.MediaServerLinked && u.PasswordHash != "",
	}
}

type listUsersOutput struct {
	Body struct {
		Users []userBody `json:"users"`
	}
}

func (s *Server) listUsers(ctx context.Context, _ *struct{}) (*listUsersOutput, error) {
	users, err := s.store.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	out := &listUsersOutput{}
	out.Body.Users = make([]userBody, 0, len(users))
	for _, u := range users {
		out.Body.Users = append(out.Body.Users, s.withUsage(ctx, toUserBody(u), u))
	}
	return out, nil
}

// withUsage fills in the quota accounting (§11). Best-effort: a usage read that fails
// leaves the count at zero rather than failing the whole list — the Users page must still
// render so an admin can act, and a wrong-looking zero is visibly different from a page
// that will not load.
func (s *Server) withUsage(ctx context.Context, body userBody, u store.User) userBody {
	limit := u.Quota
	if limit <= 0 {
		limit = s.configInt("suggest.max_acquisitions")
	}
	body.EffectiveQuota = limit
	usage, err := suggest.PendingFor(ctx, s.store, u.ID, limit)
	if err != nil {
		return body
	}
	body.PendingAcquisitions = usage.Pending
	return body
}

type patchUserInput struct {
	ID   string `path:"id"`
	Body struct {
		Role        *string `json:"role,omitempty" enum:"admin,member"`
		Quota       *int    `json:"quota,omitempty"`
		AutoApprove *bool   `json:"autoApprove,omitempty"`
		Disabled    *bool   `json:"disabled,omitempty"`
	}
}
type patchUserOutput struct{ Body userBody }

func (s *Server) patchUser(ctx context.Context, in *patchUserInput) (*patchUserOutput, error) {
	u, err := s.store.GetUser(ctx, in.ID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, errNotFound("User not found", "That user doesn't exist — it may have been removed.")
	}
	if err != nil {
		return nil, err
	}
	if in.Body.Role != nil {
		u.Role = store.Role(*in.Body.Role)
	}
	if in.Body.Quota != nil {
		u.Quota = *in.Body.Quota
	}
	if in.Body.AutoApprove != nil {
		u.AutoApprove = *in.Body.AutoApprove
	}
	// Disabling a user must immediately revoke their sessions (§11). Route it
	// through the login service so revocation always accompanies the flag.
	if in.Body.Disabled != nil && *in.Body.Disabled && !u.Disabled {
		if s.login != nil {
			if err := s.login.Disable(ctx, u.ID); err != nil {
				return nil, err
			}
			u.Disabled = true
		}
	} else if in.Body.Disabled != nil {
		u.Disabled = *in.Body.Disabled
	}

	if err := s.store.UpsertUser(ctx, u); err != nil {
		return nil, err
	}
	return &patchUserOutput{Body: toUserBody(u)}, nil
}

// sessionBody is one live session as an admin sees it (§11). ID is the stored SHA-256
// hash, not a token — it is the natural handle for revocation and cannot be reversed
// into the cookie it authenticates, so the schema's "a read never yields a usable
// cookie" guarantee survives being surfaced.
type sessionBody struct {
	ID        string `json:"id" doc:"Opaque session handle (the stored token hash); pass to DELETE /v1/sessions/{hash}"`
	UserID    string `json:"userId"`
	CreatedAt int64  `json:"createdAt" doc:"Unix ms"`
	ExpiresAt int64  `json:"expiresAt" doc:"Unix ms"`
	Current   bool   `json:"current" doc:"This is the caller's own session — revoking it signs them out"`
}

type listUserSessionsInput struct {
	ID string `path:"id"`
}
type listUserSessionsOutput struct {
	Body struct {
		Sessions []sessionBody `json:"sessions"`
	}
}

func (s *Server) listUserSessions(ctx context.Context, in *listUserSessionsInput) (*listUserSessionsOutput, error) {
	if s.sessions == nil {
		return nil, errServiceUnavailable("Sessions unavailable", "Session tracking isn't set up, so live sessions can't be listed right now.")
	}
	if _, err := s.store.GetUser(ctx, in.ID); errors.Is(err, store.ErrNotFound) {
		return nil, errNotFound("User not found", "That user doesn't exist — it may have been removed.")
	} else if err != nil {
		return nil, err
	}

	live, err := s.sessions.List(ctx, in.ID)
	if err != nil {
		return nil, err
	}

	// The caller's own cookie marks exactly one row as "this device". Read from the raw
	// request because the token itself is never put in the context — only the user is.
	var callerToken string
	if r := requestFrom(ctx); r != nil {
		if c, err := r.Cookie(auth.CookieName); err == nil {
			callerToken = c.Value
		}
	}

	out := &listUserSessionsOutput{}
	out.Body.Sessions = make([]sessionBody, 0, len(live))
	for _, sess := range live {
		out.Body.Sessions = append(out.Body.Sessions, sessionBody{
			ID:        sess.TokenHash,
			UserID:    sess.UserID,
			CreatedAt: sess.CreatedAt.UnixMilli(),
			ExpiresAt: sess.ExpiresAt.UnixMilli(),
			Current:   auth.IsCurrent(sess.TokenHash, callerToken),
		})
	}
	return out, nil
}

type revokeSessionInput struct {
	Hash string `path:"hash"`
}

func (s *Server) revokeSession(ctx context.Context, in *revokeSessionInput) (*struct{}, error) {
	if s.sessions == nil {
		return nil, errServiceUnavailable("Sessions unavailable", "Session tracking isn't set up, so this session can't be revoked right now.")
	}
	// Deliberately idempotent: revoking an already-dead session is a success, not a 404.
	// The list an admin is acting on can go stale between render and click (the session
	// may expire, or the user may sign out), and an error there would be noise about an
	// outcome the admin already has.
	return nil, s.sessions.RevokeHash(ctx, in.Hash)
}
