package api

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/danielgtaylor/huma/v2"

	catalogapi "github.com/loomarr/loomarr/internal/api/catalog"
	channelsapi "github.com/loomarr/loomarr/internal/api/channels"
	fillerapi "github.com/loomarr/loomarr/internal/api/filler"
	guideapi "github.com/loomarr/loomarr/internal/api/guide"
	"github.com/loomarr/loomarr/internal/api/httpcore"
	imagesapi "github.com/loomarr/loomarr/internal/api/images"
	libraryapi "github.com/loomarr/loomarr/internal/api/library"
	opsapi "github.com/loomarr/loomarr/internal/api/ops"
	"github.com/loomarr/loomarr/internal/api/people"
	playoutapi "github.com/loomarr/loomarr/internal/api/playout"
	proposalsapi "github.com/loomarr/loomarr/internal/api/proposals"
	setupapi "github.com/loomarr/loomarr/internal/api/setup"
	"github.com/loomarr/loomarr/internal/api/system"
	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/channels/viewing"
	"github.com/loomarr/loomarr/internal/filler/cliplibrary"
	"github.com/loomarr/loomarr/internal/filler/defectfloor"
	"github.com/loomarr/loomarr/internal/filler/records"
	"github.com/loomarr/loomarr/internal/filler/source"
	"github.com/loomarr/loomarr/internal/people/auth"
	"github.com/loomarr/loomarr/internal/platform/activity"
	"github.com/loomarr/loomarr/internal/platform/metrics"
	"github.com/loomarr/loomarr/internal/platform/settings"
	"github.com/loomarr/loomarr/internal/platform/store/proposalstore"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/proposals"
	"github.com/loomarr/loomarr/internal/proposals/accept"
	"github.com/loomarr/loomarr/internal/proposals/suggest"
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

// Options configures the API server.
type Options struct {
	Store                records.Store
	Auth                 httpcore.Authorizer
	Log                  *slog.Logger
	Metrics              *metrics.Recorder
	BackupSQLite         system.BackupStreamer // nil ⇒ /v1/backup returns 501 (Postgres)
	Ready                ReadyFunc
	Now                  func() time.Time                   // optional deterministic request clock for tests
	Login                people.LoginService                // /v1/auth/login + user disable; nil ⇒ routes absent
	Passwords            people.PasswordService             // /v1/auth/password + local account create/reset (§11); nil ⇒ routes absent
	Sessions             people.SessionManager              // /v1/auth/logout
	UserSync             people.UserSyncer                  // POST /v1/users/sync; nil ⇒ route absent
	Invitations          people.InvitationService           // /v1/invitations* (§11); nil ⇒ routes absent
	InvitationRedemption people.InvitationRedemptionService // public preview and explicit activation (§11)
	PasswordRecovery     people.PasswordRecoveryService     // public local-password recovery (§11)
	// InvitationDelivery adds explicit email send/resend and provider-safe delivery summaries.
	InvitationDelivery people.InvitationDeliveryService
	// AccessPublicURL supplies the recipient-reachable browser origin. It is read
	// at grant issuance so a hot-applied setting takes effect without restart.
	AccessPublicURL func() string
	// Devices wires /v1/auth/device/* (§11, Shield P1); nil ⇒ routes absent.
	Devices *auth.DeviceManager
	// DeviceLimiter bounds pairing starts and code-approval attempts; nil ⇒ unlimited, which is
	// acceptable only in tests. Production wires it at composition beside Devices.
	DeviceLimiter    *auth.RateLimiter
	CookieSecure     string                            // COOKIE_SECURE: auto|true|false (§11)
	TrustProxy       bool                              // TRUST_PROXY: honor X-Forwarded-For/-Proto only when true (§11)
	DevLogin         bool                              // LOOMARR_DEV_LOGIN=1 ⇒ mount POST /v1/auth/dev-login (§11); default false ⇒ route absent
	Pprof            bool                              // LOOMARR_PPROF=1 ⇒ mount /debug/pprof/* (§7); default false ⇒ routes absent
	MetricsToken     string                            // LOOMARR_METRICS_TOKEN[_FILE] (§7): the Prometheus scrape bearer; empty ⇒ /metrics refused (fail closed)
	Demo             people.DemoAccount                // LOOMARR_DEMO_USERNAME/_PASSWORD (#2268): the zero value ⇒ not a demo site
	Channels         guideapi.ChannelService           // /v1/channels* reconcile; nil ⇒ reconcile route absent
	LiveTV           setupapi.LiveTVService            // /v1/setup/*; nil ⇒ setup routes absent
	TunerRescanner   channelsapi.TunerRescanner        // §9 channel-list freshness; nil ⇒ best-effort poke unavailable
	TunarrConnect    setupapi.TunarrConnector          // /v1/setup/tunarr-connect + tunarr_library check (§6); nil ⇒ 501
	ArrDiscovery     setupapi.ArrDiscovery             // /v1/setup/arrs-from-requester; nil ⇒ fills nothing
	Suggest          SuggestService                    // /v1/proposals submit; nil ⇒ submit route 501
	ProposalProgress proposalsapi.ProposalProgress     // live generation progress on GET /v1/proposal-jobs/{jobId}; nil ⇒ none
	ProposalWorkflow proposalsapi.ProposalWorkflow     // authoritative /v1/proposal-jobs Journey reads
	Search           catalogapi.SearchService          // /v1/search; nil ⇒ search route 501
	MovieCollections libraryapi.MovieCollectionService // /v1/movie-collections; nil ⇒ route 501
	Collections      libraryapi.CollectionService      // /v1/library/collections (§2.2); nil ⇒ route 501
	LibraryRoots     libraryapi.LibraryRootService     // /v1/library/roots*, path-map, status (#2151); nil ⇒ 409
	LibraryHooks     libraryapi.LibraryHookService     // /v1/library/hooks/* (#2151); nil ⇒ 409
	IdeaLibrary      channelsapi.IdeaLibrary           ///v1/discovery/ideas (#1665); nil ⇒ route 501
	Icons            channelsapi.IconService           ///v1/channels/{id}/icon-suggestions (§icon P2); nil ⇒ 501
	// Images backs /v1/images* — the one pipeline every image travels (§22, V52). nil ⇒ the byte
	// route 404s and the record route reports the image absent, which is the honest answer for an
	// instance with no store behind it.
	Images          imagesapi.ImageService
	Events          opsapi.EventSource         // /v1/events SSE; nil ⇒ route 501
	Shutdown        <-chan struct{}            // generation lifetime; closes SSE so http.Server.Shutdown can drain
	Filler          fillerapi.Service          // /v1/filler sync/tag; nil ⇒ those routes 501
	SourceProviders *source.Registry           // /v1/filler/providers and source validation; nil ⇒ none registered
	FillerResearch  fillerapi.ResearchService  // optional structured-first web-search status and validation
	FillerDefects   *defectfloor.DefectReports // /v1/filler/defects* (#2034); nil ⇒ 501
	Pods            guideapi.PodPreviewer      // /v1/channels/{id}/pods preview (§12); nil ⇒ 501
	Taxonomy        fillerapi.TaxonomyEditor   // taxonomy impact + committed channel convergence
	// FillerLayout is the immutable storage topology applied to this server generation. Its zero
	// value means filler storage is unavailable; saved desired values do not replace it mid-run.
	FillerLayout cliplibrary.Layout
	// IntakeLanguages is the languages intake admits now, the channels' commercial languages
	// (language.IntakeSet), read live. Split review compares a proposal with it; nil reports none.
	IntakeLanguages func(context.Context) (string, error)
	SystemLLM       system.SystemLLMService // /v1/system/llm* model selection (§8.1); nil ⇒ routes 501
	// Database backs /v1/system/database* — the SQLite→PostgreSQL migration stepper
	// (§18, V11). Production wires it on both backends so a reconnect can observe the
	// successful cutover; nil is reserved for embeddings without migration status.
	Database   system.DatabaseService
	Encryption system.EncryptionService
	// Backups backs /v1/system/backups* — listing, downloading and writing the backups
	// on disk (§16, V12). nil ⇒ routes 501 (a Postgres install wires it nil).
	Backups system.BackupsService
	// SSO backs /v1/auth/sso/* (§11, V8); nil ⇒ the routes are not mounted.
	SSO people.SSOService
	// Activity records Dashboard feed lines (§12, V32); nil ⇒ nothing is recorded.
	Activity *activity.Recorder
	// DiagnosticEvents backs the bounded admin/agent diagnostic timeline.
	DiagnosticEvents opsapi.DiagnosticEventService
	// DiagnosticProcesses backs active/recent Process runs and bounded output.
	DiagnosticProcesses opsapi.DiagnosticProcessService
	// DiagnosticBundles backs preview and on-demand redacted ZIP creation.
	DiagnosticBundles opsapi.DiagnosticBundleService
	// DiagnosticCapture controls the recorder's process-local bounded debug window.
	DiagnosticCapture opsapi.DiagnosticCaptureService
	// ClientDiagnostics accepts bounded curated events from authenticated web/TV clients.
	ClientDiagnostics opsapi.ClientDiagnosticService
	// ClientCrashes backs the crash-report write and its admin reads (#1816); nil ⇒ unmounted.
	ClientCrashes opsapi.ClientCrashService
	// StartupReports backs the current/recent application-generation report.
	StartupReports opsapi.StartupReportService
	// Restart backs POST /v1/system/restart (§9.2, V13) — implemented over main's
	// generation loop. nil ⇒ 501, the honest answer for a handler with no loop behind it.
	Restart system.RestartService
	// RestartDrift names bootstrap and app-managed generation settings waiting on a restart
	// (config-design §3).
	RestartDrift             func() []string
	Jobs                     system.JobService                                         // /v1/jobs* background-job scheduler (§18.1); nil ⇒ routes 501
	Settings                 setupapi.SettingsService                                  // /v1/settings* (config-design §8); nil ⇒ routes 501
	NotificationDestinations setupapi.NotificationDestinationService                   // redacted product-delivery routing (§11)
	WebPushPublicKey         string                                                    // public half of the generated VAPID identity (§11)
	ProposalNotifications    suggest.ProposalNotifier                                  // best-effort product lifecycle publication (§11)
	DecisionQuality          proposalsapi.ProposalDecisionQuality                      // committed Proposal denial quality outcome (§17)
	BackendTransition        setupapi.BackendTransitioner                              // durable backend prepare/publish/retire coordinator
	BackendCheckpoint        func(context.Context) (httpcore.BackendCheckpoint, error) // durable checkpoint, once per operation
	Guide                    guideapi.GuideReader                                      // /v1/channels/now-next (§6, §9); nil ⇒ empty now/next
	Provision                people.Provisioner                                        // /v1/setup/bootstrap + /v1/users/import (§11); nil ⇒ routes absent
	ProposalOutlook          proposalsapi.ProposalOutlook
	Approver                 ProposalApprover   // atomic proposal + titles + channel gate (§7); required for approval
	Binder                   channelsapi.Binder // explicit channel intent/number helpers; not the approval gate
	// PlayoutObserver supplies operational snapshots and program progress.
	PlayoutObserver playoutapi.PlayoutObserver
	// PlayoutCapability returns the already-published host encoder capability. It must not
	// perform or await a probe on this observational request path.
	PlayoutCapability func() playout.Capacity
	// WatermarkPreview draws a channel's bug for the channel page's preview. Nil ⇒ the route 501s.
	WatermarkPreview channelsapi.WatermarkPreviewer
	// PlayoutSecret reads the generated `playout_token` (§11 device auth). A func so a
	// REGENERATED token takes effect without a restart. Nil ⇒ playout routes fail closed.
	PlayoutSecret func() string
	// PlayoutSecretCurrent reads the durable token at a request boundary. Production wires
	// this for Postgres replica coherence; nil preserves the local/SQLite seam above.
	PlayoutSecretCurrent func(context.Context) (string, error)
	// PlayoutResolver describes channels for the formats, play-url and filler handlers.
	PlayoutResolver playoutapi.PlayoutResolver
	// Playout is the one playback seam for MPEG-TS and HLS (§9.1 V56).
	Playout playoutapi.Playout
	// PlayoutGuide resolves programme timelines for the XMLTV guide (§9.1). Nil ⇒ the route 501s.
	PlayoutGuide playoutapi.PlayoutGuide
	// TimelineThumbs resolves a TMDB preview image per programme for the Watch timeline (§9.1 V47).
	// Nil ⇒ the strip renders with no images (TMDB optional, and it never fails the timeline).
	TimelineThumbs guideapi.TimelineThumbResolver
	// ResidentLLMVRAM reports the VRAM the LLM currently holds + its model tag, from a live Ollama
	// /api/ps probe (§9.1 V47 doctor). Powers the doctor's TRUE contention header. Nil for a hosted
	// provider or an install without a local LLM; the composition root wires it to llm ListResident.
	ResidentLLMVRAM func(ctx context.Context) (gib float64, model string)
	// LiveSettings answers a setting's live resolved value so feature routes gate on the
	// CURRENT config (a saved connection enables the route with no restart, §8.1). Routes read
	// it through the typed handles — `settings.GuideTimezone.Get(s.liveSettings)` — so a key
	// cannot be misspelt or read as the wrong kind. The composition root passes its settings
	// view; unit tests that omit it (nil) skip the live-config gates, and a handle on a nil
	// reader answers the setting's declared default.
	LiveSettings settings.Reader
	// LibraryConfigured atomically reports whether the complete live media-server
	// {flavor,url,token} connection is usable. The composition root supplies it; unit tests
	// that wire a concrete adapter may omit it.
	LibraryConfigured func() bool
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
