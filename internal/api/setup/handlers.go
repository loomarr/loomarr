// Package setup serves the settings and the setup wizard: the settings list and edits, secrets,
// connection tests, the setup status checks and their repairs (Tunarr, the Live TV tuner, the
// *arrs the requester already knows), and the notification destinations.
package setup

import (
	"context"
	"log/slog"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/api/httpcore"
	"github.com/loomarr/loomarr/internal/platform/settings"
)

// Deps is what the settings and setup routes need from the rest of the server. Every field is
// optional: a nil service leaves its routes answering 501 or unmounted, as the API's nil-semantics
// describe.
type Deps struct {
	Log *slog.Logger
	// Store names the people behind a setting's last change.
	Store httpcore.UserLister
	// LiveSettings reads live values for gating optional checks.
	LiveSettings settings.Reader
	// The names below are copied from api.Options (Settings) and api.SetupOptions.
	Settings                 SettingsService
	NotificationDestinations NotificationDestinationService
	WebPushPublicKey         string
	BackendTransition        BackendTransitioner
	LiveTV                   LiveTVService
	TunarrConnect            TunarrConnector
	ArrDiscovery             ArrDiscovery
	// LibraryConfigured resolves the complete live media-server connection from one settings
	// snapshot; nil reads as configured, the unit-test convention.
	LibraryConfigured func() bool
}

// Handlers serves the settings and setup routes.
type Handlers struct {
	log                      *slog.Logger
	store                    httpcore.UserLister
	liveSettings             settings.Reader
	settings                 SettingsService
	notificationDestinations NotificationDestinationService
	webPushPublicKey         string
	backendTransition        BackendTransitioner
	livetv                   LiveTVService
	tunarrConnect            TunarrConnector
	arrDiscovery             ArrDiscovery
	libraryConfigured        func() bool
}

// New returns the settings and setup handlers over d.
func New(d Deps) *Handlers {
	return &Handlers{
		log: d.Log, store: d.Store, liveSettings: d.LiveSettings, settings: d.Settings,
		notificationDestinations: d.NotificationDestinations, webPushPublicKey: d.WebPushPublicKey,
		backendTransition: d.BackendTransition, livetv: d.LiveTV, tunarrConnect: d.TunarrConnect,
		arrDiscovery: d.ArrDiscovery, libraryConfigured: d.LibraryConfigured,
	}
}

// Register mounts every settings and setup route on api.
func (s *Handlers) Register(api huma.API) {
	s.registerSettings(api)
	s.registerNotificationDestinations(api)
	s.registerSetup(api)
}

// ConfigInt reads a live integer setting; the server hands it to the routes that need a limit.
func (s *Handlers) ConfigInt(key settings.IntKey) int { return s.configInt(key) }

// ConnectionChecks runs the connection probes the wizard checklist, the services panel and the
// reload all share, so the three can never disagree.
func (s *Handlers) ConnectionChecks(ctx context.Context) []httpcore.SetupCheck {
	return s.runConnectionChecks(ctx)
}

// LibraryUnconfigured gates every optional media-library operation on the same complete, atomic
// {flavor,url,token} predicate. Nil preserves the unit-test convention that an explicitly wired
// adapter is usable without a settings runtime.
func (s *Handlers) LibraryUnconfigured() bool {
	return s.libraryConfigured != nil && !s.libraryConfigured()
}

// unconfigured reports whether a required string setting is empty right now. Safe when
// liveSettings is unset (unit tests): reports false so a wired dependency still serves.
func (s *Handlers) unconfigured(keys ...settings.StringKey) bool {
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

// libraryUnconfigured is LibraryUnconfigured for the routes in this package.
func (s *Handlers) libraryUnconfigured() bool { return s.LibraryUnconfigured() }
