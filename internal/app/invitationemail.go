package app

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/loomarr/loomarr/internal/invitation"
	"github.com/loomarr/loomarr/internal/notifications"
	"github.com/loomarr/loomarr/internal/scheduler"
	"github.com/loomarr/loomarr/internal/store"
)

const notificationWorkerLimit = 100

type invitationDeliveryCoordinator struct {
	invitations   *invitation.Service
	notifications *notifications.Service
}

func (c *invitationDeliveryCoordinator) SendEmail(
	ctx context.Context,
	invitationID string,
	requestID string,
) (notifications.DeliverySummary, error) {
	if c == nil || c.invitations == nil || c.notifications == nil {
		return notifications.DeliverySummary{}, notifications.ErrNotFound
	}
	value, err := c.invitations.Get(ctx, invitationID)
	if err != nil {
		return notifications.DeliverySummary{}, err
	}
	if value.Status != invitation.StatusPending {
		return notifications.DeliverySummary{}, fmt.Errorf("invitation is not pending")
	}
	if strings.TrimSpace(requestID) == "" || len(requestID) > 100 || strings.ContainsAny(requestID, "\r\n") {
		return notifications.DeliverySummary{}, fmt.Errorf("invitation email requires a safe request id")
	}
	name := value.DisplayName
	if name == "" {
		name = value.Username
	}
	key := fmt.Sprintf("invitation-email:%x", sha256.Sum256([]byte(invitationID+"\x00"+requestID)))
	_, _, err = c.notifications.Publish(ctx, notifications.PublishCommand{
		Topic: notifications.TopicAccountInvitation, RecipientKind: notifications.RecipientInvitation,
		RecipientID: invitationID, ReferenceKind: notifications.ReferenceInvitation,
		ReferenceID: invitationID, Policy: notifications.PolicyMandatoryAccount,
		Template: notifications.TemplateData{RecipientName: name}, IdempotencyKey: key,
	})
	if err != nil {
		return notifications.DeliverySummary{}, err
	}
	return c.LatestEmail(ctx, invitationID)
}

func (c *invitationDeliveryCoordinator) LatestEmail(
	ctx context.Context,
	invitationID string,
) (notifications.DeliverySummary, error) {
	return c.notifications.LatestDelivery(ctx, notifications.ReferenceInvitation, invitationID)
}

type invitationEmailRouter struct {
	invitations *invitation.Service
	config      func() notifications.EmailConfig
}

func (r invitationEmailRouter) Routes(ctx context.Context, intent notifications.Intent) ([]notifications.Route, error) {
	if intent.Topic != notifications.TopicAccountInvitation || intent.RecipientKind != notifications.RecipientInvitation {
		return nil, fmt.Errorf("invitation email router received unsupported intent")
	}
	route := notifications.Route{
		Means: notifications.MeansEmail, DestinationRef: "invitation:" + intent.RecipientID,
		DestinationRedacted: "unavailable",
	}
	address, err := r.invitations.Contact(ctx, intent.RecipientID)
	if errors.Is(err, store.ErrNotFound) {
		route.Suppressed = notifications.OutcomeDestinationUnavailable
		return []notifications.Route{route}, nil
	}
	if err != nil {
		return nil, err
	}
	route.DestinationRedacted = redactMailbox(address.Email)
	if r.config == nil || !r.config().Enabled {
		route.Suppressed = notifications.OutcomeDeliveryDisabled
	}
	return []notifications.Route{route}, nil
}

type invitationEmailMaterializer struct {
	invitations *invitation.Service
	publicURL   func() string
}

func (m invitationEmailMaterializer) Materialize(
	ctx context.Context,
	delivery notifications.Delivery,
) (notifications.MaterializedEmail, error) {
	if delivery.Intent.Topic != notifications.TopicAccountInvitation ||
		delivery.Intent.ReferenceKind != notifications.ReferenceInvitation ||
		delivery.Attempt.DestinationRef != "invitation:"+delivery.Intent.ReferenceID {
		return notifications.MaterializedEmail{}, fmt.Errorf("invalid invitation email delivery references")
	}
	address, err := m.invitations.Contact(ctx, delivery.Intent.ReferenceID)
	if errors.Is(err, store.ErrNotFound) {
		return notifications.MaterializedEmail{}, notifications.ErrEmailDestinationUnavailable
	}
	if err != nil {
		return notifications.MaterializedEmail{}, err
	}
	origin, err := recipientOriginValue(m.publicURL)
	if err != nil {
		return notifications.MaterializedEmail{}, err
	}
	issued, err := m.invitations.IssueSibling(ctx, delivery.Intent.ReferenceID, invitation.ConveyanceEmail)
	if err != nil {
		return notifications.MaterializedEmail{}, err
	}
	invalidate := func(revokeCtx context.Context) error {
		return m.invitations.RevokeIssuedGrant(revokeCtx, issued.Plaintext)
	}
	actionURL := origin + "/join#grant=" + url.QueryEscape(issued.Plaintext)
	content, err := notifications.RenderAccountEmail(delivery.Intent, actionURL, issued.ExpiresAt)
	if err != nil {
		_ = invalidate(ctx)
		return notifications.MaterializedEmail{}, err
	}
	return notifications.MaterializedEmail{
		Message: notifications.EmailMessage{
			ToAddress: address.Email, ToName: delivery.Intent.Template.RecipientName,
			Subject: content.Subject, TextBody: content.TextBody, HTMLBody: content.HTMLBody,
		},
		Invalidate: invalidate,
	}, nil
}

func notificationDeliveryJob(service *notifications.Service) scheduler.Job {
	return scheduler.Job{
		Name: "notification-delivery", Group: scheduler.GroupSystem, Title: "Deliver account messages",
		Description: "Sends queued invitation and recovery messages through their configured delivery means.",
		DefaultCron: "*/15 * * * * *", ScheduleKey: "job.notification_delivery.schedule",
		Run: func(ctx context.Context) error {
			for range notificationWorkerLimit {
				ran, err := service.RunOne(ctx, "notification-delivery")
				if err != nil {
					return err
				}
				if !ran {
					return nil
				}
			}
			return nil
		},
	}
}

func recipientOriginValue(read func() string) (string, error) {
	if read == nil {
		return "", fmt.Errorf("access public URL is unavailable")
	}
	parsed, err := url.Parse(strings.TrimSpace(read()))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" ||
		(parsed.Path != "" && parsed.Path != "/") {
		return "", fmt.Errorf("access public URL is not an absolute HTTP origin")
	}
	return parsed.Scheme + "://" + parsed.Host, nil
}

func redactMailbox(address string) string {
	local, domain, ok := strings.Cut(address, "@")
	if !ok || local == "" || domain == "" {
		return "unavailable"
	}
	first, _ := utf8.DecodeRuneInString(local)
	if first == utf8.RuneError {
		return "unavailable"
	}
	return string(first) + "***@" + domain
}

func buildInvitationDelivery(
	st store.Store,
	set resolved,
	invitationService *invitation.Service,
	registry *scheduler.Registry,
) *invitationDeliveryCoordinator {
	if st == nil || set.svc == nil || invitationService == nil {
		return nil
	}
	materializer := invitationEmailMaterializer{
		invitations: invitationService, publicURL: func() string { return set.str("access.public_url") },
	}
	adapter := notifications.NewEmailAdapter(set.emailConfig, materializer, notifications.NewSMTPSender(15*time.Second))
	service := notifications.NewService(st, invitationEmailRouter{
		invitations: invitationService, config: set.emailConfig,
	}, []notifications.Adapter{adapter}, newID, time.Now)
	if registry != nil {
		registry.Add(notificationDeliveryJob(service))
	}
	return &invitationDeliveryCoordinator{invitations: invitationService, notifications: service}
}
