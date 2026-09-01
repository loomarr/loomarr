package app

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/loomarr/loomarr/internal/auth"
	"github.com/loomarr/loomarr/internal/contact"
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
	invitations  *invitation.Service
	recovery     *auth.PasswordRecoveryService
	destinations notifications.DestinationSource
	config       func() notifications.EmailConfig
}

type combinedNotificationRouter struct {
	account notifications.Router
	product notifications.Router
}

func (r combinedNotificationRouter) Routes(ctx context.Context, intent notifications.Intent) ([]notifications.Route, error) {
	if intent.Policy == notifications.PolicyMandatoryAccount {
		return r.account.Routes(ctx, intent)
	}
	return r.product.Routes(ctx, intent)
}

type currentNotificationEligibility struct{ users store.UserStore }

func (e currentNotificationEligibility) Eligible(
	ctx context.Context,
	audience notifications.RecipientKind,
	personID string,
) (bool, error) {
	if e.users == nil || (audience != notifications.RecipientApprovers && audience != notifications.RecipientOperators) {
		return false, nil
	}
	user, err := e.users.GetUser(ctx, personID)
	if errors.Is(err, store.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return !user.Disabled && user.Role == store.RoleAdmin, nil
}

func (r invitationEmailRouter) Routes(ctx context.Context, intent notifications.Intent) ([]notifications.Route, error) {
	route := notifications.Route{
		Means:               notifications.MeansEmail,
		DestinationRedacted: "unavailable",
	}
	var address contact.Address
	var err error
	switch {
	case intent.Topic == notifications.TopicAccountInvitation &&
		intent.RecipientKind == notifications.RecipientInvitation && r.invitations != nil:
		route.DestinationRef = "invitation:" + intent.RecipientID
		address, err = r.invitations.Contact(ctx, intent.RecipientID)
	case intent.Topic == notifications.TopicLocalPasswordRecovery &&
		intent.RecipientKind == notifications.RecipientPerson && r.recovery != nil:
		route.DestinationRef = "person:" + intent.RecipientID
		address, err = r.recovery.Contact(ctx, intent.RecipientID)
	default:
		return nil, fmt.Errorf("account email router received unsupported intent")
	}
	if errors.Is(err, store.ErrNotFound) {
		route.Suppressed = notifications.OutcomeDestinationUnavailable
		return []notifications.Route{route}, nil
	}
	if err != nil {
		return nil, err
	}
	route.DestinationRedacted = redactMailbox(address.Email)
	if r.destinations == nil {
		if r.config == nil || !r.config().Enabled {
			route.Suppressed = notifications.OutcomeDeliveryDisabled
		}
		return []notifications.Route{route}, nil
	}
	destinations, listErr := r.destinations.ListNotificationDestinations(ctx)
	if listErr != nil {
		return nil, listErr
	}
	sort.Slice(destinations, func(i, j int) bool {
		if destinations[i].CreatedAt.Equal(destinations[j].CreatedAt) {
			return destinations[i].ID < destinations[j].ID
		}
		return destinations[i].CreatedAt.Before(destinations[j].CreatedAt)
	})
	for _, destination := range destinations {
		if destination.Means == notifications.MeansEmail && destination.Enabled {
			route.DestinationRef = "provider:" + destination.ID
			return []notifications.Route{route}, nil
		}
	}
	if len(destinations) == 0 || route.Suppressed == notifications.OutcomeNone {
		route.Suppressed = notifications.OutcomeDeliveryDisabled
	}
	return []notifications.Route{route}, nil
}

type invitationEmailMaterializer struct {
	invitations *invitation.Service
	recovery    *auth.PasswordRecoveryService
	contacts    contactAddressReader
	publicURL   func() string
}

type contactAddressReader interface {
	GetContactAddresses(context.Context, string) (contact.Set, error)
}

func (m invitationEmailMaterializer) Materialize(
	ctx context.Context,
	delivery notifications.Delivery,
) (notifications.MaterializedEmail, error) {
	if delivery.Intent.Policy == notifications.PolicyConfigurable &&
		delivery.Intent.RecipientKind == notifications.RecipientPerson && m.contacts != nil {
		addresses, err := m.contacts.GetContactAddresses(ctx, delivery.Intent.RecipientID)
		if err != nil || addresses.Verified == nil {
			if errors.Is(err, store.ErrNotFound) || addresses.Verified == nil {
				return notifications.MaterializedEmail{}, notifications.ErrEmailDestinationUnavailable
			}
			return notifications.MaterializedEmail{}, err
		}
		link := ""
		if origin, originErr := recipientOriginValue(m.publicURL); originErr == nil {
			link = productNotificationLink(origin, delivery.Intent)
		}
		content, err := notifications.RenderProductEmail(delivery.Intent, link)
		if err != nil {
			return notifications.MaterializedEmail{}, err
		}
		return notifications.MaterializedEmail{Message: notifications.EmailMessage{
			ToAddress: addresses.Verified.Email, Subject: content.Subject,
			TextBody: content.TextBody, HTMLBody: content.HTMLBody,
		}}, nil
	}
	var address contact.Address
	var issuedPlaintext string
	var expiresAt time.Time
	var actionPath string
	var revoke func(context.Context) error
	var err error
	switch {
	case delivery.Intent.Topic == notifications.TopicAccountInvitation &&
		delivery.Intent.ReferenceKind == notifications.ReferenceInvitation &&
		m.invitations != nil:
		address, err = m.invitations.Contact(ctx, delivery.Intent.ReferenceID)
		if err == nil {
			issued, issueErr := m.invitations.IssueSibling(ctx, delivery.Intent.ReferenceID, invitation.ConveyanceEmail)
			err = issueErr
			issuedPlaintext, expiresAt = issued.Plaintext, issued.ExpiresAt
			revoke = func(revokeCtx context.Context) error {
				return m.invitations.RevokeIssuedGrant(revokeCtx, issuedPlaintext)
			}
			actionPath = "/join#grant="
		}
	case delivery.Intent.Topic == notifications.TopicLocalPasswordRecovery &&
		delivery.Intent.ReferenceKind == notifications.ReferenceRecovery &&
		m.recovery != nil:
		address, err = m.recovery.Contact(ctx, delivery.Intent.RecipientID)
		if err == nil {
			issued, issueErr := m.recovery.IssueGrant(ctx, delivery.Intent.ReferenceID)
			err = issueErr
			issuedPlaintext, expiresAt = issued.Plaintext, issued.ExpiresAt
			revoke = func(revokeCtx context.Context) error {
				return m.recovery.RevokeIssuedGrant(revokeCtx, issuedPlaintext)
			}
			actionPath = "/reset-password#grant="
		}
	default:
		return notifications.MaterializedEmail{}, fmt.Errorf("invalid account email delivery references")
	}
	if errors.Is(err, store.ErrNotFound) {
		return notifications.MaterializedEmail{}, notifications.ErrEmailDestinationUnavailable
	}
	if err != nil {
		return notifications.MaterializedEmail{}, err
	}
	if err != nil {
		return notifications.MaterializedEmail{}, err
	}
	origin, err := recipientOriginValue(m.publicURL)
	if err != nil {
		if revoke != nil {
			_ = revoke(ctx)
		}
		return notifications.MaterializedEmail{}, err
	}
	invalidate := func(revokeCtx context.Context) error {
		return revoke(revokeCtx)
	}
	actionURL := origin + actionPath + url.QueryEscape(issuedPlaintext)
	content, err := notifications.RenderAccountEmail(delivery.Intent, actionURL, expiresAt)
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

type accountDeliveryBuild struct {
	invitations *invitationDeliveryCoordinator
	recovery    *passwordRecoveryCoordinator
	product     *notifications.ProductPublisher
	service     *notifications.Service
	validators  []notifications.DestinationValidator
}

func buildAccountDelivery(
	st store.Store,
	destinations notifications.DestinationRepository,
	providerAdapters []notifications.Adapter,
	set resolved,
	invitationService *invitation.Service,
	recoveryService *auth.PasswordRecoveryService,
	registry *scheduler.Registry,
	log *slog.Logger,
) accountDeliveryBuild {
	if st == nil || set.svc == nil || invitationService == nil || recoveryService == nil {
		return accountDeliveryBuild{}
	}
	materializer := invitationEmailMaterializer{
		invitations: invitationService, recovery: recoveryService,
		contacts:  st,
		publicURL: func() string { return set.str("access.public_url") },
	}
	adapter := notifications.NewEmailAdapter(set.emailConfig, materializer, notifications.NewSMTPSender(15*time.Second))
	adapters := append([]notifications.Adapter{adapter}, providerAdapters...)
	repository := notificationServiceRepository{Store: st, destinations: destinations}
	service := notifications.NewService(repository, combinedNotificationRouter{
		account: invitationEmailRouter{
			invitations: invitationService, recovery: recoveryService, destinations: destinations,
		},
		product: notifications.NewDestinationRouter(destinations, currentNotificationEligibility{users: st}),
	}, adapters, newID, time.Now)
	if registry != nil {
		registry.Add(notificationDeliveryJob(service))
	}
	return accountDeliveryBuild{
		invitations: &invitationDeliveryCoordinator{invitations: invitationService, notifications: service},
		product:     notifications.NewProductPublisher(service),
		service:     service,
		validators:  []notifications.DestinationValidator{adapter},
		recovery: &passwordRecoveryCoordinator{
			recovery: recoveryService, notifications: service,
			requestLimiter: auth.NewRateLimiter(0.05, 3), redeemLimiter: auth.NewRateLimiter(0.1, 5),
			log: log,
		},
	}
}

func productNotificationLink(origin string, intent notifications.Intent) string {
	switch intent.ReferenceKind {
	case notifications.ReferenceProposal:
		return origin + "/queue/approval"
	case notifications.ReferenceChannel:
		return origin + "/channels/" + url.PathEscape(intent.ReferenceID)
	case notifications.ReferenceTitle:
		return origin + "/queue/flight"
	default:
		return origin
	}
}

type notificationServiceRepository struct {
	store.Store
	destinations notifications.DestinationRepository
}

func (r notificationServiceRepository) GetNotificationDestination(
	ctx context.Context,
	id string,
) (notifications.Destination, error) {
	return r.destinations.GetNotificationDestination(ctx, id)
}
