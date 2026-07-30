package api

import (
	"context"
	"net/http"
	"runtime"
	"strconv"

	"github.com/danielgtaylor/huma/v2"

	"github.com/mantonx/loomarr/internal/buildinfo"
	"github.com/mantonx/loomarr/internal/store"
)

// The Dashboard's two lower panels (§12, V31 + V32).
//
// Services answers "is anything broken?"; Recent activity answers "what has been going on?".
// They are separate endpoints because they have genuinely different lifetimes — one is a live
// probe polled every 30s, the other is durable history — and folding them into one payload
// would tie the feed's freshness to a probe that can hang on an unreachable media server.

// ServiceRow is one line in the Services panel.
type ServiceRow struct {
	Name string `json:"name" doc:"Probe key: media_server, tunarr, llm, …"`
	// Target is what it connected to, so a failing row says WHICH endpoint. Empty for a probe
	// with no configurable target (TMDB).
	Target  string `json:"target,omitempty"`
	OK      bool   `json:"ok"`
	Hint    string `json:"hint,omitempty" doc:"Actionable detail when not OK"`
	DocHref string `json:"docHref,omitempty" doc:"Anchor into the embedded Troubleshooting docs"`
	// SettingsGroup is where "Fix →" sends the operator. A red dot that does not say where to
	// go is a puzzle, not a diagnosis (§12).
	SettingsGroup string `json:"settingsGroup,omitempty" doc:"Settings group owning this connection"`
}

// fixLocation maps a probe to the settings group that owns its connection. Derived from the
// registry's own grouping rather than invented here, so a probe cannot route somewhere its
// settings do not live.
var fixLocation = map[string]string{
	"media_server": "media_server",
	"requester":    "requester",
	"tunarr":       "tunarr",
	"llm":          "ai",
	"filler":       "filler",
}

// ServicesView is the whole panel.
type ServicesView struct {
	// Loomarr is the app's own row — version, backend and schema. The mock puts it first and
	// always green: it is the one component that must be up for anything else to be reported.
	Loomarr ServiceRow   `json:"loomarr"`
	Rows    []ServiceRow `json:"rows"`
}

func (s *Server) registerDashboardPanels(api huma.API) {
	huma.Register(api, withRole(huma.Operation{
		OperationID: "system-services", Method: http.MethodGet, Path: "/v1/system/services",
		Summary: "Every configured integration and whether it answers",
		Description: "Admin only. Runs the SAME connection checks as the wizard checklist and " +
			"/v1/system/reload — one probe implementation, so the three surfaces cannot disagree " +
			"about whether the media server is reachable. Polled by the Dashboard every 30s.",
		Tags: []string{"system"},
	}, RoleAdmin), s.systemServices)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "list-activity", Method: http.MethodGet, Path: "/v1/activity",
		Summary: "What Loomarr has been doing, newest first",
		Description: "Admin only. Reads the persisted activity table (§5), NOT the SSE bus — the bus " +
			"is deliberately lossy, so a feed built on it would drop entries exactly when the install " +
			"is busiest. Survives a restart.",
		Tags: []string{"system"},
	}, RoleAdmin), s.listActivity)
}

type systemServicesOutput struct {
	Body ServicesView
}

func (s *Server) systemServices(ctx context.Context, _ *struct{}) (*systemServicesOutput, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	info := buildinfo.Get()
	out := &systemServicesOutput{}

	// Loomarr's own row. Always OK by construction — if it were not, this request would not
	// have been answered — so it reports identity rather than health.
	backend := ""
	schema := int64(0)
	if s.store != nil {
		backend = string(store.DialectOf(s.store))
		schema = store.SchemaVersion(s.store)
	}
	out.Body.Loomarr = ServiceRow{
		Name:   "loomarr",
		OK:     true,
		Target: loomarrTarget(info.Version, backend, schema),
	}

	// ⚠ The SAME probe the wizard checklist and /v1/system/reload run. V31's gate names this
	// explicitly, and `runConnectionChecks` is why it holds structurally rather than by
	// convention: there is one implementation, so three surfaces cannot drift apart.
	out.Body.Rows = make([]ServiceRow, 0, len(connectionChecklist))
	for _, c := range s.runConnectionChecks(ctx) {
		out.Body.Rows = append(out.Body.Rows, ServiceRow{
			Name: c.Name, Target: c.Target, OK: c.OK, Hint: c.Hint, DocHref: c.DocHref,
			SettingsGroup: fixLocation[c.Name],
		})
	}
	return out, nil
}

// loomarrTarget renders the app's own identity line — "v0.9.3 · sqlite · schema 21" — matching
// how every other row shows what it is pointing at.
func loomarrTarget(version, backend string, schema int64) string {
	out := version
	if out == "" {
		out = "dev"
	}
	out += " · " + runtime.GOOS + "/" + runtime.GOARCH
	if backend != "" {
		out += " · " + backend
		if schema > 0 {
			out += " schema " + strconv.FormatInt(schema, 10)
		}
	}
	return out
}

type listActivityInput struct {
	Limit int `query:"limit" doc:"How many entries to return (default 20, max 200)"`
}

type listActivityOutput struct {
	Body struct {
		Activity []store.Activity `json:"activity"`
	}
}

func (s *Server) listActivity(ctx context.Context, in *listActivityInput) (*listActivityOutput, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	if s.store == nil {
		return nil, errNotImplemented("Activity unavailable", "There's no store configured yet.")
	}
	rows, err := s.store.ListActivity(ctx, in.Limit)
	if err != nil {
		return nil, apiErrWithCause(http.StatusBadGateway, "Couldn't read activity",
			"Loomarr couldn't read its activity history. Try again in a moment.", err)
	}
	out := &listActivityOutput{}
	out.Body.Activity = rows
	// A nil slice serializes as `null`, which a client must special-case before mapping. A
	// fresh install legitimately has no activity yet.
	if out.Body.Activity == nil {
		out.Body.Activity = []store.Activity{}
	}
	return out, nil
}
