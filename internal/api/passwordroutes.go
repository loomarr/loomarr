package api

import (
	"context"
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/mantonx/loomarr/internal/auth"
	"github.com/mantonx/loomarr/internal/store"
)

// Local account management (§11). Three routes, and the split between them IS the
// authorization model:
//
//   - POST /v1/auth/password        — SELF. Any signed-in local user. Proves the
//     current password; the caller's own id comes from the session, never the body,
//     so this can only ever change your own credential.
//   - POST /v1/users                — ADMIN. Mints a local account (an allowlist row
//     with a hash). The same class of write as /v1/users/import.
//   - POST /v1/users/{id}/password  — ADMIN. Resets someone else's password without
//     the current one. Separate route, separate handler, separate service method —
//     merging it with the self path would make "prove you know it" optional.
//
// §19 negatives (part of the gate, not extras): a member gets 403 on both admin
// routes, and the self route cannot be aimed at another user because it takes no
// target id at all.
func (s *Server) registerPasswords(api huma.API) {
	if s.passwords == nil && !s.schemaOnly {
		return
	}
	huma.Register(api, withRole(huma.Operation{
		OperationID: "change-password", Method: http.MethodPost, Path: "/v1/auth/password",
		Summary: "Change your own password", Tags: []string{"auth"},
		DefaultStatus: http.StatusNoContent,
	}, RoleMember), s.changePassword)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "create-local-user", Method: http.MethodPost, Path: "/v1/users",
		Summary: "Create a local account (admin)", Tags: []string{"users"},
	}, RoleAdmin), s.createLocalUser)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "reset-user-password", Method: http.MethodPost, Path: "/v1/users/{id}/password",
		Summary: "Reset a local user's password (admin)", Tags: []string{"users"},
		DefaultStatus: http.StatusNoContent,
	}, RoleAdmin), s.resetUserPassword)
}

type changePasswordInput struct {
	Body struct {
		// No user id: the target is ALWAYS the caller, resolved from the session. A
		// body-supplied id would turn this into an admin reset with no admin check.
		Current string `json:"current" format:"password" doc:"Your current password."`
		Next    string `json:"next" format:"password" minLength:"8" doc:"The new password (min 8 characters)."`
	}
}

func (s *Server) changePassword(ctx context.Context, in *changePasswordInput) (*struct{}, error) {
	if s.passwords == nil {
		return nil, errNotImplemented("Not available", "Local account management isn't configured on this install.")
	}
	uid := userIDFromHuma(ctx)
	if uid == "" {
		return nil, errUnauthorized("Sign in required", "Sign in to change your password.")
	}
	err := s.passwords.ChangePassword(ctx, uid, in.Body.Current, in.Body.Next)
	switch {
	case errors.Is(err, auth.ErrWrongPassword):
		// Deliberately NOT "wrong password" vs "no such user" — the caller is
		// authenticated, so the only useful distinction is whether the current
		// password matched.
		return nil, errUnauthorized("That didn't match", "Your current password isn't right. Try again.")
	case errors.Is(err, auth.ErrNotLocalUser):
		return nil, errConflict("Password lives elsewhere",
			"This account signs in with your media server, so Loomarr never stores its password — change it there and the new one works here immediately.")
	case errors.Is(err, auth.ErrWeakPassword):
		return nil, errUnprocessable("Password too short", "Use at least 8 characters.")
	case err != nil:
		return nil, err
	}
	return &struct{}{}, nil
}

type createLocalUserInput struct {
	Body struct {
		Username string `json:"username" minLength:"1"`
		Password string `json:"password" format:"password" minLength:"8"`
		Role     string `json:"role,omitempty" enum:"admin,member" doc:"Defaults to member."`
		Quota    int    `json:"quota,omitempty" minimum:"0" doc:"Pending-acquisition cap; 0 = the default."`
	}
}
type createLocalUserOutput struct {
	Body userBody
}

func (s *Server) createLocalUser(ctx context.Context, in *createLocalUserInput) (*createLocalUserOutput, error) {
	if s.passwords == nil {
		return nil, errNotImplemented("Not available", "Local account management isn't configured on this install.")
	}
	if in.Body.Quota < 0 {
		return nil, errUnprocessable("Invalid quota", "A pending-acquisition limit must be zero (use the default) or a positive number.")
	}
	// Default to member: minting an admin should be a deliberate choice, not what
	// happens when a field is omitted (§11 — roles gate real spending).
	role := store.RoleMember
	if in.Body.Role == string(store.RoleAdmin) {
		role = store.RoleAdmin
	}
	u, err := s.passwords.CreateLocal(ctx, in.Body.Username, in.Body.Password, role, in.Body.Quota)
	switch {
	case errors.Is(err, auth.ErrDuplicateUsername):
		return nil, errConflict("Name already taken", "Someone already signs in with that username. Pick another.")
	case errors.Is(err, auth.ErrWeakPassword):
		return nil, errUnprocessable("Password too short", "Use at least 8 characters.")
	case errors.Is(err, auth.ErrInvalidBootstrap):
		return nil, errUnprocessable("Details required", "Enter a username and a password.")
	case err != nil:
		return nil, err
	}
	s.activity.Info(ctx, store.ActivityKindUser, u.ID, "Created local account "+u.Name)
	return &createLocalUserOutput{Body: s.withUsage(ctx, toUserBody(u), u)}, nil
}

type resetPasswordInput struct {
	ID   string `path:"id"`
	Body struct {
		Next string `json:"next" format:"password" minLength:"8"`
	}
}

func (s *Server) resetUserPassword(ctx context.Context, in *resetPasswordInput) (*struct{}, error) {
	if s.passwords == nil {
		return nil, errNotImplemented("Not available", "Local account management isn't configured on this install.")
	}
	err := s.passwords.ResetPassword(ctx, in.ID, in.Body.Next)
	switch {
	case errors.Is(err, store.ErrNotFound):
		return nil, errNotFound("No such user", "That account doesn't exist — it may have been removed.")
	case errors.Is(err, auth.ErrNotLocalUser):
		return nil, errConflict("Nothing to reset",
			"This account signs in with your media server, so Loomarr never held its password.")
	case errors.Is(err, auth.ErrWeakPassword):
		return nil, errUnprocessable("Password too short", "Use at least 8 characters.")
	case err != nil:
		return nil, err
	}
	name := in.ID
	if u, getErr := s.store.GetUser(ctx, in.ID); getErr == nil {
		name = u.Name
	}
	s.activity.Info(ctx, store.ActivityKindUser, in.ID, "Reset Loomarr password for "+name)
	return &struct{}{}, nil
}
