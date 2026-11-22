package agents

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/loomarr/loomarr/internal/api/httpcore"
	"github.com/loomarr/loomarr/internal/platform/diagnostics"
)

// Code Mode (#4486): two tools that give a strong model the whole API without loading every
// operation into its context. `search` finds operations in the OpenAPI document; `execute` runs a
// script whose only way out is loomarr.request, sent through the router as the paired admin.
//
// WHO GETS IT: an admin's device whose owner turned Code Mode on for it. Anyone else is not shown
// the tools, and naming them gets the protocol's "unknown tool". The role and the switch are both
// read with the token on every request, so demoting the owner or turning the switch off applies at
// the next call.
//
// Every request a script makes goes through the same routes and role checks as the HTTP API, so a
// script can do exactly what its admin can, no more. What it cannot reach is listed in
// scriptRefuses. Each run is audited as a Diagnostics event.

// maxConcurrentScripts bounds how many scripts run at once across the server. Each holds up to
// sandboxLimits.Memory of heap, so this is also the memory Code Mode can use at most.
const maxConcurrentScripts = 2

// scriptRefuses is what a script may not call even though its admin could: the MCP endpoint itself
// (a script running scripts), the OAuth sign-in, and the Code Mode switch (a script must not turn
// itself on for other devices).
func scriptRefuses(path string) bool {
	for _, p := range []string{"/v1/mcp", "/v1/oauth", codeModePath} {
		if path == p || strings.HasPrefix(path, p+"/") {
			return true
		}
	}
	return false
}

func addCodeModeTools(s *mcp.Server, h *Handlers, t *toolset, actor, device string) {
	mcp.AddTool(s, &mcp.Tool{Name: "search", Annotations: readOnly(),
		Description: "Find Loomarr API operations by what they do. Returns each match's method, path, summary, " +
			"parameters and the shape of its request and reply bodies, for writing an execute script."},
		func(_ context.Context, _ *mcp.CallToolRequest, in searchIn) (*mcp.CallToolResult, searchOut, error) {
			return nil, h.searchOperations(in), nil
		})
	mcp.AddTool(s, &mcp.Tool{Name: "execute", Annotations: changes(true),
		Description: "Run JavaScript against the Loomarr API as this admin. The code is the body of an async function: " +
			"use `await loomarr.request(method, path, body?)`, which returns {status, body} and does not throw on an " +
			"HTTP error, and `return` the result. `console.log` lines come back too. There is no network, file or " +
			"module access. Limits: " + fmt.Sprintf("%s, %d MiB, %d requests, %d KiB of output.",
			defaultSandboxLimits.Time, defaultSandboxLimits.Memory>>20, defaultSandboxLimits.Requests, defaultSandboxLimits.Output>>10) +
			" A script that changes things changes them for real: confirm with the person first."},
		func(ctx context.Context, _ *mcp.CallToolRequest, in executeIn) (*mcp.CallToolResult, executeOut, error) {
			out, err := h.executeScript(ctx, t, actor, device, in)
			return nil, out, err
		})
}

type searchIn struct {
	Query string `json:"query" jsonschema:"words describing what the operation does, e.g. 'approve channel request' or 'jobs history'"`
	Limit int    `json:"limit,omitempty" jsonschema:"at most this many operations (default 8, max 25)"`
}

type operationOut struct {
	OperationID string     `json:"operationId"`
	Method      string     `json:"method"`
	Path        string     `json:"path"`
	Summary     string     `json:"summary,omitempty"`
	Description string     `json:"description,omitempty"`
	Parameters  []paramOut `json:"parameters,omitempty"`
	Body        any        `json:"body,omitempty"`
	Reply       any        `json:"reply,omitempty"`
}

type paramOut struct {
	Name     string `json:"name"`
	In       string `json:"in"`
	Required bool   `json:"required,omitempty"`
	Type     any    `json:"type,omitempty"`
}

type searchOut struct {
	Operations []operationOut `json:"operations"`
	Total      int            `json:"total" jsonschema:"how many operations matched before the limit"`
}

type executeIn struct {
	Code string `json:"code" jsonschema:"the body of an async function; await loomarr.request(method, path, body?) and return the result"`
}

type executeOut struct {
	Result   any      `json:"result" jsonschema:"what the script returned"`
	Logs     []string `json:"logs,omitempty"`
	Requests []string `json:"requests" jsonschema:"each loomarr.request the script made, with its status"`
}

// searchOperations ranks the documented operations against the query's words. The document holds
// only public routes (hidden ones, like this endpoint, are left out of it), and Code Mode is
// admin-only, so every operation listed is one the caller may call.
func (h *Handlers) searchOperations(in searchIn) searchOut {
	limit := clampLimit(in.Limit, 8, 25)
	words := strings.Fields(strings.ToLower(in.Query))
	oapi := h.openAPI()
	if oapi == nil {
		return searchOut{Operations: []operationOut{}}
	}
	type scored struct {
		op    operationOut
		score int
	}
	var found []scored
	for path, item := range oapi.Paths {
		for method, op := range map[string]*huma.Operation{
			http.MethodGet: item.Get, http.MethodPost: item.Post, http.MethodPut: item.Put,
			http.MethodPatch: item.Patch, http.MethodDelete: item.Delete,
		} {
			if op == nil || op.Hidden {
				continue
			}
			text := strings.ToLower(strings.Join([]string{op.OperationID, op.Summary, op.Description, path, strings.Join(op.Tags, " ")}, " "))
			score := 0
			for _, w := range words {
				if strings.Contains(text, w) {
					score++
					if strings.Contains(strings.ToLower(op.OperationID+" "+op.Summary), w) {
						score++ // a word in the name or summary says more than one in the prose
					}
				}
			}
			if len(words) > 0 && score == 0 {
				continue
			}
			found = append(found, scored{op: describeOperation(oapi.Components.Schemas, method, path, op), score: score})
		}
	}
	slices.SortFunc(found, func(a, b scored) int {
		return cmp.Or(cmp.Compare(b.score, a.score), cmp.Compare(a.op.Path, b.op.Path), cmp.Compare(a.op.Method, b.op.Method))
	})
	out := searchOut{Operations: []operationOut{}, Total: len(found)}
	for i := 0; i < len(found) && i < limit; i++ {
		out.Operations = append(out.Operations, found[i].op)
	}
	return out
}

func describeOperation(reg huma.Registry, method, path string, op *huma.Operation) operationOut {
	o := operationOut{OperationID: op.OperationID, Method: method, Path: path, Summary: op.Summary,
		Description: truncateRunes(op.Description, 400)}
	for _, p := range op.Parameters {
		if p == nil || p.In == "header" || p.In == "cookie" {
			continue
		}
		o.Parameters = append(o.Parameters, paramOut{Name: p.Name, In: p.In, Required: p.Required, Type: shape(reg, p.Schema, 1)})
	}
	if op.RequestBody != nil {
		if mt := op.RequestBody.Content["application/json"]; mt != nil {
			o.Body = shape(reg, mt.Schema, 3)
		}
	}
	for _, code := range []string{"200", "201"} {
		if r := op.Responses[code]; r != nil {
			if mt := r.Content["application/json"]; mt != nil {
				o.Reply = shape(reg, mt.Schema, 3)
				break
			}
		}
	}
	return o
}

// shape is a schema cut down to what a script writer needs: field names and types, nested a few
// levels, with enums kept. Descriptions and validation details are left out to keep a search
// result small enough to read many of.
func shape(reg huma.Registry, s *huma.Schema, depth int) any {
	if s == nil {
		return nil
	}
	if s.Ref != "" && reg != nil {
		if r := reg.SchemaFromRef(s.Ref); r != nil {
			s = r
		}
	}
	switch s.Type {
	case "object":
		if depth <= 0 || len(s.Properties) == 0 {
			return "object"
		}
		m := make(map[string]any, len(s.Properties))
		for name, p := range s.Properties {
			if name == "$schema" {
				continue
			}
			m[name] = shape(reg, p, depth-1)
		}
		return m
	case "array":
		if depth <= 0 {
			return "array"
		}
		return []any{shape(reg, s.Items, depth-1)}
	case "":
		return "any"
	}
	if len(s.Enum) > 0 {
		vals := make([]string, 0, len(s.Enum))
		for _, e := range s.Enum {
			vals = append(vals, fmt.Sprint(e))
		}
		return s.Type + " (" + strings.Join(vals, " | ") + ")"
	}
	return s.Type
}

// errScriptsBusy is returned when the server is already running its maximum number of scripts.
var errScriptsBusy = errors.New("another script is running; try again in a moment")

// executeScript runs one script for this caller and records the run.
func (h *Handlers) executeScript(ctx context.Context, t *toolset, actor, device string, in executeIn) (executeOut, error) {
	select {
	case h.scripts <- struct{}{}:
		defer func() { <-h.scripts }()
	default:
		return executeOut{}, errScriptsBusy
	}
	start := time.Now()
	run, err := runScript(ctx, in.Code, defaultSandboxLimits, func(ctx context.Context, method, rawPath string, body json.RawMessage) (sandboxReply, error) {
		path, query, err := scriptTarget(rawPath)
		if err != nil {
			return sandboxReply{}, err
		}
		if scriptRefuses(path) {
			return sandboxReply{Status: http.StatusForbidden}, fmt.Errorf("a script cannot call %s", path)
		}
		var b any
		if body != nil {
			b = body
		}
		reply, err := t.d.Do(ctx, t.c, method, path, query, b)
		if err != nil {
			return sandboxReply{}, err
		}
		var decoded any
		if len(reply.Body) > 0 && json.Unmarshal(reply.Body, &decoded) != nil {
			decoded = string(reply.Body)
		}
		return sandboxReply{Status: reply.Status, Body: decoded}, nil
	})
	h.audit(ctx, actor, device, time.Since(start), run.Requests, err)
	out := executeOut{Logs: run.Logs, Requests: []string{}}
	if len(run.Result) > 0 {
		_ = json.Unmarshal(run.Result, &out.Result) // JSON.stringify made it, so it parses
	}
	for _, r := range run.Requests {
		out.Requests = append(out.Requests, fmt.Sprintf("%s %s → %d", r.Method, r.Path, r.Status))
	}
	if err != nil {
		// The requests already made are part of the answer: some of them may have changed things.
		if len(out.Requests) > 0 {
			return out, fmt.Errorf("%w (after %s)", err, strings.Join(out.Requests, ", "))
		}
		return out, err
	}
	return out, nil
}

// scriptTarget splits a script's path into a path and query, refusing anything that is not a path
// on this server's API.
//
// ⚠ NO PERCENT-ENCODING IN THE PATH. url.Parse decodes it once here, and the router parses the
// string again, so "/v1/m%2563p" would be checked as "/v1/m%63p" and served as /v1/mcp. Refusing
// '%' in the path makes the string scriptRefuses checks the very string the router sees. API path
// segments are ids and names that need no encoding; a query may still carry encoded values, since
// Do re-encodes it and the router routes on the path alone.
func scriptTarget(raw string) (string, url.Values, error) {
	pathPart, _, _ := strings.Cut(raw, "?")
	pathPart, _, _ = strings.Cut(pathPart, "#")
	if strings.ContainsAny(pathPart, "%\\") {
		return "", nil, fmt.Errorf("path %q must not contain %% or \\; put values that need encoding in the query", truncateRunes(raw, 80))
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "" || u.Host != "" || u.Fragment != "" || !strings.HasPrefix(u.Path, "/v1/") {
		return "", nil, fmt.Errorf("path %q must be an API path starting /v1/", truncateRunes(raw, 80))
	}
	for _, seg := range strings.Split(u.Path, "/") {
		if seg == "." || seg == ".." {
			return "", nil, fmt.Errorf("path %q must not contain . or .. segments", truncateRunes(raw, 80))
		}
	}
	return u.Path, u.Query(), nil
}

// audit records a run: who, which device, how long, and each request's method, path and status.
// Request and reply bodies are never recorded; nor is the script.
func (h *Handlers) audit(ctx context.Context, actor, device string, took time.Duration, reqs []sandboxRequest, err error) {
	if h.deps.Events == nil {
		return
	}
	calls := make([]string, 0, len(reqs))
	for _, r := range reqs {
		calls = append(calls, fmt.Sprintf("%s %s %d", r.Method, r.Path, r.Status))
	}
	level, msg, outcome := diagnostics.LevelInfo, "An agent ran a Code Mode script", "ok"
	if err != nil {
		level, msg, outcome = diagnostics.LevelWarn, "An agent's Code Mode script failed", truncateRunes(err.Error(), 200)
	}
	h.deps.Events.Record(ctx, diagnostics.Event{
		Level: level, Source: diagnostics.SourceServer, Subsystem: "agents", Name: "code_mode.execute",
		Message: msg, ActorID: actor, RequestID: httpcore.RequestIDFrom(ctx),
		Attributes: map[string]any{"device": device, "durationMs": took.Milliseconds(), "requests": calls, "outcome": outcome},
	})
}
