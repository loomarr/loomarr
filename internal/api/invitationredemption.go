package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/auth"
	"github.com/loomarr/loomarr/internal/invitation"
	"github.com/loomarr/loomarr/internal/store"
)

func (s *Server) registerInvitationRedemption(api huma.API) {
	if s.invitationRedemption == nil && !s.schemaOnly {
		return
	}
	huma.Register(api, withRole(huma.Operation{
		OperationID: "preview-invitation", Method: http.MethodPost, Path: "/v1/invitations/preview",
		Summary: "Preview an invitation without redeeming it", Tags: []string{"invitations"},
	}, RolePublic), s.previewInvitation)
	huma.Register(api, withRole(huma.Operation{
		OperationID: "redeem-invitation", Method: http.MethodPost, Path: "/v1/invitations/redeem",
		Summary: "Prove the invited credential and activate the account", Tags: []string{"invitations"},
	}, RolePublic), s.redeemInvitation)
}

type invitationBearerInput struct {
	Body struct {
		Grant string `json:"grant" format:"password"`
	}
}

type invitationPreviewOutput struct {
	Body struct {
		Kind           string `json:"kind" enum:"local,library"`
		Username       string `json:"username,omitempty"`
		DisplayName    string `json:"displayName,omitempty"`
		Role           string `json:"role" enum:"admin,member"`
		CredentialPath string `json:"credentialPath" enum:"local_password,library_password"`
		ExpiresAt      int64  `json:"expiresAt" doc:"Unix milliseconds"`
	}
}

func (s *Server) previewInvitation(ctx context.Context, in *invitationBearerInput) (*invitationPreviewOutput, error) {
	value, err := s.invitationRedemption.Preview(ctx, in.Body.Grant)
	if err != nil {
		return nil, invitationRedemptionError(err)
	}
	out := &invitationPreviewOutput{}
	out.Body.Kind = string(value.Kind)
	out.Body.Username = value.Username
	out.Body.DisplayName = value.DisplayName
	out.Body.Role = string(value.Role)
	out.Body.ExpiresAt = value.ExpiresAt.UnixMilli()
	if value.Kind == invitation.KindLibrary {
		out.Body.CredentialPath = "library_password"
	} else {
		out.Body.CredentialPath = "local_password"
	}
	return out, nil
}

type redeemInvitationInput struct {
	Body struct {
		Grant    string `json:"grant" format:"password"`
		Username string `json:"username,omitempty"`
		Password string `json:"password" format:"password"`
	}
}

func (s *Server) redeemInvitation(ctx context.Context, in *redeemInvitationInput) (*meOutput, error) {
	preview, err := s.invitationRedemption.Preview(ctx, in.Body.Grant)
	if err != nil {
		return nil, invitationRedemptionError(err)
	}
	var token string
	var expires time.Time
	var user store.User
	if preview.Kind == invitation.KindLibrary {
		token, expires, user, err = s.invitationRedemption.RedeemLibrary(
			ctx, in.Body.Grant, in.Body.Username, in.Body.Password)
	} else {
		token, expires, user, err = s.invitationRedemption.RedeemLocal(ctx, in.Body.Grant, in.Body.Password)
	}
	if err != nil {
		return nil, invitationRedemptionError(err)
	}
	return &meOutput{
		SetCookie: s.sessionCookie(requestFrom(ctx), token, expires),
		Body: meBody{ID: user.ID, Name: user.Name, Role: string(user.Role), Disabled: user.Disabled,
			Quota: user.Quota, AutoApprove: user.AutoApprove,
			Local: !user.MediaServerLinked, OfflineLogin: user.MediaServerLinked && user.PasswordHash != ""},
	}, nil
}

func invitationRedemptionError(err error) error {
	switch {
	case errors.Is(err, auth.ErrInvalidInvitation):
		return errGone("Invitation unavailable", "This invitation is invalid, expired, revoked, or already used. Ask an administrator for a new invitation.")
	case errors.Is(err, auth.ErrWeakPassword):
		return errUnprocessable("Password too short", "Use at least 8 characters.")
	case errors.Is(err, auth.ErrInvalidCredentials):
		return errUnauthorized("Account couldn't be verified", "Those Library credentials couldn't verify the invited account. Check them and try again.")
	case errors.Is(err, auth.ErrInvitationProviderUnavailable):
		return errServiceUnavailable("Library temporarily unavailable", "Loomarr must contact the Library before this account's first sign-in. Wait for it to return, then try again.")
	default:
		return err
	}
}
