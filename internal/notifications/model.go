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
	TopicProposalSubmitted     Topic = "proposal_submitted"
	TopicProposalApproved      Topic = "proposal_approved"
	TopicProposalDeclined      Topic = "proposal_declined"
	TopicAcquisitionAvailable  Topic = "acquisition_available"
	TopicAcquisitionGaveUp     Topic = "acquisition_gave_up"
	TopicChannelLive           Topic = "channel_live"
	TopicChannelDegraded       Topic = "channel_degraded"
	TopicDeliveryTest          Topic = "delivery_test"
)

type RecipientKind string

const (
	RecipientInvitation RecipientKind = "invitation"
	RecipientPerson     RecipientKind = "person"
	RecipientApprovers  RecipientKind = "approvers"
	RecipientOperators  RecipientKind = "operators"
)

type ReferenceKind string

const (
	ReferenceInvitation  ReferenceKind = "invitation"
	ReferenceRecovery    ReferenceKind = "recovery"
	ReferenceProposal    ReferenceKind = "proposal"
	ReferenceTitle       ReferenceKind = "title"
	ReferenceChannel     ReferenceKind = "channel"
	ReferenceDestination ReferenceKind = "destination"
)

// RecipientPolicy distinguishes account/security delivery from future preference-controlled topics.
type RecipientPolicy string

const (
	PolicyMandatoryAccount RecipientPolicy = "mandatory_account"
	PolicyConfigurable     RecipientPolicy = "configurable"
)

type Means string

const (
	MeansEmail      Means = "email"
	MeansWebhook    Means = "webhook"
	MeansDiscord    Means = "discord"
	MeansNtfy       Means = "ntfy"
	MeansGotify     Means = "gotify"
	MeansApprise    Means = "apprise"
	MeansPushover   Means = "pushover"
	MeansTelegram   Means = "telegram"
	MeansMattermost Means = "mattermost"
	MeansMatrix     Means = "matrix"
	MeansWebPush    Means = "web_push"
	MeansMQTT       Means = "mqtt"
	MeansSlack      Means = "slack"
)

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

// TemplateData is intentionally narrow. Account secrets remain on the referenced domain record and
// are materialized only in memory when an attempt begins; product fields preserve bounded event-time
// display text, never a rendered body or provider payload.
type TemplateData struct {
	RecipientName string `json:"recipientName,omitempty"`
	SubjectName   string `json:"subjectName,omitempty"`
	Summary       string `json:"summary,omitempty"`
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
	switch i.Topic {
	case TopicAccountInvitation:
		if i.Policy != PolicyMandatoryAccount {
			return fmt.Errorf("account invitation requires mandatory account policy")
		}
		if i.RecipientKind != RecipientInvitation || i.ReferenceKind != ReferenceInvitation || i.RecipientID != i.ReferenceID {
			return fmt.Errorf("account invitation must reference its invitation recipient")
		}
	case TopicLocalPasswordRecovery:
		if i.Policy != PolicyMandatoryAccount {
			return fmt.Errorf("local password recovery requires mandatory account policy")
		}
		if i.RecipientKind != RecipientPerson || i.ReferenceKind != ReferenceRecovery {
			return fmt.Errorf("local password recovery must reference a person and recovery record")
		}
	case TopicProposalSubmitted:
		if err := i.validateProduct(RecipientApprovers, ReferenceProposal); err != nil {
			return err
		}
	case TopicProposalApproved, TopicProposalDeclined:
		if err := i.validateProduct(RecipientPerson, ReferenceProposal); err != nil {
			return err
		}
	case TopicAcquisitionAvailable, TopicAcquisitionGaveUp:
		if err := i.validateProductAudience(ReferenceTitle); err != nil {
			return err
		}
	case TopicChannelLive, TopicChannelDegraded:
		if err := i.validateProductAudience(ReferenceChannel); err != nil {
			return err
		}
	case TopicDeliveryTest:
		if i.Policy != PolicyConfigurable || i.ReferenceKind != ReferenceDestination ||
			i.ReferenceID != i.RecipientID {
			return fmt.Errorf("delivery test must reference its destination")
		}
		if i.RecipientKind != RecipientPerson && i.RecipientKind != RecipientApprovers && i.RecipientKind != RecipientOperators {
			return fmt.Errorf("delivery test requires a supported destination audience")
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
	if err := safeText("subject name", i.Template.SubjectName, 200); err != nil {
		return err
	}
	if err := safeText("summary", i.Template.Summary, 500); err != nil {
		return err
	}
	if i.Policy == PolicyMandatoryAccount && (i.Template.SubjectName != "" || i.Template.Summary != "") {
		return fmt.Errorf("account notification cannot persist product display data")
	}
	if i.Policy == PolicyConfigurable && i.Template.SubjectName == "" {
		return fmt.Errorf("product notification requires a subject name")
	}
	return nil
}

func (i Intent) validateProduct(recipient RecipientKind, reference ReferenceKind) error {
	if i.Policy != PolicyConfigurable {
		return fmt.Errorf("product notification requires configurable policy")
	}
	if i.RecipientKind != recipient || i.ReferenceKind != reference {
		return fmt.Errorf("topic %q requires %q recipient and %q reference", i.Topic, recipient, reference)
	}
	if recipient == RecipientApprovers && i.RecipientID != string(RecipientApprovers) {
		return fmt.Errorf("approver audience requires the canonical recipient id")
	}
	if recipient == RecipientOperators && i.RecipientID != string(RecipientOperators) {
		return fmt.Errorf("operator audience requires the canonical recipient id")
	}
	return nil
}

func (i Intent) validateProductAudience(reference ReferenceKind) error {
	if i.RecipientKind != RecipientPerson && i.RecipientKind != RecipientOperators {
		return fmt.Errorf("topic %q requires a person or operator audience", i.Topic)
	}
	return i.validateProduct(i.RecipientKind, reference)
}

func (r Route) Validate() error {
	if !validMeans(r.Means) {
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

func validMeans(means Means) bool {
	switch means {
	case MeansEmail, MeansWebhook, MeansDiscord, MeansNtfy, MeansGotify, MeansApprise,
		MeansPushover, MeansTelegram, MeansMattermost, MeansMatrix, MeansWebPush,
		MeansMQTT, MeansSlack:
		return true
	default:
		return false
	}
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
		code == OutcomeDestinationUnavailable || code == OutcomeConfigurationInvalid || code == OutcomeTransportUnavailable ||
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
