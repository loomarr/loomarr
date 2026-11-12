package api

import (
	"context"
	"log/slog"
	"sync"

	"github.com/danielgtaylor/huma/v2"

	guideapi "github.com/loomarr/loomarr/internal/api/guide"
	"github.com/loomarr/loomarr/internal/api/httpcore"
	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/channels/viewing"
	"github.com/loomarr/loomarr/internal/platform/store/proposalstore"
	"github.com/loomarr/loomarr/internal/proposals"
	"github.com/loomarr/loomarr/internal/proposals/accept"
)

// Server is the root of the API wiring: it carries the dependencies the request middleware and
// Router share across the subpackages' Handlers, and Router mounts the Huma API on a stdlib mux
// (§7.1, §14: code-first OpenAPI 3.1 via humago — no third-party router).
type Server struct {
	auth httpcore.Authorizer
	log  *slog.Logger
	// origin is COOKIE_SECURE and TRUST_PROXY: where a request came from, how cookies are marked (§11).
	origin httpcore.Origin
	// channels is the channel engine; the root reads it only for holidayCalendar.
	channels guideapi.ChannelService
	// viewing is who is watching what, from the players' own playlist polls (#1662). Always set
	// by Router; nil only on the schema-only exporter.
	viewing *viewing.Tracker
}

// SuggestService is the suggestion surface the API depends on (§8). Implemented
// by suggest.Service + the store; abstracted so the API doesn't couple to the
// worker internals.
type SuggestService interface {
	// Submit enqueues a suggestion job for an intent and returns the job id. It takes
	// the DOMAIN intent rather than flat args: the flat form was "dependency-light",
	// but this package already imports suggest (ProposalDTO carries proposals.Proposal),
	// and the flat mirror had silently dropped RuntimeTgt — a knob the suggester
	// honors (suggester.go prompt + scoring) that no client could set.
	Submit(ctx context.Context, intent proposals.Intent, createdBy string) (jobID string, err error)
	// Refine submits a new job for the channel with a refine-flavored intent, so its
	// proposal binds back to the same channel (§7 POST /v1/channels/{id}/refine) and is
	// listed as the refining admin's own request. Returns the job id to poll.
	Refine(ctx context.Context, channelID string, intent proposals.Intent, requestedBy string) (string, error)
	// SubmitBuilt queues a proposal that needed no generation (a library channel idea, #1720)
	// as the requester's own request, and returns its job id. The model never runs.
	SubmitBuilt(ctx context.Context, intent proposals.Intent, proposal proposals.Proposal, createdBy string) (jobID string, err error)
}

// holidayCalendar is the engine's holiday calendar, or the default (the US, UTC) without one.
func (s *Server) holidayCalendar() schedule.Calendar {
	if s.channels == nil {
		return schedule.Calendar{}
	}
	return s.channels.Calendar()
}

// ProposalApprover is the complete approval gate. Its implementation plans the
// channel and commits proposal, titles, and channel atomically before post-commit
// runtime work; a handler cannot reproduce or reorder that choreography.
type ProposalApprover interface {
	Approve(ctx context.Context, p proposalstore.Proposal, edit *accept.ApprovalEdit, approvedBy string) (accept.ApprovalResult, error)
	// ResolveManualLineup re-grounds a hand-picked lineup against current Library presence
	// (manual/AI-off channel creation, #1817 G2), splitting it into owned titles and titles to
	// acquire. It shares the one approval-addition resolver with Approve's edit.Add, so a
	// manually submitted proposal gets the same "client presence is not authority" guarantee.
	ResolveManualLineup(ctx context.Context, items []proposals.ProposalItem) (lineup, acquisitions []proposals.ProposalItem, err error)
}

// arrayNullableOnce guards the one write to Huma's package-level DefaultArrayNullable (see the
// note in humaConfig): the value is constant, and doing it once keeps parallel Router() builds
// from racing Huma's schema registry on it.
var arrayNullableOnce sync.Once

// humaConfig builds the OpenAPI 3.1 config with our metadata (§7.1).
func humaConfig() huma.Config {
	// ⚠ Arrays are NOT nullable (V53b). Huma's default is `true` because a Go nil slice really
	// does marshal to `null`, so out of the box every list field in this API was typed
	// `T[] | null` — 109 nullable type-unions against 4 plain arrays — and every client had to
	// handle two representations of "nothing", forever.
	//
	// ⚠ This flag alone would make the document LIE, and Huma says so in its own doc comment:
	// "any `nil` slice will still encode as `null` in JSON". It is honest because the API's
	// encoder (wireJSONFormat, set below) sends every nil slice as `[]`, and two guards hold it
	// there: `TestResponseSchemas_RequiredArraysNeverNull` encodes every response type's zero
	// value, and `TestResponses_ContainNoJSONNull` drives every parameterless GET against an
	// EMPTY store. Setting this without them would swap a truthful awkward spec for a tidy
	// false one.
	//
	// ⚠ Set HERE deliberately: `humaConfig` is the single constructor behind both the served API
	// (api.go) and the spec export (export.go), so the runtime and the document cannot disagree
	// about it. It is a package-level global in Huma rather than a Config field, so there is no
	// per-instance place to put it.
	//
	// ⚠ Through a sync.Once, NOT bare. `humaConfig` runs on every `Router()` build, and tests build
	// many routers IN PARALLEL (the device-pairing suite is the one that surfaced it): a bare write
	// here races Huma's schema registry, which READS this same global while another goroutine's
	// Router() writes it → `-race` failure. The value is a constant, so writing it once is
	// sufficient and correct; the Once makes that write happen-before every reader without changing
	// behavior. Kept inside humaConfig so the "single constructor owns it" invariant above holds.
	arrayNullableOnce.Do(func() { huma.DefaultArrayNullable = false })
	cfg := huma.DefaultConfig("Loomarr API", "0.1.0")
	// A fresh map: DefaultConfig hands out Huma's package-level DefaultFormats.
	cfg.Formats = map[string]huma.Format{"application/json": httpcore.WireJSONFormat, "json": httpcore.WireJSONFormat}
	cfg.Info.Description = "Turn a sentence into a self-maintaining virtual TV channel. " +
		"Every /v1 route requires a session cookie or Authorization: Bearer API_TOKEN (§7)."
	// Serve our own docs assets offline; Huma's default loads Stoplight from a
	// CDN which violates the offline rule (§7.1). We disable the built-in docs
	// path and mount our own at /v1/reference (api/ops/ops.go).
	cfg.DocsPath = ""
	// ⚠ The spec and schema paths move under /v1 with everything else. Huma writes SchemasPath
	// into every `$ref` in the document, so changing it rewrites references throughout — that is
	// expected, not drift. The bare /openapi* and /schemas/ prefixes stay in the SPA guard
	// (api.go) so a stale bookmark 404s instead of being handed index.html with a 200.
	cfg.OpenAPIPath = "/v1/openapi"
	cfg.SchemasPath = "/v1/schemas"
	// Wire descriptions for domain types that must not import the HTTP framework
	// (durationwire.go). ⚠ Registered HERE rather than at the two `humago.New` call sites
	// (api.go, export.go) so the runtime API and the spec export cannot disagree about a type's
	// wire format — the same reason DocsPath and the paths above live in this one function.
	registerWireAliases(cfg.Components.Schemas)
	return cfg
}
