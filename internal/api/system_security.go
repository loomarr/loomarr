package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// EncryptionService exposes non-secret envelope-encryption status and maintenance.
type EncryptionService interface {
	Status(context.Context) (EncryptionStatus, error)
	RotateDataKey(context.Context) error
}

// EncryptionStatus is safe to render in diagnostics and the Security settings page.
type EncryptionStatus struct {
	Enabled                    bool   `json:"enabled" doc:"Whether database secret encryption is active"`
	InstallationKeyFingerprint string `json:"installationKeyFingerprint" doc:"Non-secret installation-key fingerprint"`
	DataKeyCount               int    `json:"dataKeyCount" doc:"Active and retained readable data keys"`
}

func (s *Server) registerSystemSecurity(api huma.API) {
	huma.Register(api, withRole(huma.Operation{
		OperationID: "system-encryption-status", Method: http.MethodGet, Path: "/v1/system/security/encryption",
		Summary:     "Database secret-encryption status",
		Description: "Admin only. Reports the non-secret installation-key fingerprint and retained data-key count.",
		Tags:        []string{"system"},
	}, RoleAdmin), s.systemEncryptionStatus)
	huma.Register(api, withRole(huma.Operation{
		OperationID: "system-encryption-rotate-data-key", Method: http.MethodPost, Path: "/v1/system/security/encryption/rotate",
		Summary:     "Rotate the active data-encryption key",
		Description: "Admin only. New secret writes immediately use a fresh data key; prior keys remain readable.",
		Tags:        []string{"system"}, DefaultStatus: http.StatusNoContent,
	}, RoleAdmin), s.systemEncryptionRotate)
}

type encryptionStatusOutput struct{ Body EncryptionStatus }

func (s *Server) systemEncryptionStatus(ctx context.Context, _ *struct{}) (*encryptionStatusOutput, error) {
	if s.encryption == nil {
		return nil, huma.Error501NotImplemented("database secret encryption is unavailable")
	}
	status, err := s.encryption.Status(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("database secret-encryption status", err)
	}
	return &encryptionStatusOutput{Body: status}, nil
}

func (s *Server) systemEncryptionRotate(ctx context.Context, _ *struct{}) (*struct{}, error) {
	if s.encryption == nil {
		return nil, huma.Error501NotImplemented("database secret encryption is unavailable")
	}
	if err := s.encryption.RotateDataKey(ctx); err != nil {
		return nil, huma.Error500InternalServerError("rotate data-encryption key", err)
	}
	return &struct{}{}, nil
}
