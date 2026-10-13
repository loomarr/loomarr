package api

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/diagnostics"
)

// ClientCrashService is the crash-report module behind the HTTP adapter (#1816). The adapter
// supplies only the server-derived reporter; validation, scrubbing, rate limiting and paging stay
// behind the module.
type ClientCrashService interface {
	Report(context.Context, diagnostics.ClientCrashReporter, diagnostics.ClientCrashReportInput) (string, error)
	List(ctx context.Context, limit int, cursor string) (diagnostics.ClientCrashPage, error)
	Get(context.Context, string) (diagnostics.ClientCrashReport, error)
}

// registerClientCrashes mounts the crash-report write and its admin reads. Called from
// registerDiagnostics, so the live API and the OpenAPI export cannot disagree about it.
func (s *Server) registerClientCrashes(api huma.API) {
	// RoleMember: any signed-in caller, and a paired device's token is a session. Huma's schema
	// validation is skipped so that contract violations, enums included, are the module's 400.
	// Huma still decodes the body: malformed JSON, a wrong JSON type or an unknown field (the
	// closed decoder refuses a body naming its own sender) remain its 422. MaxBodyBytes refuses
	// an oversized body with 413 before it is read.
	huma.Register(api, withRole(huma.Operation{
		OperationID: "report-client-crash", Method: http.MethodPost, Path: "/v1/diagnostics/client-crashes",
		Summary: "Report an app crash",
		Description: "Stores one crash report from a signed-in app. The server records the user and paired " +
			"device from the request's credential; the body never names them. Tokens, cookies, credential " +
			"headers and URL queries are scrubbed before storage. At most 128 KiB (413 above); rate limited " +
			"per device (429).",
		Tags: []string{"diagnostics"}, DefaultStatus: http.StatusAccepted,
		MaxBodyBytes: diagnostics.ClientCrashMaxBodyBytes, SkipValidateBody: true,
	}, RoleMember), s.reportClientCrash)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "list-client-crashes", Method: http.MethodGet, Path: "/v1/diagnostics/client-crashes",
		Summary:     "List app crash reports",
		Description: "Admin only. Newest first, without stacks, breadcrumbs or native payloads. " + operatorAPI,
		Tags:        []string{"diagnostics"},
	}, RoleAdmin), s.listClientCrashes)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "get-client-crash", Method: http.MethodGet, Path: "/v1/diagnostics/client-crashes/{id}",
		Summary:     "Get one app crash report",
		Description: "Admin only. The full report, including stack, breadcrumbs and native payload.",
		Tags:        []string{"diagnostics"},
	}, RoleAdmin), s.getClientCrash)
}

type clientCrashReportInput struct {
	Body diagnostics.ClientCrashReportInput
}

type clientCrashReportOutput struct {
	Body struct {
		ID string `json:"id" doc:"The stored report's id"`
	}
}

type clientCrashListInput struct {
	Limit  int    `query:"limit" minimum:"1" maximum:"100" default:"50" doc:"Page size"`
	Cursor string `query:"cursor" maxLength:"512" doc:"Opaque cursor returned by the previous page"`
}

type clientCrashListOutput struct {
	Body diagnostics.ClientCrashPage
}

type clientCrashInput struct {
	ID string `path:"id" maxLength:"128" doc:"Crash report id"`
}

type clientCrashOutput struct {
	Body diagnostics.ClientCrashReport
}

func (s *Server) reportClientCrash(ctx context.Context, in *clientCrashReportInput) (*clientCrashReportOutput, error) {
	if s.clientCrashes == nil {
		return nil, errNotImplemented("Crash reports unavailable", "Crash reporting isn't available on this Loomarr generation.")
	}
	reporter := s.clientCrashReporter(ctx)
	id, err := s.clientCrashes.Report(ctx, reporter, in.Body)
	switch {
	case errors.Is(err, diagnostics.ErrInvalidClientCrash):
		return nil, errBadRequest("Invalid crash report",
			strings.TrimPrefix(err.Error(), diagnostics.ErrInvalidClientCrash.Error()+": "))
	case errors.Is(err, diagnostics.ErrClientCrashRateLimited):
		return nil, errTooManyRequests("Crash reports rate limited",
			"This device sent too many crash reports. Keep the report and try again later.")
	case err != nil:
		s.log.Error("client crash report failed", "event", "diagnostics.client_crash_failed",
			"subsystem", "diagnostics", "err", err)
		return nil, apiErrWithCause(http.StatusInternalServerError, "Crash report failed",
			"The crash report couldn't be stored.", err)
	}
	// One line in the Diagnostics timeline points an admin at the report; the report itself
	// stays in its own table.
	s.log.Warn("client crash reported", "event", "client.crash_reported", "subsystem", "diagnostics",
		"crash_report_id", id, "kind", in.Body.Kind, "platform", in.Body.Platform)
	out := &clientCrashReportOutput{}
	out.Body.ID = id
	return out, nil
}

// clientCrashReporter is who sent the report, from the authenticated request only.
func (s *Server) clientCrashReporter(ctx context.Context) diagnostics.ClientCrashReporter {
	var reporter diagnostics.ClientCrashReporter
	user, ok := userFrom(ctx)
	if !ok {
		return reporter
	}
	reporter.UserID = user.ID
	deviceID, ok := deviceIDFrom(ctx)
	if !ok {
		return reporter
	}
	reporter.DeviceID = deviceID
	// The name is a label for the admin reading the report; failing to find it never fails it.
	if s.devices != nil {
		if devices, err := s.devices.ListFor(ctx, user.ID); err == nil {
			for _, device := range devices {
				if device.TokenHash == deviceID {
					reporter.DeviceName = device.DeviceName
					break
				}
			}
		}
	}
	return reporter
}

func (s *Server) listClientCrashes(ctx context.Context, in *clientCrashListInput) (*clientCrashListOutput, error) {
	if s.clientCrashes == nil {
		return nil, errNotImplemented("Crash reports unavailable", "Crash reporting isn't available on this Loomarr generation.")
	}
	page, err := s.clientCrashes.List(ctx, in.Limit, in.Cursor)
	if errors.Is(err, diagnostics.ErrInvalidClientCrashQuery) {
		return nil, errBadRequest("Invalid crash report query",
			strings.TrimPrefix(err.Error(), diagnostics.ErrInvalidClientCrashQuery.Error()+": "))
	}
	if err != nil {
		s.log.Error("client crash list failed", "event", "diagnostics.client_crash_list_failed",
			"subsystem", "diagnostics", "err", err)
		return nil, huma.Error500InternalServerError("Crash reports couldn't be read.")
	}
	return &clientCrashListOutput{Body: page}, nil
}

func (s *Server) getClientCrash(ctx context.Context, in *clientCrashInput) (*clientCrashOutput, error) {
	if s.clientCrashes == nil {
		return nil, errNotImplemented("Crash reports unavailable", "Crash reporting isn't available on this Loomarr generation.")
	}
	report, err := s.clientCrashes.Get(ctx, in.ID)
	if errors.Is(err, diagnostics.ErrClientCrashNotFound) {
		return nil, errNotFound("Crash report not found", "That crash report doesn't exist or has expired.")
	}
	if err != nil {
		s.log.Error("client crash read failed", "event", "diagnostics.client_crash_read_failed",
			"subsystem", "diagnostics", "crash_report_id", in.ID, "err", err)
		return nil, huma.Error500InternalServerError("The crash report couldn't be read.")
	}
	return &clientCrashOutput{Body: report}, nil
}
