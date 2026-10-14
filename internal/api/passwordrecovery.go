package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/auth"
)

func (s *Server) registerPasswordRecovery(api huma.API) {
	if s.passwordRecovery == nil && !s.schemaOnly {
		return
	}
	huma.Register(api, withRole(huma.Operation{
		OperationID: "request-password-recovery", Method: http.MethodPost,
		Path: "/v1/auth/password-recovery/request", Summary: "Request local password recovery",
		Tags: []string{"auth"}, DefaultStatus: http.StatusAccepted,
	}, RolePublic), s.requestPasswordRecovery)
	huma.Register(api, withRole(huma.Operation{
		OperationID: "preview-password-recovery", Method: http.MethodPost,
		Path: "/v1/auth/password-recovery/preview", Summary: "Preview a password recovery grant",
		Tags: []string{"auth"},
	}, RolePublic), s.previewPasswordRecovery)
	huma.Register(api, withRole(huma.Operation{
		OperationID: "redeem-password-recovery", Method: http.MethodPost,
		Path: "/v1/auth/password-recovery/redeem", Summary: "Set a new local password",
		Tags: []string{"auth"}, DefaultStatus: http.StatusNoContent,
	}, RolePublic), s.redeemPasswordRecovery)
}

type requestPasswordRecoveryInput struct {
	Body struct {
		Username string `json:"username" maxLength:"200"`
	}
}

type requestPasswordRecoveryOutput struct {
	Body struct {
		Message string `json:"message"`
	}
}

func (s *Server) requestPasswordRecovery(
	ctx context.Context,
	in *requestPasswordRecoveryInput,
) (*requestPasswordRecoveryOutput, error) {
	rateKey := s.clientIP(requestFrom(ctx)) + "|" + strings.ToLower(strings.TrimSpace(in.Body.Username))
	if err := s.passwordRecovery.Request(ctx, in.Body.Username, rateKey); err != nil {
		if errors.Is(err, auth.ErrRateLimited) {
			return nil, errTooManyRequests("Too many requests", "Wait a moment before requesting another password reset.")
		}
		return nil, err
	}
	out := &requestPasswordRecoveryOutput{}
	out.Body.Message = "If that account can be recovered here, Loomarr will send a password reset email."
	return out, nil
}

type passwordRecoveryBearerInput struct {
	Body struct {
		Grant string `json:"grant" format:"password"`
	}
}

type previewPasswordRecoveryOutput struct {
	Body struct {
		ExpiresAt int64 `json:"expiresAt" doc:"Unix milliseconds"`
	}
}

func (s *Server) previewPasswordRecovery(
	ctx context.Context,
	in *passwordRecoveryBearerInput,
) (*previewPasswordRecoveryOutput, error) {
	value, err := s.passwordRecovery.Preview(ctx, in.Body.Grant, s.clientIP(requestFrom(ctx)))
	if err != nil {
		return nil, passwordRecoveryError(err)
	}
	out := &previewPasswordRecoveryOutput{}
	out.Body.ExpiresAt = value.ExpiresAt.UnixMilli()
	return out, nil
}

type redeemPasswordRecoveryInput struct {
	Body struct {
		Grant    string `json:"grant" format:"password"`
		Password string `json:"password" format:"password"`
	}
}

func (s *Server) redeemPasswordRecovery(
	ctx context.Context,
	in *redeemPasswordRecoveryInput,
) (*struct{}, error) {
	if err := s.passwordRecovery.Redeem(
		ctx, in.Body.Grant, in.Body.Password, s.clientIP(requestFrom(ctx)),
	); err != nil {
		return nil, passwordRecoveryError(err)
	}
	return &struct{}{}, nil
}

func passwordRecoveryError(err error) error {
	switch {
	case errors.Is(err, auth.ErrInvalidPasswordRecovery):
		return errGone("Password reset unavailable", "This password reset link is invalid, expired, revoked, or already used. Request a new one.")
	case errors.Is(err, auth.ErrWeakPassword):
		return errUnprocessable("Password too short", "Use at least 8 characters.")
	case errors.Is(err, auth.ErrRateLimited):
		return errTooManyRequests("Too many attempts", "Wait a moment before trying this password reset again.")
	default:
		return err
	}
}
