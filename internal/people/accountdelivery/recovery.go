package accountdelivery

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"

	"github.com/loomarr/loomarr/internal/people/access"
	"github.com/loomarr/loomarr/internal/people/auth"
	"github.com/loomarr/loomarr/internal/people/notifications"
)

// PasswordRecovery runs the public password-recovery flow: it rate-limits each step, queues the
// recovery email, and keeps every response uniform so it never reveals which accounts exist.
type PasswordRecovery struct {
	recovery       *auth.PasswordRecoveryService
	notifications  passwordRecoveryNotifier
	requestLimiter auth.Limiter
	redeemLimiter  *auth.RateLimiter
	log            *slog.Logger
}

// newRecoveryRequestLimiter throttles recovery requests per account and per client address: each
// accepted request can email an account holder, so one address must not reach every account by
// rotating usernames.
func newRecoveryRequestLimiter() *auth.SourceLimiter { return auth.NewSourceLimiter(0.05, 3, 0.05, 6) }

type passwordRecoveryNotifier interface {
	Publish(context.Context, notifications.PublishCommand) (notifications.Intent, bool, error)
}

func (c *PasswordRecovery) Request(ctx context.Context, username, rateKey string) error {
	if c == nil || c.recovery == nil || c.notifications == nil {
		return fmt.Errorf("password recovery is unavailable")
	}
	if c.requestLimiter != nil && !c.requestLimiter.Allow(rateKey) {
		return auth.ErrRateLimited
	}
	request, err := c.recovery.Request(ctx, username)
	if err != nil || request == nil {
		return err
	}
	key := fmt.Sprintf("password-recovery-email:%x", sha256.Sum256([]byte(request.Recovery.ID)))
	_, _, err = c.notifications.Publish(ctx, notifications.PublishCommand{
		Topic:         notifications.TopicLocalPasswordRecovery,
		RecipientKind: notifications.RecipientPerson, RecipientID: request.Recovery.UserID,
		ReferenceKind: notifications.ReferenceRecovery, ReferenceID: request.Recovery.ID,
		Policy:         notifications.PolicyMandatoryAccount,
		Template:       notifications.TemplateData{RecipientName: request.RecipientName},
		IdempotencyKey: key,
	})
	// A queue failure occurs only after eligibility has been established. Returning it would make
	// the public status distinguish a real local account from every no-op case. Keep the response
	// uniform and retain the operator-facing failure only in server logs, without username/email.
	if err != nil && c.log != nil {
		c.log.Warn("password recovery notification could not be queued", "err", err)
	}
	return nil
}

func (c *PasswordRecovery) Preview(
	ctx context.Context,
	grant string,
	rateKey string,
) (access.RecoveryRecord, error) {
	if c == nil || c.recovery == nil {
		return access.RecoveryRecord{}, auth.ErrInvalidPasswordRecovery
	}
	if c.redeemLimiter != nil && !c.redeemLimiter.Allow("preview|"+rateKey) {
		return access.RecoveryRecord{}, auth.ErrRateLimited
	}
	return c.recovery.Preview(ctx, grant)
}

func (c *PasswordRecovery) Redeem(
	ctx context.Context,
	grant string,
	password string,
	rateKey string,
) error {
	if c == nil || c.recovery == nil {
		return auth.ErrInvalidPasswordRecovery
	}
	if c.redeemLimiter != nil && !c.redeemLimiter.Allow("redeem|"+rateKey) {
		return auth.ErrRateLimited
	}
	return c.recovery.Redeem(ctx, grant, password)
}
