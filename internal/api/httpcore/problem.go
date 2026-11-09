// Package httpcore is the plumbing every API area shares and none owns: RFC 7807 problem
// constructors and the transformer that stamps them with the request id, the request-id and
// security-header middleware, the caller's role and the authorizer seam, the route-role marker
// every registration carries, the raw-operation helpers, the wire-JSON never-null format and the
// docs page. It knows no Server, no store and no domain package, so the per-area handler packages
// under internal/api and the router in internal/api both build on it.
package httpcore

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

func StatusFromError(err error, fallback int) int {
	var statusErr huma.StatusError
	if errors.As(err, &statusErr) {
		return statusErr.GetStatus()
	}
	return fallback
}

// Loomarr's error copy convention (§7). Every problem a person can see is written for a
// person: a short Title-Case `title` and a full-sentence `detail` that says how to fix it —
// never a lowercase fragment, never a raw API path or stack. RFC 7807's other two fields
// carry the machine detail instead: `instance` is the request correlation id (so a
// developer greps the logs), and `type` optionally links to the troubleshooting docs.
//
// Use apiErr / the status shims below at every call site so title+detail are always set
// together. The bare huma.Error4xx helpers default the title to the HTTP status label
// ("Unauthorized") and take only a detail — which is what produced the old
// "Unauthorized / invalid username or password" stack.

// Problem builds a problem with an explicit friendly title and detail. docHref is an optional
// anchor into the embedded troubleshooting docs (e.g. "troubleshooting#llm"); it becomes the
// problem's `type`. The request `instance` id + server-side logging are added centrally by
// ErrorTransformer, so callers never handle them.
func Problem(status int, title, detail string, docHref ...string) huma.StatusError {
	m := &huma.ErrorModel{Status: status, Title: title, Detail: detail}
	if len(docHref) > 0 && docHref[0] != "" {
		m.Type = "/help?page=" + docHref[0]
	}
	return m
}

// ProblemWithCause is Problem for a call that has an underlying cause to record. The cause is
// passed to huma so ErrorTransformer logs it (keyed by the request id) — it is
// NOT shown to the client. Use this wherever the old call passed a trailing `err`.
func ProblemWithCause(status int, title, detail string, errs ...error) huma.StatusError {
	// Build via huma so the causes ride along as ErrorModel.Errors for the hook to log;
	// then overwrite the status-label title with our friendly one.
	m, ok := huma.NewError(status, detail, errs...).(*huma.ErrorModel)
	if !ok {
		return Problem(status, title, detail)
	}
	m.Title = title
	return m
}

// Status shims — the same set the handlers reach for, but title-first so the friendly
// summary is impossible to forget. detail should be a full sentence.
func BadRequest(title, detail string, doc ...string) huma.StatusError {
	return Problem(http.StatusBadRequest, title, detail, doc...)
}
func Unauthorized(title, detail string, doc ...string) huma.StatusError {
	return Problem(http.StatusUnauthorized, title, detail, doc...)
}

// (`errForbidden` lived here until 2026-08-10, breaking this family's symmetry on purpose. Its
// only caller was `requireAdmin`; 403s now come from the authorization middleware, which writes
// them via huma.WriteErr so the request-id stamping applies. A helper whose only caller is gone
// is dead weight that reads as a capability — the same reason `normalizeForMatch` went from
// internal/filler. Restore it the moment a handler genuinely needs to refuse mid-body.)
func NotFound(title, detail string, doc ...string) huma.StatusError {
	return Problem(http.StatusNotFound, title, detail, doc...)
}
func Gone(title, detail string, doc ...string) huma.StatusError {
	return Problem(http.StatusGone, title, detail, doc...)
}
func Conflict(title, detail string, doc ...string) huma.StatusError {
	return Problem(http.StatusConflict, title, detail, doc...)
}
func FeatureNotConfigured(title, detail string) huma.StatusError {
	return &huma.ErrorModel{
		Status: http.StatusConflict,
		Title:  title,
		Detail: detail,
		Type:   "feature_not_configured",
	}
}
func GroundingNotConfigured(title, detail string) huma.StatusError {
	return &huma.ErrorModel{
		Status: http.StatusConflict,
		Title:  title,
		Detail: detail,
		Type:   "grounding_not_configured",
	}
}
func Unprocessable(title, detail string, doc ...string) huma.StatusError {
	return Problem(http.StatusUnprocessableEntity, title, detail, doc...)
}
func TooManyRequests(title, detail string, doc ...string) huma.StatusError {
	return Problem(http.StatusTooManyRequests, title, detail, doc...)
}
func ServiceUnavailable(title, detail string, doc ...string) huma.StatusError {
	return Problem(http.StatusServiceUnavailable, title, detail, doc...)
}
func NotImplemented(title, detail string, doc ...string) huma.StatusError {
	return Problem(http.StatusNotImplemented, title, detail, doc...)
}

// Note: 5xx sites (BadGateway/Internal) always carry a cause, so they go through
// ProblemWithCause rather than a plain shim — no cause-less 5xx shim is needed.

// ErrorTransformer is the response transformer registered once in the huma config, so that
// EVERY problem — ours via Problem, huma's own validation errors, and anything from
// huma.WriteErr — is stamped with the request's correlation id (RFC 7807 `instance`) and
// logged with its full detail keyed by that id. This is the seam that lets us strip
// debugging text from client-facing copy without losing debuggability: the id on the
// response points a developer straight at the server log line.
//
// A returned StatusError is written directly by huma (bypassing NewErrorWithContext), but it
// STILL flows through the response Transformer — with the request context — so a transformer
// is the one seam that catches every error body. Registered once in humaConfig.
func ErrorTransformer(log *slog.Logger) huma.Transformer {
	return func(ctx huma.Context, _ string, v any) (any, error) {
		m, ok := v.(*huma.ErrorModel)
		if !ok {
			return v, nil // not an error body — leave it alone
		}
		reqID, path := "", ""
		if ctx != nil {
			reqID = RequestIDFrom(ctx.Context())
			if op := ctx.Operation(); op != nil {
				path = op.Method + " " + op.Path
			}
		}
		if reqID != "" {
			m.Instance = reqID
		}
		// Log server-side detail keyed by the id. 5xx are our errors; 4xx are the client's
		// mistake (info). m.Errors carries the wrapped causes we deliberately kept OUT of the
		// client-facing detail — this is where a developer reads them.
		eventName := "api.request_rejected"
		if m.Status >= 500 {
			eventName = "api.request_failed"
		}
		attrs := []any{
			"event", eventName, "subsystem", "api", "request_id", reqID, "instance", reqID,
			"status", m.Status, "path", path, "title", m.Title, "detail", m.Detail,
		}
		if len(m.Errors) > 0 {
			attrs = append(attrs, "cause", joinErrorDetails(m.Errors))
		}
		if m.Status >= 500 && ctx != nil && errors.Is(ctx.Context().Err(), context.Canceled) {
			// The client hung up mid-request; the handler's "context canceled" surfaced as a 500
			// but nobody is left to receive it and it is not our fault. Kept at debug for tracing.
			log.Debug("request cancelled by client", "event", "api.request_cancelled",
				"subsystem", "api", "request_id", reqID, "path", path)
		} else if m.Status >= 500 {
			log.Error("request failed", attrs...)
		} else if m.Status >= 400 {
			log.Info("request rejected", attrs...)
		}
		return m, nil
	}
}

// WriteJSON writes v as the response body through the wire-JSON format, so a nil slice or map is
// `[]` or `{}` on the wire, never null.
func WriteJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = WireJSONFormat.Marshal(w, v)
}

// WriteProblem sends an RFC 7807 problem+json body from a PLAIN mux handler (the ones Huma
// doesn't wrap: /v1/events, /v1/backup, the /v1 catch-all 404). It mirrors what the Huma
// path produces — friendly title+detail plus the request `instance` id — so a client sees
// the same shape everywhere. Logs 4xx/5xx server-side keyed by the id, like the Huma hook; a nil
// log writes the body and logs nothing.
func WriteProblem(log *slog.Logger, w http.ResponseWriter, r *http.Request, status int, title, detail string) {
	reqID := RequestIDFrom(r.Context())
	body := &huma.ErrorModel{Status: status, Title: title, Detail: detail, Instance: reqID}
	if log != nil {
		eventName := "api.request_rejected"
		if status >= 500 {
			eventName = "api.request_failed"
		}
		attrs := []any{
			"event", eventName, "subsystem", "api", "request_id", reqID, "instance", reqID,
			"status", status, "path", r.Method + " " + r.URL.Path, "detail", detail,
		}
		if status >= 500 {
			log.Error("request failed", attrs...)
		} else if status >= 400 {
			log.Info("request rejected", attrs...)
		}
	}
	WriteJSON(w, status, body)
}

func joinErrorDetails(errs []*huma.ErrorDetail) string {
	out := ""
	for i, e := range errs {
		if e == nil {
			continue
		}
		if i > 0 {
			out += "; "
		}
		out += e.Message
	}
	return out
}
