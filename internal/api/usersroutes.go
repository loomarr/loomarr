package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

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

	if s.userSync != nil {
		huma.Register(api, huma.Operation{
			OperationID: "sync-users", Method: http.MethodPost, Path: "/v1/users/sync",
			Summary: "Import/sync users from the media server (admin)", Tags: []string{"users"},
		}, s.syncUsers)
	}
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
}

func toUserBody(u store.User) userBody {
	return userBody{ID: u.ID, Name: u.Name, Role: string(u.Role), Disabled: u.Disabled, Quota: u.Quota, AutoApprove: u.AutoApprove}
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
