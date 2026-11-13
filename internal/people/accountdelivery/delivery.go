// Package accountdelivery delivers the messages an account depends on: invitation emails and
// password-recovery emails. It routes each intent, mints its single-use grant only inside a
// claimed delivery attempt, renders the message, and runs the shared delivery worker; product
// events reach the same service through the publisher it builds.
package accountdelivery

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

	"github.com/loomarr/loomarr/internal/people/access"
	"github.com/loomarr/loomarr/internal/people/auth"
	"github.com/loomarr/loomarr/internal/people/contact"
	"github.com/loomarr/loomarr/internal/people/notifications"
	"github.com/loomarr/loomarr/internal/platform/jobs"
	"github.com/loomarr/loomarr/internal/platform/settings"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/identitystore"
)

const notificationWorkerLimit = 100

// InvitationDelivery sends an invitation's email and reports its latest delivery.
type InvitationDelivery struct {
	invitations   *access.Service
	notifications *notifications.Service
}

func (c *InvitationDelivery) SendEmail(
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
	if value.Status != access.StatusPending {
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

func (c *InvitationDelivery) LatestEmail(
	ctx context.Context,
	invitationID string,
) (notifications.DeliverySummary, error) {
	return c.notifications.LatestDelivery(ctx, notifications.ReferenceInvitation, invitationID)
}

type invitationEmailRouter struct {
	invitations  *access.Service
	recovery     *auth.PasswordRecoveryService
	destinations notifications.DestinationSource
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
	if e.users == nil || (audience != notifications.RecipientPerson &&
		audience != notifications.RecipientApprovers && audience != notifications.RecipientOperators) {
		return false, nil
	}
	user, err := e.users.GetUser(ctx, personID)
	if errors.Is(err, store.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if user.Disabled {
		return false, nil
	}
	if audience == notifications.RecipientPerson {
		return true, nil
	}
	return user.Role == identitystore.RoleAdmin, nil
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
		route.Suppressed = notifications.OutcomeDeliveryDisabled
		return []notifications.Route{route}, nil
	}
	destinations, listErr := r.destinations.ListNotificationDestinationMetadata(ctx)
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
	invitations *access.Service
	recovery    *auth.PasswordRecoveryService
	contacts    productEmailRecipientReader
	publicURL   func() string
}

type productEmailRecipientReader interface {
	GetContactAddresses(context.Context, string) (contact.Set, error)
	ListUsers(context.Context) ([]identitystore.User, error)
}

func (m invitationEmailMaterializer) Materialize(
	ctx context.Context,
	delivery notifications.Delivery,
) (notifications.MaterializedEmail, error) {
	if delivery.Intent.Policy == notifications.PolicyConfigurable && m.contacts != nil {
		link := ""
		if origin, originErr := recipientOriginValue(m.publicURL); originErr == nil {
			link = productNotificationLink(origin, delivery.Intent)
		}
		content, err := notifications.RenderProductEmail(delivery.Intent, link)
		if err != nil {
			return notifications.MaterializedEmail{}, err
		}
		if delivery.Intent.RecipientKind == notifications.RecipientPerson {
			addresses, addressErr := m.contacts.GetContactAddresses(ctx, delivery.Intent.RecipientID)
			if addressErr != nil {
				if errors.Is(addressErr, store.ErrNotFound) {
					return notifications.MaterializedEmail{}, notifications.ErrEmailDestinationUnavailable
				}
				return notifications.MaterializedEmail{}, addressErr
			}
			if addresses.Verified == nil {
				return notifications.MaterializedEmail{}, notifications.ErrEmailDestinationUnavailable
			}
			return notifications.MaterializedEmail{Message: notifications.EmailMessage{
				ToAddress: addresses.Verified.Email, Subject: content.Subject,
				TextBody: content.TextBody, HTMLBody: content.HTMLBody,
			}}, nil
		}
		if delivery.Intent.RecipientKind != notifications.RecipientApprovers &&
			delivery.Intent.RecipientKind != notifications.RecipientOperators {
			return notifications.MaterializedEmail{}, notifications.ErrEmailDestinationUnavailable
		}
		users, listErr := m.contacts.ListUsers(ctx)
		if listErr != nil {
			return notifications.MaterializedEmail{}, listErr
		}
		messages := make([]notifications.EmailMessage, 0, len(users))
		for _, user := range users {
			if user.Disabled || user.Role != identitystore.RoleAdmin {
				continue
			}
			addresses, addressErr := m.contacts.GetContactAddresses(ctx, user.ID)
			if addressErr != nil {
				if errors.Is(addressErr, store.ErrNotFound) {
					continue
				}
				return notifications.MaterializedEmail{}, addressErr
			}
			if addresses.Verified == nil {
				continue
			}
			messages = append(messages, notifications.EmailMessage{
				ToAddress: addresses.Verified.Email, ToName: user.Name, Subject: content.Subject,
				TextBody: content.TextBody, HTMLBody: content.HTMLBody,
			})
		}
		if len(messages) == 0 {
			return notifications.MaterializedEmail{}, notifications.ErrEmailDestinationUnavailable
		}
		return notifications.MaterializedEmail{Messages: messages}, nil
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
			issued, issueErr := m.invitations.IssueSibling(ctx, delivery.Intent.ReferenceID, access.ConveyanceEmail)
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

func notificationDeliveryJob(service *notifications.Service) jobs.Job {
	return jobs.Job{
		Name: "notification-delivery", Group: jobs.GroupSystem, Title: "Deliver account messages",
		Description: "Sends queued invitation and recovery messages through their configured delivery means.",
		DefaultCron: "*/15 * * * * *", ScheduleKey: settings.JobNotificationDeliverySchedule.Key(),
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

// Delivery is the account-message delivery stack: the coordinators the API drives, the product
// publisher, the notification service and the destination validators.
type Delivery struct {
	Invitations *InvitationDelivery
	Recovery    *PasswordRecovery
	Product     *notifications.ProductPublisher
	Service     *notifications.Service
	Validators  []notifications.DestinationValidator
}

// Store is the identity and contact persistence the delivery stack reads.
type Store interface {
	store.UserStore
	productEmailRecipientReader
}

// Config is what Build needs.
type Config struct {
	Store            Store
	Repository       notifications.Repository
	ProviderAdapters []notifications.Adapter
	// PublicURL resolves access.public_url at call time.
	PublicURL   func() string
	Invitations *access.Service
	Recovery    *auth.PasswordRecoveryService
	// Registry receives the notification-delivery job; nil registers nothing.
	Registry *jobs.Registry
	Log      *slog.Logger
	NewID    func() string
}

// Build assembles the delivery stack. Without a store, an invitation service or a recovery
// service it returns the zero Delivery.
func Build(c Config) Delivery {
	if c.Store == nil || c.Invitations == nil || c.Recovery == nil {
		return Delivery{}
	}
	materializer := invitationEmailMaterializer{
		invitations: c.Invitations, recovery: c.Recovery,
		contacts:  c.Store,
		publicURL: c.PublicURL,
	}
	adapter := notifications.NewEmailAdapter(materializer, notifications.NewSMTPSender(15*time.Second))
	adapters := append([]notifications.Adapter{adapter}, c.ProviderAdapters...)
	service := notifications.NewService(c.Repository, combinedNotificationRouter{
		account: invitationEmailRouter{
			invitations: c.Invitations, recovery: c.Recovery, destinations: c.Repository,
		},
		product: notifications.NewDestinationRouter(c.Repository, currentNotificationEligibility{users: c.Store}),
	}, adapters, c.NewID, time.Now)
	if c.Registry != nil {
		c.Registry.Add(notificationDeliveryJob(service))
	}
	return Delivery{
		Invitations: &InvitationDelivery{invitations: c.Invitations, notifications: service},
		Product:     notifications.NewProductPublisher(service),
		Service:     service,
		Validators:  []notifications.DestinationValidator{adapter},
		Recovery: &PasswordRecovery{
			recovery: c.Recovery, notifications: service,
			requestLimiter: newRecoveryRequestLimiter(), redeemLimiter: auth.NewRateLimiter(0.1, 5),
			log: c.Log,
		},
	}
}

func productNotificationLink(origin string, intent notifications.Intent) string {
	switch intent.ReferenceKind {
	case notifications.ReferenceProposal:
		return origin + "/requests/needs-you"
	case notifications.ReferenceChannel:
		return origin + "/channels/" + url.PathEscape(intent.ReferenceID)
	case notifications.ReferenceTitle:
		return origin + "/requests/in-progress"
	default:
		return origin
	}
}
