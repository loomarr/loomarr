package api

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/danielgtaylor/huma/v2"

	channelsapi "github.com/loomarr/loomarr/internal/api/channels"
	fillerapi "github.com/loomarr/loomarr/internal/api/filler"
	guideapi "github.com/loomarr/loomarr/internal/api/guide"
	"github.com/loomarr/loomarr/internal/api/httpcore"
	"github.com/loomarr/loomarr/internal/api/people"
	playoutapi "github.com/loomarr/loomarr/internal/api/playout"
	proposalsapi "github.com/loomarr/loomarr/internal/api/proposals"
	"github.com/loomarr/loomarr/internal/api/system"
	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/channels/viewing"
	"github.com/loomarr/loomarr/internal/filler/cliplibrary"
	"github.com/loomarr/loomarr/internal/filler/defectfloor"
	"github.com/loomarr/loomarr/internal/filler/records"
	"github.com/loomarr/loomarr/internal/filler/source"
	"github.com/loomarr/loomarr/internal/media/encode"
	"github.com/loomarr/loomarr/internal/people/auth"
	"github.com/loomarr/loomarr/internal/people/notifications"
	"github.com/loomarr/loomarr/internal/platform/activity"
	"github.com/loomarr/loomarr/internal/platform/metrics"
	"github.com/loomarr/loomarr/internal/platform/settings"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
	"github.com/loomarr/loomarr/internal/platform/store/identitystore"
	"github.com/loomarr/loomarr/internal/platform/store/proposalstore"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/proposals"
	"github.com/loomarr/loomarr/internal/proposals/accept"
	"github.com/loomarr/loomarr/internal/proposals/suggest"
)

// Server holds the API dependencies and builds the Huma API on a stdlib mux
// (§7.1, §14: code-first OpenAPI 3.1 via humago — no third-party router).
type Server struct {
	store   records.Store
	auth    httpcore.Authorizer
	log     *slog.Logger
	metrics *metrics.Recorder
	// The /v1/auth/*, user, invitation, password and SSO routes are served by people.Handlers,
	// built from the same Options in Router.
	//
	// devices is the pairing handshake's manager (§11, Shield P1), also read by the request
	// middleware. Nil until configured.
	devices *auth.DeviceManager
	origin  httpcore.Origin // COOKIE_SECURE and TRUST_PROXY: where a request came from, how cookies are marked (§11)
	// devLogin mounts POST /v1/auth/dev-login — a credential-free admin sign-in for
	// development (§11). Set from LOOMARR_DEV_LOGIN at boot and read ONCE here, when
	// routes are registered: an unset flag means the route never exists, which is a
	// stronger guarantee than a handler that checks a value it could later re-read.
	devLogin bool
	// demo is the demo site's public sign-in (#2268), reported by GET /v1/setup/state.
	demo DemoAccount
	// channels/livetv wire /v1/channels* and /v1/setup/* (§7/§9); nil until
	// Phase 10 is configured.
	channels guideapi.ChannelService
	livetv   LiveTVService
	// tunarrConnect wires the media server as Tunarr's media source (§6) — backs
	// POST /v1/setup/tunarr-connect + the tunarr_library setup check. Nil ⇒ 501.
	tunarrConnect TunarrConnector
	// arrDiscovery reads the Sonarr and Radarr the requester already knows, for
	// POST /v1/setup/arrs-from-requester. Nil ⇒ the route fills nothing.
	arrDiscovery ArrDiscovery
	// suggest/search wire /v1/proposals*, /v1/search (§7.2/§8); nil until
	// Phase 11 is configured.
	suggest          SuggestService
	search           SearchService
	movieCollections MovieCollectionService
	// collections backs the scope.collections picker; nil ⇒ the route 501s, the same
	// nil-semantics every other optional service here uses.
	collections CollectionService
	// libraryRoots backs /v1/library/roots*, /v1/library/path-map and /v1/library/status (#2151);
	// nil ⇒ those routes 409 as not configured.
	libraryRoots LibraryRootService
	// libraryHooks backs /v1/library/hooks/* (#2151 decision 10); nil ⇒ those routes 409.
	libraryHooks LibraryHookService
	// images backs /v1/images* — the one pipeline every image travels (§22, V52). nil ⇒ the
	// byte route 404s and the record route reports the image absent, which is the honest answer
	// for an instance where the service is not wired.
	images   ImageService
	events   EventSource     // /v1/events SSE (Phase 11); nil ⇒ route 501
	shutdown <-chan struct{} // generation shutdown closes long-lived streams before HTTP drain
	taxonomy TaxonomyEditor  // taxonomy impact + graph convergence; nil keeps store-only test wiring
	// The jobs, database, encryption, backups, restart and help routes are served by
	// system.Handlers, built from the same Options in Router. A generation's start time
	// (§16 uptime) is stamped there per build, never in a package-level var: the restart loop
	// (§9.2) proved a package-level start time survives the rebuild and lies about uptime.
	//
	// systemLLM wires /v1/system/llm* (§8.1 model selection); nil ⇒ routes 501. It is also read here.
	systemLLM system.SystemLLMService
	// now is an optional request-clock seam for projections whose membership is time-bounded.
	// Production leaves it nil and uses time.Now; tests pin it to prove exact cutoff behaviour.
	now func() time.Time
	// viewing is who is watching what, from the players' own playlist polls (#1662). Always set
	// by Router; nil only on the schema-only exporter.
	viewing *viewing.Tracker
	// sso wires /v1/auth/sso/* — the OIDC credential path (§11, V8). nil ⇒ the routes are
	// NOT MOUNTED, which is the honest posture for an unconfigured provider: a
	// sign-in-with button that 501s is worse than one that is not offered.
	sso people.SSOService
	// activity records Dashboard feed lines (§12, V32). Nil-safe on the Recorder itself, so
	// a handler built without one (unit tests) records nothing rather than needing a guard
	// at every write point.
	activity *activity.Recorder
	// diagnosticEvents is the admin-only retained technical timeline. Nil keeps the route mounted
	// but truthfully unavailable for store-less generations.
	diagnosticEvents DiagnosticEventService
	// diagnosticProcesses owns bounded Process-run metadata, progress, and output reads.
	diagnosticProcesses DiagnosticProcessService
	// diagnosticBundles owns bounded preview and on-demand archive assembly.
	diagnosticBundles DiagnosticBundleService
	diagnosticCapture DiagnosticCaptureService
	// clientDiagnostics accepts a closed authenticated web/TV event set; it never grants reads.
	clientDiagnostics ClientDiagnosticService
	// clientCrashes takes crash reports from any signed-in app and serves them to admins (#1816).
	clientCrashes ClientCrashService
	// settings wires /v1/settings* + secrets regeneration (config-design §8);
	// nil ⇒ routes 501. Implemented by a thin adapter over settings.Service.
	settings                 SettingsService
	notificationDestinations NotificationDestinationService
	webPushPublicKey         string
	// backendTransition owns the durable setting mutation -> prepare -> publish -> retire
	// workflow for playout backend and URL changes. It serializes that entire workflow
	// across replicas; failures after mutation remain non-fatal follow-on failures.
	backendTransition BackendTransitioner
	// provision wires /v1/setup/bootstrap + /v1/users/import (§11); nil ⇒ routes
	// absent. Implemented by auth.Provisioner.
	provision Provisioner
	// liveSettings answers a setting's live resolved value (config-design §3 hot-apply).
	// The composition root always-constructs the feature services and passes this so
	// a route gates on the CURRENT config (a saved connection enables it with no
	// restart, §8.1). Nil in unit tests that wire deps directly — then the nil-dep
	// check alone gates, preserving the old contract.
	liveSettings settings.Reader
	// libraryConfigured resolves the complete live media-server connection from one settings
	// snapshot. A domain-specific seam keeps handlers from independently reading flavor, URL,
	// and token and accidentally observing a mixed generation during a concurrent save.
	libraryConfigured func() bool
	// ready is the same readiness /readyz reports, surfaced through the typed
	// /v1/system/version so the UI can show it without an untyped fetch. nil ⇒ ready.
	ready          ReadyFunc
	startupReports StartupReportService

	// playoutObserver supplies operational snapshots and program progress (§9.1, §12).
	playoutObserver playoutapi.PlayoutObserver
	// playoutCapability supplies the retained encoder probe result without starting a probe.
	// Unlike live session stats, it remains available while playout is idle.
	playoutCapability func() playout.Capacity
	// playoutSecret reads the generated `playout_token` (§11 device auth). A func rather
	// than the value so a REGENERATED token takes effect without a restart — rotation is
	// an operator action the UI offers, and a cached value would keep authorizing the old
	// one. Nil ⇒ every playout route 404s (fail closed, never serve unauthenticated).
	playoutSecret func() string
	// playoutSecretCurrent is the durable Postgres-replica read. Production prefers it at
	// request security boundaries so a rotation handled by another process invalidates the
	// old token and admits the new one immediately. Tests and SQLite use playoutSecret.
	playoutSecretCurrent func(context.Context) (string, error)
	// playoutResolver describes channels for the formats, play-url and filler handlers. Nil ⇒ those
	// report playout as not configured.
	playoutResolver playoutapi.PlayoutResolver
	// playout is the one playback seam for MPEG-TS and HLS (§9.1 V56).
	playout playoutapi.Playout
	// playoutFont labels the offline card; nil ⇒ unlabelled, never a failed encode.
	playoutFont func() string
	// playoutTonemap reports whether this ffmpeg build can tone-map HDR→SDR; nil ⇒ no, so an HDR
	// source transcodes untone-mapped rather than emitting a filter the build would reject at
	// graph-init. Same fail-safe direction as playoutFont, for the same reason.
	playoutTonemap    func() bool
	playoutGPUTonemap func() encode.GPUFilters
	// reclaimVRAM frees GPU memory the encoders need — in practice, evicts the resident local LLM
	// (§8.2 Evictor, §9.1 V47 retry ladder). Called ONLY when a hardware encode has already produced
	// nothing, so the common case never touches it. Nil ⇒ no local LLM to reclaim (a hosted provider,
	// or none), and the ladder skips straight from hardware to the software fallback.
	reclaimVRAM func(ctx context.Context)
	// residentLLMVRAM reports the VRAM the LLM ACTUALLY holds right now + its model tag, from a live
	// Ollama /api/ps probe (§9.1 V47 doctor). The doctor uses it for the true contention picture,
	// replacing an on-disk-size estimate that misreported an unloaded model as resident. Returns
	// (0, "") when nothing is resident or the provider is hosted; nil ⇒ the doctor omits the header.
	residentLLMVRAM func(ctx context.Context) (gib float64, model string)
	// schemaOnly is set ONLY by ExportOpenAPI (§7.1): it makes the register* funcs
	// emit every operation's SCHEMA into the spec even when its live service is nil,
	// so the exported `api/openapi.yaml` is complete (auth, bootstrap, import, sync)
	// and orval can type the whole surface. Handlers are never invoked during export,
	// so a nil service behind a registered op is harmless. Runtime nil-guarding is
	// unchanged — at runtime schemaOnly is false and an absent service still 404s.
	schemaOnly bool
}

// personNames resolves user ids to display names, one ListUsers per request.
func (s *Server) personNames(ctx context.Context) (httpcore.PersonNames, error) {
	return httpcore.LoadPersonNames(ctx, s.store)
}

// unconfigured reports whether a required string setting is empty right now. Safe when
// liveSettings is unset (unit tests) — reports false so a wired dep still serves.
func (s *Server) unconfigured(keys ...settings.StringKey) bool {
	if s.liveSettings == nil {
		return false
	}
	for _, k := range keys {
		if k.Get(s.liveSettings) == "" {
			return true
		}
	}
	return false
}

// libraryUnconfigured gates every optional media-library operation on the same complete,
// atomic {flavor,url,token} predicate. Nil preserves the unit-test convention that an
// explicitly wired adapter is usable without a settings runtime.
func (s *Server) libraryUnconfigured() bool {
	return s.libraryConfigured != nil && !s.libraryConfigured()
}

// SettingsService is the settings surface the API depends on (config-design §8).
// Implemented by a settings.Service adapter in the composition root. Its read
// side is the typed settings.Reader on Options.LiveSettings; this interface is the
// management surface. Secrets are masked here — a value never crosses this
// boundary (§4).
type SettingsService interface {
	// List returns every operator-managed setting with its resolved value (secrets
	// masked) + provenance + audit metadata (config-design §8).
	List(ctx context.Context) []SettingEntry
	// Patch applies per-key edits, returning a per-key result (saved | invalid |
	// pinned). Hot-applies on success. updatedBy is the admin's id (§3 audit).
	Patch(ctx context.Context, edits map[string]string, updatedBy string) []SettingResult
	// Clear drops one key's stored override (reverts to env/default) — the explicit
	// clear, and the only way to unset a secret (config-design §8/§9). The result
	// status maps to HTTP: invalid → 404, pinned → 409, saved → 204.
	Clear(ctx context.Context, key string) SettingResult
	// SetEnvOverride takes a key back from the environment, or hands it back
	// (config-design §3.1). Status maps to HTTP: unknown → 404, not_pinned → 409,
	// applied → 204. updatedBy is the admin's id, so the field's existing
	// "changed by … · when" line reports who overrode the deploy config.
	SetEnvOverride(ctx context.Context, key string, on bool, updatedBy string) SettingResult
	// Features returns the computed feature availability (config-design §7).
	Features(ctx context.Context) map[string]bool
	// MissingRequirements explains a false feature verdict with the same registry-owned
	// prerequisite calculation. Values are setting keys ordered for recovery.
	MissingRequirements(ctx context.Context, feature string) []string
	// RegenerateSecret rotates a generated API or playout token and returns the new
	// value (config-design §4).
	RegenerateSecret(ctx context.Context, name string) (value string, err error)
	// RevealSecret returns a generated API or playout token's CURRENT value
	// (config-design §4's eye-toggle). Reading must never rotate.
	RevealSecret(ctx context.Context, name string) (value string, err error)
	// Test runs one named connection check (config-design §8, powers Test buttons).
	Test(ctx context.Context, check string) (ok bool, hint string)
}

type NotificationDestinationService interface {
	Create(context.Context, notifications.Principal, notifications.DestinationCommand) (notifications.DestinationSummary, error)
	List(context.Context, notifications.Principal) ([]notifications.DestinationSummary, error)
	Update(context.Context, notifications.Principal, string, notifications.DestinationUpdateCommand) (notifications.DestinationSummary, error)
	Delete(context.Context, notifications.Principal, string, string) (notifications.DestinationDeleteResult, error)
	Test(context.Context, notifications.Principal, string, string) (notifications.DestinationTestResult, error)
}

// BackendTransitioner is the one deep settings consequence for playout publication. The
// implementation holds its cross-replica lock while invoking mutation and all publication phases.
// mutation returns whether it durably changed a transition input; false skips transition repair.
type BackendTransitioner interface {
	ApplyMutation(ctx context.Context, mutation func(context.Context) bool) error
	// Reconnect force-repairs the durably applied tuner/listing target under the same
	// cross-replica serialization boundary as ordinary backend publication.
	Reconnect(ctx context.Context) (tunersReset int, err error)
}

// SettingEntry is the API view of one setting (config-design §8). For a secret,
// Value is empty and Set/Preview carry the masked state — the value never appears.
// SettingEnumOption is one choice of an enum setting: the stored value + its display
// label (config-design §5). The label is registry-owned so the UI never re-derives it.
type SettingEnumOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// SettingPreset is one common choice for an open-ended setting, offered ahead of a custom value.
// Value is canonical, so it equals a resolved value that matches it.
type SettingPreset struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type SettingEntry struct {
	Key          string              `json:"key"`
	Label        string              `json:"label,omitempty" doc:"Registry-owned human label for workflow forms."`
	Group        string              `json:"group"`
	Owner        string              `json:"owner" doc:"Stable task id for the setting's one curated editor. Browser paths are frontend-owned."`
	Kind         string              `json:"kind"`
	Apply        string              `json:"apply" enum:"live,restart" doc:"When a saved value becomes active: live on the next owning operation, or restart in the next app generation."`
	Presentation string              `json:"presentation,omitempty" doc:"Human editor semantics beyond kind, e.g. bytes, path, or language."`
	Unit         string              `json:"unit,omitempty" doc:"What a plain number counts, shown beside its input, e.g. GiB, MiB or %. Empty for a bare count."`
	Value        string              `json:"value,omitempty" doc:"Resolved value (non-secret). Empty for secrets."`
	Set          bool                `json:"set" doc:"For secrets: whether a value is stored."`
	Preview      string              `json:"preview,omitempty" doc:"For secrets: masked '…a1b2' tail (§4)."`
	BuiltIn      bool                `json:"builtIn,omitempty" doc:"For secrets: none is set, and the key Loomarr ships with is in use instead (the bundled TMDB key). Never previewed."`
	Provenance   string              `json:"provenance" enum:"env,db,default" doc:"env locks the UI field (§3)."`
	Caution      bool                `json:"caution,omitempty" doc:"A stored value self-healed to default (§3)."`
	EnvPinnable  bool                `json:"envPinnable,omitempty" doc:"The environment sets this key, so it can be taken back with the unlock (§3.1). Only meaningful together with provenance/envOverride."`
	EnvOverride  bool                `json:"envOverride,omitempty" doc:"An admin took this key back from the environment (§3.1): the env var is set but deliberately not winning. Reported alongside provenance — such a key resolves honestly as 'db' — so the UI can say 'overriding SEERR_URL' rather than implying the variable is unset."`
	EnvVar       string              `json:"envVar,omitempty" doc:"The environment variable that pins (or would pin) this key — named so the UI can tell the operator exactly what it is overriding, and what to unset to hand it back."`
	Advanced     bool                `json:"advanced"`
	Secret       bool                `json:"secret"`
	Enum         []string            `json:"enum,omitempty" doc:"Enum values (the closed set) — labels are in enumOptions."`
	EnumOptions  []SettingEnumOption `json:"enumOptions,omitempty" doc:"Enum choices with display labels (config-design §5)."`
	Presets      []SettingPreset     `json:"presets,omitempty" doc:"Common choices offered before a custom value, in canonical form. Any value the setting accepts still saves."`
	ShowWhen     map[string][]string `json:"showWhen,omitempty" doc:"Conditional visibility: show only when a named key's current value is listed (§5)."`
	RequiredFor  string              `json:"requiredFor,omitempty"`
	Doc          string              `json:"doc" doc:"One plain sentence, shown under the field."`
	Detail       string              `json:"detail,omitempty" doc:"The rest of the help: trade-offs, units and edge cases, shown behind the field's info icon. Absent when the sentence says it all."`
	UpdatedBy    string              `json:"updatedBy,omitempty" doc:"Display name of the person who last changed it. Empty when that person is unknown; never a raw user id (#2736)."`
	UpdatedByYou bool                `json:"updatedByYou,omitempty" doc:"The signed-in caller made the last change, so the UI can say 'you'."`
	UpdatedAt    string              `json:"updatedAt,omitempty" doc:"RFC3339; empty for env/system writes."`
}

// SettingResult is one key's PATCH outcome (config-design §8).
type SettingResult struct {
	Key     string `json:"key"`
	Status  string `json:"status" enum:"saved,invalid,pinned" doc:"saved | invalid(problem) | pinned (§8)."`
	Problem string `json:"problem,omitempty" doc:"Validation message when status=invalid (never echoes a secret, §4)."`
}

// TaxonomyEditor is the deep graph-edit module used by taxonomy writes. The store still owns the
// atomic graph transaction; this seam adds an authoritative preview and post-commit channel
// convergence without making HTTP handlers understand either implementation.
type TaxonomyEditor interface {
	Preview(ctx context.Context, edit clipstore.TaxonomyEdit) (TaxonomyImpact, error)
	Apply(ctx context.Context, edit clipstore.TaxonomyEdit) (TaxonomyImpact, error)
}

type TaxonomyImpact struct {
	Store    clipstore.TaxonomyEditImpact
	Channels []TaxonomyChannelImpact
}

type TaxonomyChannelImpact struct {
	ID     string
	Name   string
	Number int
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

// SearchService backs GET /v1/search (§7.2) — the SAME catalog impl as the LLM
// grounding tool. Returns candidates as generic maps so the API layer needn't
// import the catalog types (kept dependency-light).
type SearchService interface {
	Search(ctx context.Context, request SearchRequest) ([]SearchCandidate, error)
}

// MovieCollectionService resolves authoritative TMDB movie-collection rosters
// for the bounded Title Keys visible in proposal review. The returned members
// are ordinary grounded candidates so choosing them still uses the existing
// per-title approval gate.
type MovieCollectionService interface {
	ResolveMovieCollections(context.Context, MovieCollectionRequest) (MovieCollectionResolution, error)
}

type MovieCollectionRequest struct {
	Keys []string
}

type MovieCollectionResolution struct {
	Collections []MovieCollection `json:"collections"`
	// Complete is false when a requested Key or referenced roster could not be
	// resolved or validated. That keeps incomplete evidence distinct from an
	// authoritative standalone.
	Complete bool `json:"complete"`
}

type MovieCollection struct {
	TMDBID  int               `json:"tmdbId" minimum:"1"`
	Name    string            `json:"name" minLength:"1"`
	Members []SearchCandidate `json:"members" minItems:"2"`
}

// SearchRequest selects exactly one Catalog operation. Query uses federated title
// search; Discovery uses the provider-neutral structured discovery path. The API
// handler rejects requests that provide both or neither.
type SearchRequest struct {
	Query     string
	MediaType string
	Scope     string
	Limit     int
	Discovery *SearchDiscovery
}

// SearchDiscovery is the dependency-light API representation of Catalog's
// structured discovery request (§7.2). The composition-root adapter is the sole
// conversion seam into catalog.DiscoveryQuery.
type SearchDiscovery struct {
	MediaType        string
	Keywords         []string
	Genres           []string
	YearFrom         int
	YearTo           int
	OriginalLanguage string
	OriginCountry    string
	RuntimeMin       int
	RuntimeMax       int
	VoteAverageMin   float64
	VoteCountMin     int
	Network          string
	Cast             []string
	Creators         []string
}

// CollectionService backs GET /v1/library/collections — the media server's collections
// (Emby/Jellyfin BoxSets) that back `scope.collections` (programming-design §2.2).
//
// ⚠ A LIST, not a search. Collections are a small closed set the operator curates by hand,
// so the picker shows all of them; there is no query parameter because there is nothing to
// narrow. This is why it is not a `scope` on /v1/search — a Candidate models a provisionable
// title and a collection is not one, the same distinction that keeps clips off that route.
type CollectionService interface {
	Collections(ctx context.Context) ([]LibraryCollection, error)
}

// LibraryCollection is the API view of one media-server collection. The ID is opaque and
// server-local — it is what `scope.collections` stores — so the Name is carried alongside it
// purely so the picker can show a human something. ⚠ Never a TMDB collection (franchise) id;
// see programming-design §2.2 on the two namespaces sharing one English word.
type LibraryCollection struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	ChildCount int    `json:"childCount,omitempty"`
}

// SearchCandidate is the API view of a catalog candidate (§7.2). It carries the
// same source-backed editorial and enforcement evidence as the LLM grounding
// tool, so the doc's "humans and the model see identical results" promise (§8)
// holds on the read side too.
type SearchCandidate struct {
	MediaType        string   `json:"mediaType"`
	TMDBID           int      `json:"tmdbId,omitempty"`
	TVDBID           int      `json:"tvdbId,omitempty"`
	Name             string   `json:"name"`
	Year             int      `json:"year,omitempty"`
	InLibrary        bool     `json:"inLibrary"`
	LibraryItemID    string   `json:"libraryItemId,omitempty"`
	Genres           []string `json:"genres,omitempty"`
	Overview         string   `json:"overview,omitempty"`
	OriginalLanguage string   `json:"originalLanguage,omitempty"`
	OriginCountries  []string `json:"originCountries,omitempty"`
	RuntimeMinutes   int      `json:"runtimeMinutes,omitempty"`
	VoteAverage      float64  `json:"voteAverage,omitempty"`
	VoteCount        int      `json:"voteCount,omitempty"`
	Keywords         []string `json:"keywords,omitempty"`
	Networks         []string `json:"networks,omitempty"`
	Cast             []string `json:"cast,omitempty"`
	Creators         []string `json:"creators,omitempty"`
	OfficialRating   string   `json:"officialRating,omitempty"`
}

// holidayCalendar is the engine's holiday calendar, or the default (the US, UTC) without one.
func (s *Server) holidayCalendar() schedule.Calendar {
	if s.channels == nil {
		return schedule.Calendar{}
	}
	return s.channels.Calendar()
}

// LiveTVService backs the Live TV setup status check (§6/§7). Mutating repair belongs to
// BackendTransitioner so it cannot bypass durable publication ordering.
type LiveTVService interface {
	Wired(ctx context.Context) (bool, error)
}

// TunarrConnector wires the media server as *Tunarr's* media source (§6) so Tunarr
// can stream + index the library — backs POST /v1/setup/tunarr-connect and the
// tunarr_library setup check. Idempotent. Implemented by livetvpublish.MediaSourceConnector.
type TunarrConnector interface {
	Connect(ctx context.Context) (sourceID string, librariesEnabled int, err error)
	LibrariesReady(ctx context.Context) (bool, error)
}

// ArrDiscovery reads the Sonarr and Radarr the requester (Seerr) already knows, so the
// wizard can fill in their connections instead of asking for them (#2151).
type ArrDiscovery interface {
	// ArrServers is Seerr's default Sonarr and Radarr; none when Seerr isn't the requester.
	ArrServers(ctx context.Context) ([]DiscoveredArr, error)
	// Reachable probes one arr at url with apiKey.
	Reachable(ctx context.Context, app, url, apiKey string) error
}

// DiscoveredArr is one arr the requester knows: the addresses to try, in order, and its key.
type DiscoveredArr struct {
	App    string
	URLs   []string
	APIKey string
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

// Provisioner owns first-run bootstrap + explicit import (§11). Together with
// PasswordService.CreateLocal these are the ONLY paths that create users — every
// one of them an explicit admin action, never a side effect of someone signing in.
// Implemented by auth.Provisioner.
type Provisioner interface {
	// Bootstrap creates the first local admin, once (ErrBootstrapClosed after).
	Bootstrap(ctx context.Context, username, password string) (identitystore.User, error)
	// Import allowlists the named media-server user ids (admin-only).
	Import(ctx context.Context, ids []string) (int, error)
	// Candidates lists media-server accounts available to import, each flagged with
	// whether it is already allowlisted (§11). Read-only — listing never provisions.
	Candidates(ctx context.Context) ([]auth.Candidate, error)
	// Bootstrapped reports whether an owning admin exists — the one fact the
	// unauthenticated GET /v1/setup/state exposes (§7).
	Bootstrapped(ctx context.Context) (bool, error)
}

// (BackupStreamer lives in internal/api/system.)

// Options configures the API server.
type Options struct {
	Store                records.Store
	Auth                 httpcore.Authorizer
	Log                  *slog.Logger
	Metrics              *metrics.Recorder
	BackupSQLite         system.BackupStreamer // nil ⇒ /v1/backup returns 501 (Postgres)
	Ready                ReadyFunc
	Now                  func() time.Time                   // optional deterministic request clock for tests
	Login                people.LoginService                // /v1/auth/login + user disable (Phase 9); nil ⇒ routes absent
	Passwords            people.PasswordService             // /v1/auth/password + local account create/reset (§11); nil ⇒ routes absent
	Sessions             people.SessionManager              // /v1/auth/logout (Phase 9)
	UserSync             people.UserSyncer                  // POST /v1/users/sync (Phase 9); nil ⇒ route absent
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
	CookieSecure     string                        // COOKIE_SECURE: auto|true|false (§11)
	TrustProxy       bool                          // TRUST_PROXY: honor X-Forwarded-For/-Proto only when true (§11)
	DevLogin         bool                          // LOOMARR_DEV_LOGIN=1 ⇒ mount POST /v1/auth/dev-login (§11); default false ⇒ route absent
	Pprof            bool                          // LOOMARR_PPROF=1 ⇒ mount /debug/pprof/* (§7); default false ⇒ routes absent
	MetricsToken     string                        // LOOMARR_METRICS_TOKEN[_FILE] (§7): the Prometheus scrape bearer; empty ⇒ /metrics refused (fail closed)
	Demo             DemoAccount                   // LOOMARR_DEMO_USERNAME/_PASSWORD (#2268): the zero value ⇒ not a demo site
	Channels         guideapi.ChannelService       // /v1/channels* reconcile (Phase 10); nil ⇒ reconcile route absent
	LiveTV           LiveTVService                 // /v1/setup/* (Phase 10); nil ⇒ setup routes absent
	TunerRescanner   channelsapi.TunerRescanner    // §9 channel-list freshness; nil ⇒ best-effort poke unavailable
	TunarrConnect    TunarrConnector               // /v1/setup/tunarr-connect + tunarr_library check (§6); nil ⇒ 501
	ArrDiscovery     ArrDiscovery                  // /v1/setup/arrs-from-requester; nil ⇒ fills nothing
	Suggest          SuggestService                // /v1/proposals submit (Phase 11); nil ⇒ submit route 501
	ProposalProgress proposalsapi.ProposalProgress // live generation progress on GET /v1/proposal-jobs/{jobId}; nil ⇒ none
	ProposalWorkflow proposalsapi.ProposalWorkflow // authoritative /v1/proposal-jobs Journey reads
	Search           SearchService                 // /v1/search (Phase 11); nil ⇒ search route 501
	MovieCollections MovieCollectionService        // /v1/movie-collections; nil ⇒ route 501
	Collections      CollectionService             // /v1/library/collections (§2.2); nil ⇒ route 501
	LibraryRoots     LibraryRootService            // /v1/library/roots*, path-map, status (#2151); nil ⇒ 409
	LibraryHooks     LibraryHookService            // /v1/library/hooks/* (#2151); nil ⇒ 409
	IdeaLibrary      channelsapi.IdeaLibrary       ///v1/discovery/ideas (#1665); nil ⇒ route 501
	Icons            channelsapi.IconService       ///v1/channels/{id}/icon-suggestions (§icon P2); nil ⇒ 501
	// Images backs /v1/images* — the one pipeline every image travels (§22, V52). nil ⇒ the byte
	// route 404s and the record route reports the image absent, which is the honest answer for an
	// instance with no store behind it.
	Images          ImageService
	Events          EventSource                // /v1/events SSE (Phase 11); nil ⇒ route 501
	Shutdown        <-chan struct{}            // generation lifetime; closes SSE so http.Server.Shutdown can drain
	Filler          fillerapi.Service          // /v1/filler sync/tag (Phase 12); nil ⇒ those routes 501
	SourceProviders *source.Registry           // /v1/filler/providers and source validation; nil ⇒ none registered
	FillerResearch  fillerapi.ResearchService  // optional structured-first web-search status and validation
	FillerDefects   *defectfloor.DefectReports // /v1/filler/defects* (#2034); nil ⇒ 501
	Pods            guideapi.PodPreviewer      // /v1/channels/{id}/pods preview (§12); nil ⇒ 501
	Taxonomy        TaxonomyEditor             // taxonomy impact + committed channel convergence
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
	DiagnosticEvents DiagnosticEventService
	// DiagnosticProcesses backs active/recent Process runs and bounded output.
	DiagnosticProcesses DiagnosticProcessService
	// DiagnosticBundles backs preview and on-demand redacted ZIP creation.
	DiagnosticBundles DiagnosticBundleService
	// DiagnosticCapture controls the recorder's process-local bounded debug window.
	DiagnosticCapture DiagnosticCaptureService
	// ClientDiagnostics accepts bounded curated events from authenticated web/TV clients.
	ClientDiagnostics ClientDiagnosticService
	// ClientCrashes backs the crash-report write and its admin reads (#1816); nil ⇒ unmounted.
	ClientCrashes ClientCrashService
	// StartupReports backs the current/recent application-generation report.
	StartupReports StartupReportService
	// Restart backs POST /v1/system/restart (§9.2, V13) — implemented over main's
	// generation loop. nil ⇒ 501, the honest answer for a handler with no loop behind it.
	Restart system.RestartService
	// RestartDrift names bootstrap and app-managed generation settings waiting on a restart
	// (config-design §3).
	RestartDrift             func() []string
	Jobs                     system.JobService                                         // /v1/jobs* background-job scheduler (§18.1); nil ⇒ routes 501
	Settings                 SettingsService                                           // /v1/settings* (config-design §8); nil ⇒ routes 501
	NotificationDestinations NotificationDestinationService                            // redacted product-delivery routing (§11)
	WebPushPublicKey         string                                                    // public half of the generated VAPID identity (§11)
	ProposalNotifications    suggest.ProposalNotifier                                  // best-effort product lifecycle publication (§11)
	DecisionQuality          proposalsapi.ProposalDecisionQuality                      // committed Proposal denial quality outcome (§17)
	BackendTransition        BackendTransitioner                                       // durable backend prepare/publish/retire coordinator
	BackendCheckpoint        func(context.Context) (httpcore.BackendCheckpoint, error) // durable checkpoint, once per operation
	Guide                    guideapi.GuideReader                                      // /v1/channels/now-next (§6, §9); nil ⇒ empty now/next
	Provision                Provisioner                                               // /v1/setup/bootstrap + /v1/users/import (§11); nil ⇒ routes absent
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
	// PlayoutFont is the font the offline card is labelled with — a property of the HOST
	// (filesystem + ffmpeg build), so it is injected like PlayoutSecret rather than resolved
	// in the handler. Nil or "" ⇒ an unlabelled card, which is a supported rendering and the
	// deliberate fail-safe: see playout.CardFontFor for why a font file alone is not enough.
	PlayoutFont func() string
	// PlayoutTonemap reports whether this ffmpeg build carries the HDR→SDR filters (zscale +
	// tonemap). A property of the BUILD, so it is injected here rather than probed in the handler,
	// which keeps ProgramArgs a pure function. Nil ⇒ no CPU tone-mapping. See playout.TonemapperFor.
	PlayoutTonemap func() bool
	// PlayoutGPUTonemap reports which GPU tone-mappers the build carries (playout.GPUFiltersFor).
	// Nil ⇒ none: HDR then tone-maps on the CPU after the GPU downscale.
	PlayoutGPUTonemap func() encode.GPUFilters
	// ReclaimVRAM frees GPU memory the hardware encoders need — evicts the resident local LLM
	// (§8.2 Evictor, §9.1 V47). Wired to the LLM provider's Evict when the provider is local and
	// implements Evictor; nil for a hosted provider (nothing local to reclaim). The retry ladder
	// calls it only after a hardware encode has already produced nothing.
	ReclaimVRAM func(ctx context.Context)
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
	// path and mount our own at /v1/reference (ops.go).
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
