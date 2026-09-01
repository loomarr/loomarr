package notifications

import (
	"fmt"
	"time"
)

// DestinationScope distinguishes administrator-owned shared delivery from one person's verified
// contact or browser subscription. Scope is part of authorization and cannot be inferred from means.
type DestinationScope string

const (
	ScopeInstallation DestinationScope = "installation"
	ScopePerson       DestinationScope = "person"
)

// Destination is the complete module-internal routing record. Configuration and Credentials are
// resolved only after an attempt is claimed; API read models must use DestinationSummary instead.
type Destination struct {
	ID            string
	Means         Means
	Label         string
	Scope         DestinationScope
	OwnerID       string
	Audience      RecipientKind
	Topics        []Topic
	Enabled       bool
	Configuration map[string]string
	Credentials   map[string]string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// DestinationSummary is safe to return to management callers. It deliberately carries neither
// provider configuration nor credentials; provider-specific surfaces add independently redacted
// fields when their adapter is implemented.
type DestinationSummary struct {
	ID                    string
	Means                 Means
	Label                 string
	Scope                 DestinationScope
	OwnerID               string
	Audience              RecipientKind
	Topics                []Topic
	Enabled               bool
	CredentialsConfigured bool
	CreatedAt             time.Time
	UpdatedAt             time.Time
	Health                *DestinationHealth
}

// DestinationHealth is a payload-free, credential-free operational aggregate. OutcomeCode is a
// closed vocabulary; provider error strings and message content never enter this read model.
type DestinationHealth struct {
	LastSuccessAt        time.Time
	LastFailureAt        time.Time
	LastFailureOutcome   OutcomeCode
	QueuedCount          int
	TerminalFailureCount int
}

func (d Destination) Validate() error {
	if err := identifier("destination id", d.ID); err != nil {
		return err
	}
	if !validMeans(d.Means) {
		return fmt.Errorf("invalid delivery means %q", d.Means)
	}
	if d.Label == "" {
		return fmt.Errorf("destination requires a label")
	}
	if err := safeText("destination label", d.Label, 120); err != nil {
		return err
	}
	if d.CreatedAt.IsZero() || d.UpdatedAt.IsZero() || d.UpdatedAt.Before(d.CreatedAt) {
		return fmt.Errorf("destination requires ordered created and updated times")
	}
	switch d.Scope {
	case ScopeInstallation:
		if d.OwnerID != "" {
			return fmt.Errorf("installation destination cannot have a person owner")
		}
		if d.Audience != RecipientApprovers && d.Audience != RecipientOperators {
			return fmt.Errorf("installation destination requires an approver or operator audience")
		}
		if d.Means == MeansEmail || d.Means == MeansWebPush {
			return fmt.Errorf("installation destination cannot use person delivery means %q", d.Means)
		}
	case ScopePerson:
		if err := identifier("destination owner", d.OwnerID); err != nil {
			return err
		}
		if d.Audience != RecipientPerson && d.Audience != RecipientApprovers && d.Audience != RecipientOperators {
			return fmt.Errorf("person destination requires a supported audience")
		}
		if d.Means != MeansEmail && d.Means != MeansWebPush {
			return fmt.Errorf("person destination requires email or web push")
		}
	default:
		return fmt.Errorf("invalid destination scope %q", d.Scope)
	}
	if len(d.Topics) == 0 {
		return fmt.Errorf("destination requires at least one topic")
	}
	seen := make(map[Topic]struct{}, len(d.Topics))
	for _, topic := range d.Topics {
		if _, exists := seen[topic]; exists {
			return fmt.Errorf("destination topic %q is duplicated", topic)
		}
		seen[topic] = struct{}{}
		if !topicSupportsAudience(topic, d.Audience) {
			return fmt.Errorf("topic %q does not support audience %q", topic, d.Audience)
		}
	}
	if err := validateDestinationValues("configuration", d.Configuration, 100, 4000); err != nil {
		return err
	}
	return validateDestinationValues("credentials", d.Credentials, 20, 8000)
}

func (d Destination) Summary() DestinationSummary {
	return DestinationSummary{
		ID: d.ID, Means: d.Means, Label: d.Label, Scope: d.Scope, OwnerID: d.OwnerID,
		Audience: d.Audience, Topics: append([]Topic(nil), d.Topics...), Enabled: d.Enabled,
		CredentialsConfigured: len(d.Credentials) > 0, CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
	}
}

func topicSupportsAudience(topic Topic, audience RecipientKind) bool {
	switch topic {
	case TopicProposalSubmitted:
		return audience == RecipientApprovers
	case TopicProposalApproved, TopicProposalDeclined:
		return audience == RecipientPerson
	case TopicAcquisitionAvailable, TopicAcquisitionGaveUp, TopicChannelLive, TopicChannelDegraded:
		return audience == RecipientPerson || audience == RecipientOperators
	default:
		return false
	}
}

func validateDestinationValues(name string, values map[string]string, limit, valueLimit int) error {
	if len(values) > limit {
		return fmt.Errorf("destination %s has too many fields", name)
	}
	for key, value := range values {
		if err := identifier("destination "+name+" key", key); err != nil {
			return err
		}
		if err := safeText("destination "+name+" value", value, valueLimit); err != nil {
			return err
		}
	}
	return nil
}
