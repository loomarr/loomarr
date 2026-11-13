package people

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/api/httpcore"
	"github.com/loomarr/loomarr/internal/people/access"
	"github.com/loomarr/loomarr/internal/people/auth"
	"github.com/loomarr/loomarr/internal/platform/store/identitystore"
)

func (s *Handlers) registerInvitationRedemption(api huma.API) {
	if s.invitationRedemption == nil && !s.schemaOnly {
		return
	}
	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "preview-invitation", Method: http.MethodPost, Path: "/v1/invitations/preview",
		Summary: "Preview an invitation without redeeming it", Tags: []string{"invitations"},
	}, httpcore.RolePublic), s.previewInvitation)
	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "redeem-invitation", Method: http.MethodPost, Path: "/v1/invitations/redeem",
		Summary: "Prove the invited credential and activate the account", Tags: []string{"invitations"},
	}, httpcore.RolePublic), s.redeemInvitation)
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

// newInvitationPreviewLimiter allows a link to be opened a few times from one source (a refresh, a
// second device) and no more than that in a burst.
func newInvitationPreviewLimiter() *auth.RateLimiter { return auth.NewRateLimiter(0.2, 10) }

// newInvitationRedeemLimiter holds a redemption to a few attempts per account from one source, a
// few more per source across usernames, and a slowly refilling allowance per account across every
// source, so rotating addresses does not buy a fresh burst of password guesses.
func newInvitationRedeemLimiter() *auth.SourceLimiter {
	return auth.NewSourceLimiter(0.1, 5, 0.1, 10, 1.0/30, 10)
}

func tooManyInvitationAttempts() error {
	return httpcore.TooManyRequests("Too many attempts", "Too many invitation attempts. Wait a moment and try again.")
}

func (s *Handlers) previewInvitation(ctx context.Context, in *invitationBearerInput) (*invitationPreviewOutput, error) {
	source := auth.ThrottleSource(s.origin.ClientIP(httpcore.RequestFrom(ctx)))
	if !s.invitationPreviewLimiter.Allow(source) {
		return nil, tooManyInvitationAttempts()
	}
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
	if value.Kind == access.KindLibrary {
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
		Password string `json:"password" format:"password" minLength:"8"`
	}
}

func (s *Handlers) redeemInvitation(ctx context.Context, in *redeemInvitationInput) (*meOutput, error) {
	if !s.invitationRedeemLimiter.Allow(auth.LoginRateKey(s.origin.ClientIP(httpcore.RequestFrom(ctx)), in.Body.Username)) {
		return nil, tooManyInvitationAttempts()
	}
	preview, err := s.invitationRedemption.Preview(ctx, in.Body.Grant)
	if err != nil {
		return nil, invitationRedemptionError(err)
	}
	var token string
	var expires time.Time
	var user identitystore.User
	if preview.Kind == access.KindLibrary {
		token, expires, user, err = s.invitationRedemption.RedeemLibrary(
			ctx, in.Body.Grant, in.Body.Username, in.Body.Password)
	} else {
		token, expires, user, err = s.invitationRedemption.RedeemLocal(ctx, in.Body.Grant, in.Body.Password)
	}
	if err != nil {
		return nil, invitationRedemptionError(err)
	}
	return &meOutput{
		SetCookie: s.sessionCookie(httpcore.RequestFrom(ctx), token, expires),
		Body: meBody{ID: user.ID, Name: user.Name, Role: string(user.Role), Disabled: user.Disabled,
			Quota: user.Quota, AutoApprove: user.AutoApprove,
			Local: !user.MediaServerLinked, OfflineLogin: user.MediaServerLinked && user.PasswordHash != ""},
	}, nil
}

func invitationRedemptionError(err error) error {
	switch {
	case errors.Is(err, auth.ErrInvalidInvitation):
		return httpcore.Gone("Invitation unavailable", "This invitation is invalid, expired, revoked, or already used. Ask an administrator for a new invitation.")
	case errors.Is(err, auth.ErrWeakPassword):
		return errPasswordTooShort()
	case errors.Is(err, auth.ErrInvalidCredentials):
		return httpcore.Unauthorized("Account couldn't be verified", "Those Library credentials couldn't verify the invited account. Check them and try again.")
	case errors.Is(err, auth.ErrInvitationProviderUnavailable):
		return httpcore.ServiceUnavailable("Library temporarily unavailable", "Loomarr must contact the Library before this account's first sign-in. Wait for it to return, then try again.")
	default:
		return err
	}
}
