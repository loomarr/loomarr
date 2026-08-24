package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/diagnostics"
)

// DiagnosticEventService is the one read capability the HTTP adapter needs from the diagnostics
// module. Authorization and content negotiation stay here; bounds, cursors, and projection remain
// behind the module's interface.
type DiagnosticEventService interface {
	Query(context.Context, diagnostics.EventQuery) (diagnostics.EventPage, error)
}

// DiagnosticProcessService is the one bounded Process-run read capability exposed by HTTP.
type DiagnosticProcessService interface {
	Query(context.Context, diagnostics.ProcessQuery) (diagnostics.ProcessPage, error)
	Get(context.Context, string) (diagnostics.ProcessDetail, error)
	Output(context.Context, string) (diagnostics.ProcessOutput, error)
}

// DiagnosticBundleService owns preview and ZIP assembly so HTTP cannot drift selection, redaction,
// or safety bounds between the two operations.
type DiagnosticBundleService interface {
	Preview(context.Context, diagnostics.BundleSelection) (diagnostics.BundlePreview, error)
	Build(context.Context, diagnostics.BundleSelection) (diagnostics.BundleResult, error)
}

// DiagnosticCaptureService controls the recorder's one process-local bounded debug window.
type DiagnosticCaptureService interface {
	VerboseCapture() diagnostics.VerboseCapture
	StartVerboseCapture(time.Duration, string, string) (diagnostics.VerboseCapture, error)
	StopVerboseCapture() diagnostics.VerboseCapture
}

// ClientDiagnosticService is the write half of Diagnostics. The HTTP adapter supplies only the
// server-derived actor; vocabulary, validation, rate limiting and Recorder admission stay behind
// the module interface.
type ClientDiagnosticService interface {
	Ingest(context.Context, string, diagnostics.ClientBatch) (int, error)
}

// StartupReportService exposes the current in-memory generation and recent retained reports.
type StartupReportService interface {
	Current() diagnostics.StartupReport
	Health() diagnostics.HealthReport
	Recent(context.Context, int) ([]diagnostics.StartupReport, error)
}

// HealthRefreshService invokes the same bounded probe runner used by the scheduler.
type HealthRefreshService interface {
	Refresh(context.Context) (diagnostics.HealthReport, error)
}

type currentHealthOutput struct {
	Body diagnostics.HealthReport
}

type startupReportsInput struct {
	Limit int `query:"limit" minimum:"1" maximum:"20" default:"10" doc:"Current plus recent generations to return"`
}

type startupReportsOutput struct {
	Body struct {
		Current diagnostics.StartupReport   `json:"current"`
		Items   []diagnostics.StartupReport `json:"items"`
	}
}

type diagnosticEventsInput struct {
	From              int64              `query:"from" minimum:"0" doc:"Window start as Unix epoch milliseconds; defaults to one hour before to"`
	To                int64              `query:"to" minimum:"0" doc:"Window end as Unix epoch milliseconds; defaults to now"`
	Limit             int                `query:"limit" minimum:"1" maximum:"200" default:"100" doc:"Page size"`
	Cursor            string             `query:"cursor" maxLength:"512" doc:"Opaque cursor returned by the previous page"`
	Level             diagnostics.Level  `query:"level" enum:"debug,info,warn,error" doc:"Exact severity"`
	Source            diagnostics.Source `query:"source" enum:"server,web,android_tv" doc:"Exact observing runtime"`
	Event             string             `query:"event" maxLength:"128" doc:"Exact stable event name"`
	Subsystem         string             `query:"subsystem" maxLength:"128" doc:"Exact subsystem"`
	RequestID         string             `query:"requestId" maxLength:"128" doc:"Exact API request correlation id"`
	PlaybackSessionID string             `query:"playbackSessionId" maxLength:"128" doc:"Exact playback-session correlation id"`
	ChannelID         string             `query:"channelId" maxLength:"128" doc:"Exact Channel correlation id"`
	ScheduleBlockID   string             `query:"scheduleBlockId" maxLength:"128" doc:"Exact schedule-block correlation id"`
	JobID             string             `query:"jobId" maxLength:"128" doc:"Exact Job correlation id"`
	ProcessRunID      string             `query:"processRunId" maxLength:"128" doc:"Exact Process-run correlation id"`
	InstanceID        string             `query:"instanceId" maxLength:"128" doc:"Exact Loomarr instance correlation id"`
	Text              string             `query:"text" maxLength:"256" doc:"Case-insensitive text match across event, message, subsystem, and attributes"`
}

type diagnosticProcessesInput struct {
	From      int64                     `query:"from" minimum:"0" doc:"Window start as Unix epoch milliseconds; defaults to one hour before to"`
	To        int64                     `query:"to" minimum:"0" doc:"Window end as Unix epoch milliseconds; defaults to now"`
	Limit     int                       `query:"limit" minimum:"1" maximum:"200" default:"100" doc:"Page size"`
	Cursor    string                    `query:"cursor" maxLength:"512" doc:"Opaque cursor returned by the previous page"`
	Status    diagnostics.ProcessStatus `query:"status" enum:"running,succeeded,failed,cancelled" doc:"Exact lifecycle status"`
	Purpose   string                    `query:"purpose" maxLength:"128" doc:"Exact process purpose"`
	ChannelID string                    `query:"channelId" maxLength:"128" doc:"Exact Channel correlation id"`
	JobID     string                    `query:"jobId" maxLength:"128" doc:"Exact Job correlation id"`
}

type diagnosticProcessInput struct {
	ID string `path:"id" maxLength:"128" doc:"Opaque Process-run id"`
}

type diagnosticProcessPageOutput struct {
	Body diagnostics.ProcessPage
}

type diagnosticProcessDetailOutput struct {
	Body diagnostics.ProcessDetail
}

type diagnosticBundleInput struct {
	Body diagnostics.BundleSelection
}

type diagnosticBundlePreviewOutput struct {
	Body diagnostics.BundlePreview
}

type verboseCaptureInput struct {
	Body struct {
		DurationMinutes int    `json:"durationMinutes" minimum:"1" maximum:"15"`
		Subsystem       string `json:"subsystem,omitempty" maxLength:"128"`
		ChannelID       string `json:"channelId,omitempty" maxLength:"128"`
	}
}

type verboseCaptureOutput struct {
	Body diagnostics.VerboseCapture
}

type clientDiagnosticsInput struct {
	Body diagnostics.ClientBatch
}

type clientDiagnosticsOutput struct {
	Body struct {
		Accepted int `json:"accepted"`
	}
}

func (s *Server) registerDiagnostics(api huma.API) {
	if s.startupReports != nil || s.schemaOnly {
		huma.Register(api, withRole(huma.Operation{
			OperationID: "get-current-health", Method: http.MethodGet, Path: "/v1/diagnostics/health",
			Summary:     "Get current health",
			Description: "Returns the freshness-aware health of the running Loomarr generation from the same required checks used by readiness.",
			Tags:        []string{"diagnostics"},
		}, RoleAdmin), s.getCurrentHealth)
		huma.Register(api, withRole(huma.Operation{
			OperationID: "list-startup-reports", Method: http.MethodGet, Path: "/v1/diagnostics/startup-reports",
			Summary:     "List startup reports",
			Description: "Returns the live application-generation startup report and recent completed reports retained in Diagnostics.",
			Tags:        []string{"diagnostics"},
		}, RoleAdmin), s.listStartupReports)
	}
	if s.healthRefresh != nil || s.schemaOnly {
		huma.Register(api, withRole(huma.Operation{
			OperationID: "refresh-current-health", Method: http.MethodPost, Path: "/v1/diagnostics/health/refresh",
			Summary:     "Refresh current health",
			Description: "Runs the same bounded health probes used by Loomarr's named System health task and returns the refreshed source of truth.",
			Tags:        []string{"diagnostics"},
		}, RoleAdmin), s.refreshCurrentHealth)
	}
	if s.clientDiagnostics != nil || s.schemaOnly {
		huma.Register(api, withRole(huma.Operation{
			OperationID: "ingest-client-diagnostics", Method: http.MethodPost, Path: "/v1/diagnostics/client-events",
			Summary:     "Report curated client diagnostics",
			Description: "Accepts a bounded closed event batch from the signed-in web or Android TV client. Actor, receipt time, severity, subsystem, and retained attributes are server-owned.",
			Tags:        []string{"diagnostics"}, DefaultStatus: http.StatusAccepted,
		}, RoleMember), s.ingestClientDiagnostics)
	}
	op := huma.Operation{
		OperationID: "list-diagnostic-events",
		Method:      http.MethodGet,
		Path:        "/v1/diagnostics/events",
		Summary:     "List diagnostic events",
		Description: "Returns one bounded, newest-first diagnostic timeline page as typed JSON or the same records as NDJSON.",
		Tags:        []string{"diagnostics"},
		Responses: map[string]*huma.Response{
			"200": {
				Description: "A filtered diagnostic-event page",
				Headers: map[string]*huma.Param{
					"X-Next-Cursor": {
						Description: "Opaque next-page cursor; absent when this is the final page",
						Schema:      &huma.Schema{Type: huma.TypeString},
					},
				},
				Content: map[string]*huma.MediaType{
					"application/json": {
						Schema: huma.SchemaFromType(api.OpenAPI().Components.Schemas, reflect.TypeFor[diagnostics.EventPage]()),
					},
					"application/x-ndjson": {
						Schema: &huma.Schema{Type: huma.TypeString, Description: "One EventView JSON object per line"},
					},
				},
			},
		},
	}
	rawInputOp(api, op, RoleAdmin, s.diagnosticEventsHandler)
	huma.Register(api, withRole(huma.Operation{
		OperationID: "list-diagnostic-processes", Method: http.MethodGet, Path: "/v1/diagnostics/processes",
		Summary: "List diagnostic Process runs", Description: "Returns one bounded newest-first page of active and recent external media Process runs.",
		Tags: []string{"diagnostics"},
	}, RoleAdmin), s.listDiagnosticProcesses)
	huma.Register(api, withRole(huma.Operation{
		OperationID: "get-diagnostic-process", Method: http.MethodGet, Path: "/v1/diagnostics/processes/{id}",
		Summary: "Get a diagnostic Process run", Description: "Returns bounded lifecycle metadata and downsampled progress for one Process run.",
		Tags: []string{"diagnostics"},
	}, RoleAdmin), s.getDiagnosticProcess)
	rawInputOp(api, huma.Operation{
		OperationID: "get-diagnostic-process-output", Method: http.MethodGet, Path: "/v1/diagnostics/processes/{id}/output",
		Summary: "Read diagnostic Process output", Description: "Streams one Process run's bounded redacted diagnostic output as readable text.",
		Tags: []string{"diagnostics"}, Responses: map[string]*huma.Response{
			"200": {Description: "Bounded redacted Process output", Headers: map[string]*huma.Param{
				"X-Diagnostic-Discarded-Lines": {Description: "Count of output lines discarded before this retained view", Schema: &huma.Schema{Type: huma.TypeInteger}},
				"X-Diagnostic-Truncated":       {Description: "Whether earlier output was discarded", Schema: &huma.Schema{Type: huma.TypeBoolean}},
			}, Content: map[string]*huma.MediaType{"text/plain": {Schema: &huma.Schema{Type: huma.TypeString}}}},
		},
	}, RoleAdmin, s.diagnosticProcessOutputHandler)
	huma.Register(api, withRole(huma.Operation{
		OperationID: "preview-diagnostic-support-bundle", Method: http.MethodPost, Path: "/v1/diagnostics/support-bundle/preview",
		Summary: "Preview a Support bundle", Description: "Returns the entries, counts, estimated size, truncation, versions, drops, and redactions selected for a bounded Support bundle without creating the archive.",
		Tags: []string{"diagnostics"},
	}, RoleAdmin), s.previewDiagnosticBundle)
	rawInputOp(api, huma.Operation{
		OperationID: "download-diagnostic-support-bundle", Method: http.MethodPost, Path: "/v1/diagnostics/support-bundle",
		Summary: "Download a Support bundle", Description: "Builds and downloads one bounded redacted ZIP from the same explicit selection accepted by preview.",
		Tags: []string{"diagnostics"}, Responses: map[string]*huma.Response{"200": {Description: "Redacted Support bundle ZIP", Content: map[string]*huma.MediaType{"application/zip": {Schema: &huma.Schema{Type: huma.TypeString, Format: "binary"}}}}},
	}, RoleAdmin, s.diagnosticBundleHandler)
	huma.Register(api, withRole(huma.Operation{
		OperationID: "get-diagnostic-verbose-capture", Method: http.MethodGet, Path: "/v1/diagnostics/verbose-capture",
		Summary: "Get verbose capture", Description: "Returns the current process-local bounded debug-capture window.", Tags: []string{"diagnostics"},
	}, RoleAdmin), s.getVerboseCapture)
	huma.Register(api, withRole(huma.Operation{
		OperationID: "start-diagnostic-verbose-capture", Method: http.MethodPost, Path: "/v1/diagnostics/verbose-capture",
		Summary: "Start verbose capture", Description: "Temporarily retains scoped debug evidence through the ordinary bounded and redacted recorder.", Tags: []string{"diagnostics"},
	}, RoleAdmin), s.startVerboseCapture)
	huma.Register(api, withRole(huma.Operation{
		OperationID: "stop-diagnostic-verbose-capture", Method: http.MethodDelete, Path: "/v1/diagnostics/verbose-capture",
		Summary: "Stop verbose capture", Description: "Immediately restores the default diagnostic retention threshold.", Tags: []string{"diagnostics"},
	}, RoleAdmin), s.stopVerboseCapture)
}

func (s *Server) previewDiagnosticBundle(ctx context.Context, input *diagnosticBundleInput) (*diagnosticBundlePreviewOutput, error) {
	if s.diagnosticBundles == nil {
		return nil, huma.Error501NotImplemented("Support bundles aren't available on this Loomarr generation.")
	}
	preview, err := s.diagnosticBundles.Preview(ctx, input.Body)
	if err != nil {
		if errors.Is(err, diagnostics.ErrInvalidBundleSelection) {
			return nil, errBadRequest("Invalid Support bundle selection", strings.TrimPrefix(err.Error(), diagnostics.ErrInvalidBundleSelection.Error()+": "))
		}
		if errors.Is(err, diagnostics.ErrBundleTooLarge) {
			return nil, huma.Error413RequestEntityTooLarge("The selected evidence exceeds the 16 MiB Support bundle limit.")
		}
		s.log.Error("diagnostic bundle preview failed", "event", "diagnostics.bundle_preview_failed", "subsystem", "diagnostics", "err", err)
		return nil, huma.Error500InternalServerError("The Support bundle preview couldn't be created.")
	}
	return &diagnosticBundlePreviewOutput{Body: preview}, nil
}

func (s *Server) diagnosticBundleHandler(w http.ResponseWriter, r *http.Request, input *diagnosticBundleInput) {
	if s.diagnosticBundles == nil {
		s.writeProblem(w, r, http.StatusNotImplemented, "Support bundles unavailable", "Support bundles aren't available on this Loomarr generation.")
		return
	}
	result, err := s.diagnosticBundles.Build(r.Context(), input.Body)
	if err != nil {
		switch {
		case errors.Is(err, diagnostics.ErrInvalidBundleSelection):
			s.writeProblem(w, r, http.StatusBadRequest, "Invalid Support bundle selection", strings.TrimPrefix(err.Error(), diagnostics.ErrInvalidBundleSelection.Error()+": "))
		case errors.Is(err, diagnostics.ErrBundleTooLarge):
			s.writeProblem(w, r, http.StatusRequestEntityTooLarge, "Support bundle too large", "The selected evidence exceeds the 16 MiB Support bundle limit.")
		default:
			s.log.Error("diagnostic bundle failed", "event", "diagnostics.bundle_failed", "subsystem", "diagnostics", "err", err)
			s.writeProblem(w, r, http.StatusInternalServerError, "Couldn't create Support bundle", "The selected evidence couldn't be assembled.")
		}
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="loomarr-support-%s.zip"`, time.UnixMilli(result.Manifest.GeneratedAt).UTC().Format("20060102T150405Z")))
	w.Header().Set("Content-Length", fmt.Sprint(len(result.Content)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(result.Content)
}

func (s *Server) getVerboseCapture(_ context.Context, _ *struct{}) (*verboseCaptureOutput, error) {
	if s.diagnosticCapture == nil {
		return nil, huma.Error501NotImplemented("Verbose capture isn't available on this Loomarr generation.")
	}
	return &verboseCaptureOutput{Body: s.diagnosticCapture.VerboseCapture()}, nil
}

func (s *Server) startVerboseCapture(_ context.Context, input *verboseCaptureInput) (*verboseCaptureOutput, error) {
	if s.diagnosticCapture == nil {
		return nil, huma.Error501NotImplemented("Verbose capture isn't available on this Loomarr generation.")
	}
	capture, err := s.diagnosticCapture.StartVerboseCapture(
		time.Duration(input.Body.DurationMinutes)*time.Minute, input.Body.Subsystem, input.Body.ChannelID,
	)
	if errors.Is(err, diagnostics.ErrInvalidVerboseCapture) {
		return nil, errBadRequest("Invalid verbose capture", strings.TrimPrefix(err.Error(), diagnostics.ErrInvalidVerboseCapture.Error()+": "))
	}
	if err != nil {
		return nil, huma.Error503ServiceUnavailable("Verbose capture couldn't be started.")
	}
	return &verboseCaptureOutput{Body: capture}, nil
}

func (s *Server) stopVerboseCapture(_ context.Context, _ *struct{}) (*verboseCaptureOutput, error) {
	if s.diagnosticCapture == nil {
		return nil, huma.Error501NotImplemented("Verbose capture isn't available on this Loomarr generation.")
	}
	return &verboseCaptureOutput{Body: s.diagnosticCapture.StopVerboseCapture()}, nil
}

func (s *Server) listDiagnosticProcesses(
	ctx context.Context, input *diagnosticProcessesInput,
) (*diagnosticProcessPageOutput, error) {
	if s.diagnosticProcesses == nil {
		return nil, huma.Error501NotImplemented("Process diagnostics aren't available on this Loomarr generation.")
	}
	page, err := s.diagnosticProcesses.Query(ctx, diagnostics.ProcessQuery{
		From: input.From, To: input.To, Limit: input.Limit, Cursor: input.Cursor,
		Status: input.Status, Purpose: input.Purpose, ChannelID: input.ChannelID, JobID: input.JobID,
	})
	if err != nil {
		if errors.Is(err, diagnostics.ErrInvalidProcessQuery) {
			return nil, errBadRequest("Invalid Process filters", strings.TrimPrefix(err.Error(), diagnostics.ErrInvalidProcessQuery.Error()+": "))
		}
		s.log.Error("diagnostic process query failed", "event", "diagnostics.processes_query_failed", "subsystem", "diagnostics", "err", err)
		return nil, huma.Error500InternalServerError("The retained Process timeline couldn't be read.")
	}
	return &diagnosticProcessPageOutput{Body: page}, nil
}

func (s *Server) getDiagnosticProcess(
	ctx context.Context, input *diagnosticProcessInput,
) (*diagnosticProcessDetailOutput, error) {
	if s.diagnosticProcesses == nil {
		return nil, huma.Error501NotImplemented("Process diagnostics aren't available on this Loomarr generation.")
	}
	detail, err := s.diagnosticProcesses.Get(ctx, input.ID)
	if errors.Is(err, diagnostics.ErrProcessNotFound) {
		return nil, huma.Error404NotFound("Process run not found.")
	}
	if err != nil {
		s.log.Error("diagnostic process detail failed", "event", "diagnostics.process_detail_failed", "subsystem", "diagnostics", "process_run_id", input.ID, "err", err)
		return nil, huma.Error500InternalServerError("The retained Process run couldn't be read.")
	}
	return &diagnosticProcessDetailOutput{Body: detail}, nil
}

func (s *Server) diagnosticProcessOutputHandler(
	w http.ResponseWriter, r *http.Request, input *diagnosticProcessInput,
) {
	if s.diagnosticProcesses == nil {
		s.writeProblem(w, r, http.StatusNotImplemented, "Process diagnostics unavailable", "Process diagnostics aren't available on this Loomarr generation.")
		return
	}
	output, err := s.diagnosticProcesses.Output(r.Context(), input.ID)
	if errors.Is(err, diagnostics.ErrProcessNotFound) {
		s.writeProblem(w, r, http.StatusNotFound, "Process run not found", "The requested retained Process run does not exist.")
		return
	}
	if errors.Is(err, diagnostics.ErrProcessOutputUnavailable) {
		s.writeProblem(w, r, http.StatusGone, "Process output unavailable", "The Process run exists, but its bounded output is unavailable or retention-expired.")
		return
	}
	if err != nil {
		s.log.Error("diagnostic process output failed", "event", "diagnostics.process_output_failed", "subsystem", "diagnostics", "process_run_id", input.ID, "err", err)
		s.writeProblem(w, r, http.StatusInternalServerError, "Couldn't read Process output", "The retained Process output couldn't be read.")
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("X-Diagnostic-Discarded-Lines", fmt.Sprint(output.DiscardedLines))
	w.Header().Set("X-Diagnostic-Truncated", fmt.Sprint(output.Truncated))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(output.Content)
}

func (s *Server) ingestClientDiagnostics(
	ctx context.Context, input *clientDiagnosticsInput,
) (*clientDiagnosticsOutput, error) {
	if s.clientDiagnostics == nil {
		return nil, errNotImplemented("Client diagnostics unavailable",
			"Client diagnostic ingestion isn't available on this Loomarr generation.")
	}
	actorID := "api_token"
	if user, ok := userFrom(ctx); ok {
		actorID = user.ID
	}
	accepted, err := s.clientDiagnostics.Ingest(ctx, actorID, input.Body)
	if err != nil {
		switch {
		case errors.Is(err, diagnostics.ErrInvalidClientBatch):
			detail := strings.TrimPrefix(err.Error(), diagnostics.ErrInvalidClientBatch.Error()+": ")
			return nil, errBadRequest("Invalid client diagnostics", detail)
		case errors.Is(err, diagnostics.ErrClientRateLimited):
			return nil, errTooManyRequests("Client diagnostics rate limited",
				"This client sent too many diagnostic observations. Playback can continue while the bounded queue retries later.")
		case errors.Is(err, diagnostics.ErrClientUnavailable):
			return nil, errServiceUnavailable("Client diagnostics unavailable",
				"Playback can continue, but this diagnostic batch couldn't be retained.")
		default:
			s.log.Error("client diagnostic ingestion failed", "event", "diagnostics.client_ingestion_failed", "subsystem", "diagnostics", "err", err)
			return nil, apiErrWithCause(http.StatusInternalServerError, "Client diagnostics failed",
				"Playback can continue, but this diagnostic batch couldn't be accepted.", err)
		}
	}
	out := &clientDiagnosticsOutput{}
	out.Body.Accepted = accepted
	return out, nil
}

func (s *Server) getCurrentHealth(_ context.Context, _ *struct{}) (*currentHealthOutput, error) {
	if s.startupReports == nil {
		return nil, huma.Error501NotImplemented("Current Health isn't available on this Loomarr generation.")
	}
	return &currentHealthOutput{Body: s.startupReports.Health()}, nil
}

func (s *Server) refreshCurrentHealth(ctx context.Context, _ *struct{}) (*currentHealthOutput, error) {
	if s.healthRefresh == nil {
		return nil, huma.Error501NotImplemented("Current Health refresh isn't available on this Loomarr generation.")
	}
	health, err := s.healthRefresh.Refresh(ctx)
	if err != nil {
		s.log.Error("current health refresh failed", "event", "diagnostics.health_refresh_failed", "subsystem", "diagnostics", "err", err)
		return nil, huma.Error500InternalServerError("Current Health couldn't be refreshed.")
	}
	return &currentHealthOutput{Body: health}, nil
}

func (s *Server) listStartupReports(ctx context.Context, input *startupReportsInput) (*startupReportsOutput, error) {
	if s.startupReports == nil {
		return nil, huma.Error501NotImplemented("Startup reports aren't available on this Loomarr generation.")
	}
	items, err := s.startupReports.Recent(ctx, input.Limit)
	if err != nil {
		s.log.Error("startup report query failed", "event", "diagnostics.startup_query_failed", "subsystem", "diagnostics", "err", err)
		return nil, huma.Error500InternalServerError("The retained startup reports couldn't be read.")
	}
	out := &startupReportsOutput{}
	out.Body.Current = s.startupReports.Current()
	out.Body.Items = items
	return out, nil
}

func (s *Server) diagnosticEventsHandler(
	w http.ResponseWriter, r *http.Request, input *diagnosticEventsInput,
) {
	if s.diagnosticEvents == nil {
		s.writeProblem(w, r, http.StatusNotImplemented, "Diagnostics unavailable",
			"Diagnostic event retention isn't available on this Loomarr generation.")
		return
	}
	page, err := s.diagnosticEvents.Query(r.Context(), diagnostics.EventQuery{
		From: input.From, To: input.To, Limit: input.Limit, Cursor: input.Cursor,
		Level: input.Level, Source: input.Source, Event: input.Event, Subsystem: input.Subsystem,
		RequestID: input.RequestID, PlaybackSessionID: input.PlaybackSessionID,
		ChannelID: input.ChannelID, ScheduleBlockID: input.ScheduleBlockID,
		JobID: input.JobID, ProcessRunID: input.ProcessRunID, InstanceID: input.InstanceID,
		Text: input.Text,
	})
	if err != nil {
		if errors.Is(err, diagnostics.ErrInvalidEventQuery) {
			detail := strings.TrimPrefix(err.Error(), diagnostics.ErrInvalidEventQuery.Error()+": ")
			s.writeProblem(w, r, http.StatusBadRequest, "Invalid diagnostic filters", detail)
			return
		}
		s.log.Error("diagnostic event query failed", "event", "diagnostics.events_query_failed", "subsystem", "diagnostics", "err", err)
		s.writeProblem(w, r, http.StatusInternalServerError, "Couldn't load diagnostics",
			"The retained diagnostic timeline couldn't be read.")
		return
	}

	if acceptsNDJSON(r.Header.Get("Accept")) {
		if page.NextCursor != "" {
			w.Header().Set("X-Next-Cursor", page.NextCursor)
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.WriteHeader(http.StatusOK)
		if err := diagnostics.WriteNDJSON(w, page); err != nil {
			s.log.Error("diagnostic event stream failed", "event", "diagnostics.events_stream_failed", "subsystem", "diagnostics", "err", err)
		}
		return
	}
	if page.NextCursor != "" {
		w.Header().Set("X-Next-Cursor", page.NextCursor)
	}
	writeJSON(w, http.StatusOK, page)
}

func acceptsNDJSON(accept string) bool {
	for _, value := range strings.Split(strings.ToLower(accept), ",") {
		if strings.TrimSpace(strings.SplitN(value, ";", 2)[0]) == "application/x-ndjson" {
			return true
		}
	}
	return false
}
