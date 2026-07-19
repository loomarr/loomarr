package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/mantonx/loomarr/internal/auth"
	"github.com/mantonx/loomarr/internal/store"
)

// registerUsers mounts /v1/users* (§11). All are admin-only.
func (s *Server) registerUsers(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "list-users", Method: http.MethodGet, Path: "/v1/users",
		Summary: "List users (admin)", Tags: []string{"users"},
	}, s.listUsers)

	huma.Register(api, huma.Operation{
		OperationID: "patch-user", Method: http.MethodPatch, Path: "/v1/users/{id}",
		Summary: "Update a user's role/quota/disabled (admin)", Tags: []string{"users"},
	}, s.patchUser)

	if s.userSync != nil || s.schemaOnly {
		huma.Register(api, huma.Operation{
			OperationID: "sync-users", Method: http.MethodPost, Path: "/v1/users/sync",
			Summary: "Import/sync users from the media server (admin)", Tags: []string{"users"},
		}, s.syncUsers)
	}

	huma.Register(api, huma.Operation{
		OperationID: "list-user-sessions", Method: http.MethodGet, Path: "/v1/users/{id}/sessions",
		Summary: "List a user's live sessions (admin)", Tags: []string{"users"},
	}, s.listUserSessions)

	huma.Register(api, huma.Operation{
		OperationID: "revoke-session", Method: http.MethodDelete, Path: "/v1/sessions/{hash}",
		Summary: "Revoke one session (admin)", Tags: []string{"users"},
	}, s.revokeSession)
}

type syncOutput struct {
	Body struct {
		Synced int `json:"synced"`
	}
}

func (s *Server) syncUsers(ctx context.Context, _ *struct{}) (*syncOutput, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
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
	ID          string `json:"id"`
	Name        string `json:"name"`
	Role        string `json:"role" enum:"admin,member"`
	Disabled    bool   `json:"disabled"`
	Quota       int    `json:"quota"`
	AutoApprove bool   `json:"autoApprove"`
	// Local reports the credential path (§11): true ⇒ a Loomarr-native user verified
	// against a stored bcrypt hash; false ⇒ an imported media-server user verified
	// against Emby/Jellyfin. The UI needs this to label rows and to explain why a
	// password action applies to some users and not others. The hash itself is never
	// exposed — only whether one exists.
	Local bool `json:"local"`
}

func toUserBody(u store.User) userBody {
	return userBody{
		ID: u.ID, Name: u.Name, Role: string(u.Role), Disabled: u.Disabled,
		Quota: u.Quota, AutoApprove: u.AutoApprove, Local: u.PasswordHash != "",
	}
}

type listUsersOutput struct {
	Body struct {
		Users []userBody `json:"users"`
	}
}

func (s *Server) listUsers(ctx context.Context, _ *struct{}) (*listUsersOutput, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	users, err := s.store.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	out := &listUsersOutput{}
	out.Body.Users = make([]userBody, 0, len(users))
	for _, u := range users {
		out.Body.Users = append(out.Body.Users, toUserBody(u))
	}
	return out, nil
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
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	u, err := s.store.GetUser(ctx, in.ID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, huma.Error404NotFound("no such user")
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
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	if s.sessions == nil {
		return nil, huma.Error503ServiceUnavailable("sessions are not configured")
	}
	if _, err := s.store.GetUser(ctx, in.ID); errors.Is(err, store.ErrNotFound) {
		return nil, huma.Error404NotFound("no such user")
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
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	if s.sessions == nil {
		return nil, huma.Error503ServiceUnavailable("sessions are not configured")
	}
	// Deliberately idempotent: revoking an already-dead session is a success, not a 404.
	// The list an admin is acting on can go stale between render and click (the session
	// may expire, or the user may sign out), and an error there would be noise about an
	// outcome the admin already has.
	return nil, s.sessions.RevokeHash(ctx, in.Hash)
}
