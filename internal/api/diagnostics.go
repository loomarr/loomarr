package api

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/diagnostics"
)

// DiagnosticEventService is the one read capability the HTTP adapter needs from the diagnostics
// module. Authorization and content negotiation stay here; bounds, cursors, and projection remain
// behind the module's interface.
type DiagnosticEventService interface {
	Query(context.Context, diagnostics.EventQuery) (diagnostics.EventPage, error)
}

type diagnosticEventsInput struct {
	From              int64              `query:"from" minimum:"0" doc:"Window start as Unix epoch milliseconds; defaults to one hour before to"`
	To                int64              `query:"to" minimum:"0" doc:"Window end as Unix epoch milliseconds; defaults to now"`
	Limit             int                `query:"limit" minimum:"1" maximum:"200" default:"100" doc:"Page size"`
	Cursor            string             `query:"cursor" maxLength:"512" doc:"Opaque cursor returned by the previous page"`
	Level             diagnostics.Level  `query:"level" enum:"debug,info,warn,error" doc:"Exact severity"`
	Source            diagnostics.Source `query:"source" enum:"server,web,android_tv" doc:"Exact observing runtime"`
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

func (s *Server) registerDiagnostics(api huma.API) {
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
		Level: input.Level, Source: input.Source, Subsystem: input.Subsystem,
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
