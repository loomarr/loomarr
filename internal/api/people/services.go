package people

import (
	"context"
	"time"

	"github.com/loomarr/loomarr/internal/people/access"
	"github.com/loomarr/loomarr/internal/people/contact"
	"github.com/loomarr/loomarr/internal/people/notifications"
	"github.com/loomarr/loomarr/internal/platform/store/identitystore"
)

// UserSyncer refreshes already-imported users from the media server (§11).
// Returns the count synced. It never ADDS users (import defines the allowlist).
type UserSyncer interface {
	Sync(ctx context.Context) (int, error)
}

// LoginService verifies credentials and issues a session (§11).
type LoginService interface {
	Login(ctx context.Context, username, password, rateKey string) (token string, expires time.Time, u identitystore.User, err error)
	// DevLogin issues a session for an existing admin with no credential (§11).
	// Only ever called when the server was started with LOOMARR_DEV_LOGIN=1 — the
	// route is not registered otherwise.
	DevLogin(ctx context.Context) (token string, expires time.Time, u identitystore.User, err error)
	Disable(ctx context.Context, userID string) error
}

// PasswordService owns LOCAL credential mutations (§11). Kept separate from
// LoginService because verifying a credential and changing one are different
// authority: the first is what any signed-in user does constantly, the second is
// what an account owner (or an admin) does deliberately.
// Implemented by auth.PasswordService.
type PasswordService interface {
	// ChangePassword is the SELF path: proves the current password, then replaces it
	// and revokes every session for that user.
	ChangePassword(ctx context.Context, userID, current, next string) error
	// CreateLocal mints a local account — an allowlist row carrying an Argon2id hash.
	// Admin-only at the route layer; the same kind of explicit write as Import.
	CreateLocal(ctx context.Context, username, password string, role identitystore.Role, quota int) (identitystore.User, error)
	// ResetPassword is the ADMIN path: no current password (that's the point), so it
	// relies entirely on the caller's role. Deliberately not the same call as
	// ChangePassword — sharing one would make the "prove you know it" check optional.
	ResetPassword(ctx context.Context, targetUserID, next string) error
}

// InvitationService is the narrow administrator-facing seam of the Invitation module.
// Durable grant hashes and provider lookup mechanics remain behind it.
type InvitationService interface {
	Create(context.Context, access.CreateCommand) (access.Invitation, error)
	Get(context.Context, string) (access.Invitation, error)
	List(context.Context) ([]access.Invitation, error)
	Contact(context.Context, string) (contact.Address, error)
	Regenerate(context.Context, string, access.Conveyance) (access.IssuedGrant, error)
	Revoke(context.Context, string) error
}

// InvitationDeliveryService composes provider-neutral delivery work beside the Invitation
// lifecycle. Sending mail never creates or redeems an allowlist row.
type InvitationDeliveryService interface {
	SendEmail(context.Context, string, string) (notifications.DeliverySummary, error)
	LatestEmail(context.Context, string) (notifications.DeliverySummary, error)
}

// InvitationRedemptionService is the public credential-proof seam. Bearers are
// accepted only in request bodies and never enter a route, query, or durable DTO.
type InvitationRedemptionService interface {
	Preview(context.Context, string) (access.Invitation, error)
	RedeemLocal(context.Context, string, string) (string, time.Time, identitystore.User, error)
	RedeemLibrary(context.Context, string, string, string) (string, time.Time, identitystore.User, error)
}

// PasswordRecoveryService is the public, enumeration-safe local credential recovery seam.
// Bearers remain body-only and successful reset deliberately issues no replacement session.
type PasswordRecoveryService interface {
	Request(context.Context, string, string) error
	Preview(context.Context, string, string) (access.RecoveryRecord, error)
	Redeem(context.Context, string, string, string) error
}

// SessionManager revokes sessions (logout) and exposes them for admin review (§11).
type SessionManager interface {
	Revoke(ctx context.Context, token string) error
	List(ctx context.Context, userID string) ([]identitystore.Session, error)
	RevokeHash(ctx context.Context, tokenHash string) error
}
