package people

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/api/httpcore"
	"github.com/loomarr/loomarr/internal/people/auth"
	"github.com/loomarr/loomarr/internal/people/contact"
	"github.com/loomarr/loomarr/internal/platform/metrics"
	"github.com/loomarr/loomarr/internal/platform/settings"
	"github.com/loomarr/loomarr/internal/platform/store/identitystore"
	"github.com/loomarr/loomarr/internal/proposals/accept"
)

// Store is the slice of the identity store the people routes read and write: the user directory and
// the contact addresses beside it. The identity store satisfies it.
type Store interface {
	// QuotaStore is what a user's pending-acquisition usage is computed from.
	accept.QuotaStore
	GetUser(ctx context.Context, id string) (identitystore.User, error)
	UpsertUser(ctx context.Context, u identitystore.User) error
	ListUsers(ctx context.Context) ([]identitystore.User, error)
	UserLastSeen(ctx context.Context) (map[string]time.Time, error)
	ListContactAddresses(ctx context.Context) ([]contact.Address, error)
	PutPendingContactAddress(ctx context.Context, address contact.Address) error
	DeletePendingContactAddress(ctx context.Context, userID string) error
	DeleteContactAddresses(ctx context.Context, userID string) error
}

// Deps is what the people routes need from the rest of the server. Every field is optional: a nil
// service leaves its routes unmounted (or answering 501), as the API's nil-semantics describe.
type Deps struct {
	Store     Store
	Log       *slog.Logger
	Metrics   *metrics.Recorder
	Origin    httpcore.Origin
	Login     LoginService
	Sessions  SessionManager
	Passwords PasswordService
	UserSync  UserSyncer

	Invitations          InvitationService
	InvitationDelivery   InvitationDeliveryService
	InvitationRedemption InvitationRedemptionService
	PasswordRecovery     PasswordRecoveryService
	// AccessPublicURL supplies the recipient-reachable browser origin.
	AccessPublicURL func() string

	Devices       *auth.DeviceManager
	DeviceLimiter *auth.RateLimiter
	// DevLogin mounts the credential-free dev sign-in (LOOMARR_DEV_LOGIN=1).
	DevLogin bool
	SSO      SSOService
	// Provision owns first-run bootstrap and the media-server user import; nil leaves those routes
	// unmounted. Demo is the demo site's public sign-in, reported by the setup state.
	Provision Provisioner
	Demo      DemoAccount

	// ConfigInt reads an integer setting; LibraryUnconfigured reports that no library source is
	// connected yet.
	ConfigInt           func(settings.IntKey) int
	LibraryUnconfigured func() bool
}

// Handlers serves the people routes.
type Handlers struct {
	store                Store
	log                  *slog.Logger
	metrics              *metrics.Recorder
	origin               httpcore.Origin
	login                LoginService
	sessions             SessionManager
	passwords            PasswordService
	userSync             UserSyncer
	invitations          InvitationService
	invitationDelivery   InvitationDeliveryService
	invitationRedemption InvitationRedemptionService
	passwordRecovery     PasswordRecoveryService
	accessPublicURL      func() string
	devices              *auth.DeviceManager
	deviceLimiter        *auth.RateLimiter
	// invitationPreviewLimiter and invitationRedeemLimiter throttle the two public invitation
	// routes. Redeeming a library invitation asks the media server to check a caller-chosen
	// username and password, so it is held per account across every source as well as per source.
	invitationPreviewLimiter *auth.RateLimiter
	invitationRedeemLimiter  *auth.SourceLimiter
	devLogin                 bool
	sso                      SSOService
	provision                Provisioner
	demo                     DemoAccount

	configIntFn           func(settings.IntKey) int
	libraryUnconfiguredFn func() bool

	// schemaOnly is set only by the OpenAPI exporter: it mounts every route whatever is configured,
	// so the generated spec lists them all.
	schemaOnly bool
}

// New returns the people handlers over d. schemaOnly mounts every route regardless of which
// services d carries; only the OpenAPI exporter sets it.
func New(d Deps, schemaOnly bool) *Handlers {
	return &Handlers{
		store: d.Store, log: d.Log, metrics: d.Metrics, origin: d.Origin,
		login: d.Login, sessions: d.Sessions, passwords: d.Passwords, userSync: d.UserSync,
		invitations: d.Invitations, invitationDelivery: d.InvitationDelivery,
		invitationRedemption: d.InvitationRedemption, passwordRecovery: d.PasswordRecovery,
		accessPublicURL: d.AccessPublicURL, devices: d.Devices, deviceLimiter: d.DeviceLimiter,
		invitationPreviewLimiter: newInvitationPreviewLimiter(), invitationRedeemLimiter: newInvitationRedeemLimiter(),
		devLogin: d.DevLogin, sso: d.SSO, provision: d.Provision, demo: d.Demo,
		configIntFn: d.ConfigInt, libraryUnconfiguredFn: d.LibraryUnconfigured,
		schemaOnly: schemaOnly,
	}
}

// Register mounts every people route on api.
func (s *Handlers) Register(api huma.API) {
	s.registerAuth(api)
	s.registerDeviceAuth(api)
	s.registerUsers(api)
	s.registerInvitations(api)
	s.registerInvitationRedemption(api)
	s.registerPasswordRecovery(api)
	s.registerPasswords(api)
	s.registerSSO(api)
	s.registerProvisioning(api)
}

func (s *Handlers) configInt(key settings.IntKey) int {
	if s.configIntFn == nil {
		return 0
	}
	return s.configIntFn(key)
}

func (s *Handlers) libraryUnconfigured() bool {
	return s.libraryUnconfiguredFn != nil && s.libraryUnconfiguredFn()
}

func (s *Handlers) writeProblem(w http.ResponseWriter, r *http.Request, status int, title, detail string) {
	httpcore.WriteProblem(s.log, w, r, status, title, detail)
}
