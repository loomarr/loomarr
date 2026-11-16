package api

import (
	"context"
	"log/slog"
	"net/netip"
	"time"

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
	"github.com/loomarr/loomarr/internal/channels/viewing"
	"github.com/loomarr/loomarr/internal/filler/cliplibrary"
	"github.com/loomarr/loomarr/internal/filler/defectfloor"
	"github.com/loomarr/loomarr/internal/filler/records"
	"github.com/loomarr/loomarr/internal/filler/source"
	"github.com/loomarr/loomarr/internal/people/auth"
	"github.com/loomarr/loomarr/internal/platform/activity"
	"github.com/loomarr/loomarr/internal/platform/metrics"
	"github.com/loomarr/loomarr/internal/platform/settings"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/proposals/suggest"
)

// Options configures the API server. The fields here are the cross-cutting ones every area reads;
// each area's own services sit in one struct named for it (People, Setup, System, Playout, Channels,
// Guide, Proposals, Filler, Library, Ops), so "what wires the playout routes" is PlayoutOptions.
// Router hands each area package its slice of these, and an area that needs another's service
// (the playout status header reads System.LLM, the channel routes read Proposals.Approver) takes it
// from the owning struct rather than keeping a second copy.
type Options struct {
	Store   records.Store
	Auth    httpcore.Authorizer
	Log     *slog.Logger
	Metrics *metrics.Recorder
	Ready   ReadyFunc
	Now     func() time.Time // optional deterministic request clock for tests
	// Shutdown is the generation lifetime; it closes SSE so http.Server.Shutdown can drain.
	Shutdown <-chan struct{}
	// LiveSettings answers a setting's live resolved value so feature routes gate on the
	// CURRENT config (a saved connection enables the route with no restart, §8.1). Routes read
	// it through the typed handles — `settings.GuideTimezone.Get(s.liveSettings)` — so a key
	// cannot be misspelt or read as the wrong kind. The composition root passes its settings
	// view; unit tests that omit it (nil) skip the live-config gates, and a handle on a nil
	// reader answers the setting's declared default.
	LiveSettings settings.Reader
	// Settings backs /v1/settings* (config-design §8); nil ⇒ routes 501. The channel, filler and
	// proposal routes read it too, so it is not any one area's.
	Settings setupapi.SettingsService
	// Activity records Dashboard feed lines (§12, V32); nil ⇒ nothing is recorded.
	Activity *activity.Recorder
	// Images backs /v1/images* — the one pipeline every image travels (§22, V52). nil ⇒ the byte
	// route 404s and the record route reports the image absent, which is the honest answer for an
	// instance with no store behind it. The channel, guide and library routes draw their art from it.
	Images       imagesapi.ImageService
	CookieSecure string // COOKIE_SECURE: auto|true|false (§11)
	// TrustedProxies is `security.trusted_proxies`: forwarding headers are honored only from a
	// socket peer inside one of these ranges (§11). Empty trusts none.
	TrustedProxies []netip.Prefix

	People    PeopleOptions
	Setup     SetupOptions
	System    SystemOptions
	Playout   PlayoutOptions
	Channels  ChannelsOptions
	Guide     GuideOptions
	Proposals ProposalsOptions
	Filler    FillerOptions
	Library   LibraryOptions
	Ops       OpsOptions
}

// PeopleOptions wires the sign-in, session, invitation, device-pairing and SSO routes
// (/v1/auth/*, /v1/invitations*, /v1/users/*).
type PeopleOptions struct {
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
	DeviceLimiter *auth.RateLimiter
	DevLogin      bool               // LOOMARR_DEV_LOGIN=1 ⇒ mount POST /v1/auth/dev-login (§11); default false ⇒ route absent
	Demo          people.DemoAccount // LOOMARR_DEMO_USERNAME/_PASSWORD (#2268): the zero value ⇒ not a demo site
	// SSO backs /v1/auth/sso/* (§11, V8); nil ⇒ the routes are not mounted.
	SSO people.SSOService
	// Provision backs /v1/setup/bootstrap + /v1/users/import (§11); nil ⇒ routes absent.
	Provision people.Provisioner
}

// SetupOptions wires first-run setup, the media-server and arr connections, notification routing
// and the durable backend transition (/v1/setup/*, /v1/notification-destinations*).
type SetupOptions struct {
	LiveTV        setupapi.LiveTVService   // /v1/setup/*; nil ⇒ setup routes absent
	TunarrConnect setupapi.TunarrConnector // /v1/setup/tunarr-connect + tunarr_library check (§6); nil ⇒ 501
	ArrDiscovery  setupapi.ArrDiscovery    // /v1/setup/arrs-from-requester; nil ⇒ fills nothing
	// NotificationDestinations is the redacted product-delivery routing (§11).
	NotificationDestinations setupapi.NotificationDestinationService
	// WebPushPublicKey is the public half of the generated VAPID identity (§11).
	WebPushPublicKey string
	// BackendTransition is the durable backend prepare/publish/retire coordinator.
	BackendTransition setupapi.BackendTransitioner
	// BackendCheckpoint reads the durable checkpoint, once per operation. The channel and playout
	// routes consult it too, so a transition in flight holds their writes.
	BackendCheckpoint func(context.Context) (httpcore.BackendCheckpoint, error)
}

// SystemOptions wires the system routes: backups, the database migration stepper, encryption,
// the language-model selection, restart and the background-job scheduler (/v1/system/*, /v1/jobs*).
type SystemOptions struct {
	BackupSQLite system.BackupStreamer // nil ⇒ /v1/backup returns 501 (Postgres)
	// Backups backs /v1/system/backups* — listing, downloading and writing the backups
	// on disk (§16, V12). nil ⇒ routes 501 (a Postgres install wires it nil).
	Backups system.BackupsService
	// Database backs /v1/system/database* — the SQLite→PostgreSQL migration stepper
	// (§18, V11). Production wires it on both backends so a reconnect can observe the
	// successful cutover; nil is reserved for embeddings without migration status.
	Database   system.DatabaseService
	Encryption system.EncryptionService
	// LLM backs /v1/system/llm* model selection (§8.1); nil ⇒ routes 501. The playout status header
	// reads its GPU name and VRAM.
	LLM system.SystemLLMService
	// Restart backs POST /v1/system/restart (§9.2, V13) — implemented over main's
	// generation loop. nil ⇒ 501, the honest answer for a handler with no loop behind it.
	Restart system.RestartService
	// RestartDrift names bootstrap and app-managed generation settings waiting on a restart
	// (config-design §3).
	RestartDrift func() []string
	Jobs         system.JobService // /v1/jobs* background-job scheduler (§18.1); nil ⇒ routes 501
}

// PlayoutOptions wires the §9.1 streaming routes (MPEG-TS, HLS, the XMLTV guide, play-url) and the
// status projection (/v1/playout/*).
type PlayoutOptions struct {
	// Service is the one playback seam for MPEG-TS and HLS (§9.1 V56).
	Service playoutapi.Playout
	// Observer supplies operational snapshots and program progress. The system and ops
	// diagnostics read it too.
	Observer playoutapi.PlayoutObserver
	// Viewing is the household-viewing tracker the playlist polls feed (#1662). The composition
	// root shares it with the watch-history recorder (#3010); nil ⇒ Router makes its own. The
	// channel routes read the same tracker.
	Viewing *viewing.Tracker
	// Capability returns the already-published host encoder capability. It must not
	// perform or await a probe on this observational request path.
	Capability func() playout.Capacity
	// Secret reads the generated `playout_token` (§11 device auth). A func so a
	// REGENERATED token takes effect without a restart. Nil ⇒ playout routes fail closed.
	Secret func() string
	// SecretCurrent reads the durable token at a request boundary. Production wires
	// this for Postgres replica coherence; nil preserves the local/SQLite seam above.
	SecretCurrent func(context.Context) (string, error)
	// Resolver describes channels for the formats, play-url and filler handlers.
	Resolver playoutapi.PlayoutResolver
	// Guide resolves programme timelines for the XMLTV guide (§9.1). Nil ⇒ the route 501s.
	Guide playoutapi.PlayoutGuide
	// ResidentLLMVRAM reports the VRAM the LLM currently holds + its model tag, from a live Ollama
	// /api/ps probe (§9.1 V47 doctor). Powers the doctor's TRUE contention header. Nil for a hosted
	// provider or an install without a local LLM; the composition root wires it to llm ListResident.
	ResidentLLMVRAM func(ctx context.Context) (gib float64, model string)
}

// ChannelsOptions wires the channel routes: the engine, numbering, icons, discovery ideas and the
// watermark preview (/v1/channels*, /v1/discovery/*).
type ChannelsOptions struct {
	// Service is the channel engine (/v1/channels* reconcile; nil ⇒ reconcile route absent). The
	// guide and filler routes read it too.
	Service        guideapi.ChannelService
	TunerRescanner channelsapi.TunerRescanner // §9 channel-list freshness; nil ⇒ best-effort poke unavailable
	Binder         channelsapi.Binder         // explicit channel intent/number helpers; not the approval gate
	IdeaLibrary    channelsapi.IdeaLibrary    ///v1/discovery/ideas (#1665); nil ⇒ route 501
	Icons          channelsapi.IconService    ///v1/channels/{id}/icon-suggestions (§icon P2); nil ⇒ 501
	// WatermarkPreview draws a channel's bug for the channel page's preview. Nil ⇒ the route 501s.
	WatermarkPreview channelsapi.WatermarkPreviewer
}

// GuideOptions wires the guide reads: now/next, the pod preview and the Watch timeline
// (/v1/channels/now-next, /v1/channels/{id}/pods, the timeline).
type GuideOptions struct {
	Reader guideapi.GuideReader  // /v1/channels/now-next (§6, §9); nil ⇒ empty now/next
	Pods   guideapi.PodPreviewer // /v1/channels/{id}/pods preview (§12); nil ⇒ 501. The filler routes read it too.
	// TimelineThumbs resolves a TMDB preview image per programme for the Watch timeline (§9.1 V47).
	// Nil ⇒ the strip renders with no images (TMDB optional, and it never fails the timeline).
	TimelineThumbs guideapi.TimelineThumbResolver
}

// ProposalsOptions wires the proposal routes and the approval gate (/v1/proposals*,
// /v1/proposal-jobs*). The channel routes use Suggest and Approver as well.
type ProposalsOptions struct {
	Suggest  SuggestService                // /v1/proposals submit; nil ⇒ submit route 501
	Progress proposalsapi.ProposalProgress // live generation progress on GET /v1/proposal-jobs/{jobId}; nil ⇒ none
	Workflow proposalsapi.ProposalWorkflow // authoritative /v1/proposal-jobs Journey reads
	Outlook  proposalsapi.ProposalOutlook
	// Approver is the atomic proposal + titles + channel gate (§7); required for approval.
	Approver ProposalApprover
	// Notifications is the best-effort product lifecycle publication (§11).
	Notifications suggest.ProposalNotifier
	// DecisionQuality is the committed Proposal denial quality outcome (§17).
	DecisionQuality proposalsapi.ProposalDecisionQuality
}

// FillerOptions wires the filler routes: the pool, sources, research, defect reports and
// taxonomy (/v1/filler*).
type FillerOptions struct {
	Service         fillerapi.Service          // /v1/filler sync/tag; nil ⇒ those routes 501
	SourceProviders *source.Registry           // /v1/filler/providers and source validation; nil ⇒ none registered
	Research        fillerapi.ResearchService  // optional structured-first web-search status and validation
	Defects         *defectfloor.DefectReports // /v1/filler/defects* (#2034); nil ⇒ 501
	Taxonomy        fillerapi.TaxonomyEditor   // taxonomy impact + committed channel convergence
	// Layout is the immutable storage topology applied to this server generation. Its zero
	// value means filler storage is unavailable; saved desired values do not replace it mid-run.
	Layout cliplibrary.Layout
	// IntakeLanguages is the languages intake admits now, the channels' commercial languages
	// (language.IntakeSet), read live. Split review compares a proposal with it; nil reports none.
	IntakeLanguages func(context.Context) (string, error)
}

// LibraryOptions wires the library routes: roots, hooks, collections and title search
// (/v1/library/*, /v1/movie-collections, /v1/search).
type LibraryOptions struct {
	Search           catalogapi.SearchService          // /v1/search; nil ⇒ search route 501
	MovieCollections libraryapi.MovieCollectionService // /v1/movie-collections; nil ⇒ route 501
	Collections      libraryapi.CollectionService      // /v1/library/collections (§2.2); nil ⇒ route 501
	Roots            libraryapi.LibraryRootService     // /v1/library/roots*, path-map, status (#2151); nil ⇒ 409
	Hooks            libraryapi.LibraryHookService     // /v1/library/hooks/* (#2151); nil ⇒ 409
	// Configured atomically reports whether the complete live media-server
	// {flavor,url,token} connection is usable. The composition root supplies it; unit tests
	// that wire a concrete adapter may omit it. The setup and sign-in routes read it too.
	Configured func() bool
}

// OpsOptions wires the operator surface: the event stream, diagnostics, client reports and the
// profiler/metrics guards (/v1/events, /v1/diagnostics*, /metrics, /debug/pprof).
type OpsOptions struct {
	Events       opsapi.EventSource // /v1/events SSE; nil ⇒ route 501
	Pprof        bool               // LOOMARR_PPROF=1 ⇒ mount /debug/pprof/* (§7); default false ⇒ routes absent
	MetricsToken string             // LOOMARR_METRICS_TOKEN[_FILE] (§7): the Prometheus scrape bearer; empty ⇒ /metrics refused (fail closed)
	// StartupReports backs the current/recent application-generation report.
	StartupReports opsapi.StartupReportService
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
}
