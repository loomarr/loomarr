package app

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"

	"github.com/loomarr/loomarr/internal/auth"
	"github.com/loomarr/loomarr/internal/notifications"
	"github.com/loomarr/loomarr/internal/recovery"
)

type passwordRecoveryCoordinator struct {
	recovery       *auth.PasswordRecoveryService
	notifications  passwordRecoveryNotifier
	requestLimiter *auth.RateLimiter
	redeemLimiter  *auth.RateLimiter
	log            *slog.Logger
}

type passwordRecoveryNotifier interface {
	Publish(context.Context, notifications.PublishCommand) (notifications.Intent, bool, error)
}

func (c *passwordRecoveryCoordinator) Request(ctx context.Context, username, rateKey string) error {
	if c == nil || c.recovery == nil || c.notifications == nil {
		return fmt.Errorf("password recovery is unavailable")
	}
	if c.requestLimiter != nil && !c.requestLimiter.Allow("request|"+rateKey) {
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

func (c *passwordRecoveryCoordinator) Preview(
	ctx context.Context,
	grant string,
	rateKey string,
) (recovery.Record, error) {
	if c == nil || c.recovery == nil {
		return recovery.Record{}, auth.ErrInvalidPasswordRecovery
	}
	if c.redeemLimiter != nil && !c.redeemLimiter.Allow("preview|"+rateKey) {
		return recovery.Record{}, auth.ErrRateLimited
	}
	return c.recovery.Preview(ctx, grant)
}

func (c *passwordRecoveryCoordinator) Redeem(
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
