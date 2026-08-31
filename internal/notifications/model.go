// Package notifications owns channel-neutral notification intents and delivery work (§11).
// Callers publish typed domain events; adapters alone know how a delivery means executes them.
package notifications

import (
	"fmt"
	"strings"
	"time"
	"unicode"
)

const Retention = 30 * 24 * time.Hour

type Topic string

const (
	TopicAccountInvitation     Topic = "account_invitation"
	TopicLocalPasswordRecovery Topic = "local_password_recovery"
)

type RecipientKind string

const (
	RecipientInvitation RecipientKind = "invitation"
	RecipientPerson     RecipientKind = "person"
)

type ReferenceKind string

const (
	ReferenceInvitation ReferenceKind = "invitation"
	ReferenceRecovery   ReferenceKind = "recovery"
)

// RecipientPolicy distinguishes account/security delivery from future preference-controlled topics.
type RecipientPolicy string

const (
	PolicyMandatoryAccount RecipientPolicy = "mandatory_account"
	PolicyConfigurable     RecipientPolicy = "configurable"
)

type Means string

const MeansEmail Means = "email"

type Status string

const (
	StatusQueued     Status = "queued"
	StatusSending    Status = "sending"
	StatusDelivered  Status = "delivered"
	StatusFailed     Status = "failed"
	StatusSuppressed Status = "suppressed"
)

type FailureClass string

const (
	FailureNone                   FailureClass = ""
	FailureTransientPreAcceptance FailureClass = "transient_pre_acceptance"
	FailurePermanent              FailureClass = "permanent"
	FailureAmbiguous              FailureClass = "ambiguous_acceptance"
	FailureCancelled              FailureClass = "cancelled"
)

// OutcomeCode is deliberately closed and non-secret. Adapters map provider detail to one of these
// operator-actionable classes instead of persisting arbitrary error strings.
type OutcomeCode string

const (
	OutcomeNone                   OutcomeCode = ""
	OutcomeDeliveryDisabled       OutcomeCode = "delivery_disabled"
	OutcomeDestinationUnavailable OutcomeCode = "destination_unavailable"
	OutcomePreferenceDisabled     OutcomeCode = "preference_disabled"
	OutcomeMeansUnavailable       OutcomeCode = "means_unavailable"
	OutcomeRecipientRejected      OutcomeCode = "recipient_rejected"
	OutcomeConfigurationInvalid   OutcomeCode = "configuration_invalid"
	OutcomeTransportUnavailable   OutcomeCode = "transport_unavailable"
	OutcomeAcceptanceAmbiguous    OutcomeCode = "acceptance_ambiguous"
	OutcomeCancelled              OutcomeCode = "cancelled"
	OutcomeWorkerInterrupted      OutcomeCode = "worker_interrupted"
)

// TemplateData is intentionally narrow. Secure values remain on the referenced domain record and
// are materialized only in memory when an attempt begins.
type TemplateData struct {
	RecipientName string `json:"recipientName,omitempty"`
}

type Intent struct {
	ID             string
	Topic          Topic
	RecipientKind  RecipientKind
	RecipientID    string
	ReferenceKind  ReferenceKind
	ReferenceID    string
	Policy         RecipientPolicy
	Template       TemplateData
	IdempotencyKey string
	CreatedAt      time.Time
	TerminalAt     time.Time
}

// Route is the module-owned routing decision persisted with the initial attempt. DestinationRef is
// an opaque durable lookup key, never an address or bearer URL; DestinationRedacted is display-safe.
type Route struct {
	Means               Means
	DestinationRef      string
	DestinationRedacted string
	Suppressed          OutcomeCode
}

type Attempt struct {
	ID                  string
	IntentID            string
	Means               Means
	DestinationRef      string
	DestinationRedacted string
	Status              Status
	AttemptNumber       int
	AvailableAt         time.Time
	LeaseOwner          string
	LeaseExpiresAt      time.Time
	StartedAt           time.Time
	FinishedAt          time.Time
	ProviderMessageID   string
	FailureClass        FailureClass
	OutcomeCode         OutcomeCode
	CreatedAt           time.Time
}

type Completion struct {
	AttemptID         string
	LeaseOwner        string
	Status            Status
	ProviderMessageID string
	FailureClass      FailureClass
	OutcomeCode       OutcomeCode
	FinishedAt        time.Time
	Next              *Attempt
}

// DeliverySummary is the provider-safe read model composed into the owning domain's UI. It never
// carries the destination reference, recipient address, rendered body, or bearer grant.
type DeliverySummary struct {
	Status            Status
	AttemptNumber     int
	OutcomeCode       OutcomeCode
	ProviderMessageID string
	UpdatedAt         time.Time
}

func (c Completion) Validate(current Attempt) error {
	if err := identifier("attempt id", c.AttemptID); err != nil {
		return err
	}
	if err := identifier("lease owner", c.LeaseOwner); err != nil {
		return err
	}
	if c.FinishedAt.IsZero() {
		return fmt.Errorf("delivery completion requires finished time")
	}
	if c.Status != StatusDelivered && c.Status != StatusFailed && c.Status != StatusSuppressed {
		return fmt.Errorf("delivery completion requires a terminal status")
	}
	if err := validateOutcome(c.Status, c.FailureClass, c.OutcomeCode, c.ProviderMessageID); err != nil {
		return err
	}
	if c.Next == nil {
		return nil
	}
	if c.Status != StatusFailed || c.FailureClass != FailureTransientPreAcceptance {
		return fmt.Errorf("only pre-acceptance transient failure may create a retry")
	}
	if err := c.Next.Validate(); err != nil {
		return fmt.Errorf("validate next delivery attempt: %w", err)
	}
	if c.Next.IntentID != current.IntentID || c.Next.Means != current.Means ||
		c.Next.DestinationRef != current.DestinationRef ||
		c.Next.DestinationRedacted != current.DestinationRedacted ||
		c.Next.AttemptNumber != current.AttemptNumber+1 || c.Next.Status != StatusQueued {
		return fmt.Errorf("retry must preserve route and advance exactly one attempt")
	}
	return nil
}

func (i Intent) Validate() error {
	if err := identifier("intent id", i.ID); err != nil {
		return err
	}
	if err := identifier("recipient id", i.RecipientID); err != nil {
		return err
	}
	if err := identifier("reference id", i.ReferenceID); err != nil {
		return err
	}
	if err := identifier("idempotency key", i.IdempotencyKey); err != nil {
		return err
	}
	if i.Policy != PolicyMandatoryAccount && i.Policy != PolicyConfigurable {
		return fmt.Errorf("invalid recipient policy %q", i.Policy)
	}
	if i.Policy != PolicyMandatoryAccount {
		return fmt.Errorf("topic %q does not support configurable recipient policy", i.Topic)
	}
	switch i.Topic {
	case TopicAccountInvitation:
		if i.RecipientKind != RecipientInvitation || i.ReferenceKind != ReferenceInvitation || i.RecipientID != i.ReferenceID {
			return fmt.Errorf("account invitation must reference its invitation recipient")
		}
	case TopicLocalPasswordRecovery:
		if i.RecipientKind != RecipientPerson || i.ReferenceKind != ReferenceRecovery {
			return fmt.Errorf("local password recovery must reference a person and recovery record")
		}
	default:
		return fmt.Errorf("invalid notification topic %q", i.Topic)
	}
	if i.CreatedAt.IsZero() {
		return fmt.Errorf("notification intent requires created time")
	}
	if err := safeText("recipient name", i.Template.RecipientName, 200); err != nil {
		return err
	}
	return nil
}

func (r Route) Validate() error {
	if r.Means != MeansEmail {
		return fmt.Errorf("invalid delivery means %q", r.Means)
	}
	if err := identifier("destination reference", r.DestinationRef); err != nil {
		return err
	}
	if err := safeText("redacted destination", r.DestinationRedacted, 320); err != nil {
		return err
	}
	if r.DestinationRedacted == "" {
		return fmt.Errorf("route requires a redacted destination")
	}
	if r.Suppressed != OutcomeNone && !validSuppression(r.Suppressed) {
		return fmt.Errorf("invalid suppression code %q", r.Suppressed)
	}
	return nil
}

func (a Attempt) Validate() error {
	if err := identifier("attempt id", a.ID); err != nil {
		return err
	}
	if err := identifier("intent id", a.IntentID); err != nil {
		return err
	}
	if err := (Route{Means: a.Means, DestinationRef: a.DestinationRef, DestinationRedacted: a.DestinationRedacted}).Validate(); err != nil {
		return err
	}
	if a.AttemptNumber < 1 || a.AttemptNumber > MaxAttempts {
		return fmt.Errorf("attempt number %d outside 1..%d", a.AttemptNumber, MaxAttempts)
	}
	if a.AvailableAt.IsZero() || a.CreatedAt.IsZero() {
		return fmt.Errorf("delivery attempt requires available and created times")
	}
	if !validStatus(a.Status) {
		return fmt.Errorf("invalid delivery status %q", a.Status)
	}
	switch a.Status {
	case StatusQueued:
		if a.LeaseOwner != "" || !a.LeaseExpiresAt.IsZero() || !a.StartedAt.IsZero() || !a.FinishedAt.IsZero() {
			return fmt.Errorf("queued attempt cannot carry lease or execution times")
		}
	case StatusSending:
		if err := identifier("lease owner", a.LeaseOwner); err != nil {
			return err
		}
		if a.LeaseExpiresAt.IsZero() || a.StartedAt.IsZero() || !a.FinishedAt.IsZero() {
			return fmt.Errorf("sending attempt requires an active lease and start time")
		}
	case StatusDelivered, StatusFailed, StatusSuppressed:
		if a.LeaseOwner != "" || !a.LeaseExpiresAt.IsZero() || a.FinishedAt.IsZero() {
			return fmt.Errorf("terminal attempt requires a finish time and no active lease")
		}
	}
	return validateOutcome(a.Status, a.FailureClass, a.OutcomeCode, a.ProviderMessageID)
}

func validateOutcome(status Status, class FailureClass, code OutcomeCode, messageID string) error {
	if err := safeText("provider message id", messageID, 200); err != nil {
		return err
	}
	switch status {
	case StatusQueued, StatusSending:
		if class != FailureNone || code != OutcomeNone || messageID != "" {
			return fmt.Errorf("active attempt cannot carry a terminal outcome")
		}
	case StatusDelivered:
		if class != FailureNone || code != OutcomeNone {
			return fmt.Errorf("delivered attempt cannot carry a failure")
		}
	case StatusFailed:
		if !validFailure(class) || class == FailureNone || !validFailureCode(code) {
			return fmt.Errorf("failed attempt requires a bounded failure class and code")
		}
		if messageID != "" {
			return fmt.Errorf("failed attempt cannot claim a provider message id")
		}
	case StatusSuppressed:
		if class != FailureNone || !validSuppression(code) || messageID != "" {
			return fmt.Errorf("suppressed attempt requires a suppression code only")
		}
	default:
		return fmt.Errorf("invalid delivery status %q", status)
	}
	return nil
}

func validStatus(status Status) bool {
	return status == StatusQueued || status == StatusSending || status == StatusDelivered ||
		status == StatusFailed || status == StatusSuppressed
}

func validFailure(class FailureClass) bool {
	return class == FailureNone || class == FailureTransientPreAcceptance || class == FailurePermanent ||
		class == FailureAmbiguous || class == FailureCancelled
}

func validFailureCode(code OutcomeCode) bool {
	return code == OutcomeMeansUnavailable || code == OutcomeRecipientRejected ||
		code == OutcomeConfigurationInvalid || code == OutcomeTransportUnavailable ||
		code == OutcomeAcceptanceAmbiguous || code == OutcomeCancelled || code == OutcomeWorkerInterrupted
}

func validSuppression(code OutcomeCode) bool {
	return code == OutcomeDeliveryDisabled || code == OutcomeDestinationUnavailable || code == OutcomePreferenceDisabled
}

func identifier(name, value string) error {
	if value == "" || len(value) > 200 {
		return fmt.Errorf("%s must contain 1..200 characters", name)
	}
	return safeText(name, value, 200)
}

func safeText(name, value string, limit int) error {
	if len(value) > limit || strings.ContainsAny(value, "\r\n") || strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return fmt.Errorf("%s contains unsafe or excessive text", name)
	}
	return nil
}
