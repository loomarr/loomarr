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

// Destination is the complete module-internal delivery record. Credentials are resolved only after
// an attempt is claimed or an authorized update begins; API reads use DestinationSummary instead.
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
	// SubscriptionFingerprint is a non-secret one-way identifier used only to make one person's
	// Browser Push endpoint unique and to match deletion to the current browser.
	SubscriptionFingerprint string
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

// DestinationMetadata is the credential-free routing and management read model. CredentialKeys
// records only which server-defined sensitive fields are configured; it never contains values.
type DestinationMetadata struct {
	ID                      string
	Means                   Means
	Label                   string
	Scope                   DestinationScope
	OwnerID                 string
	Audience                RecipientKind
	Topics                  []Topic
	Enabled                 bool
	Configuration           map[string]string
	CredentialKeys          []string
	SubscriptionFingerprint string
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

// DestinationRecord is the persistence-safe form of a destination. The database layer only
// handles an opaque credential envelope; plaintext credentials exist solely in Destination at
// the notification module boundary after authenticated decryption.
type DestinationRecord struct {
	ID                      string
	Means                   Means
	Label                   string
	Scope                   DestinationScope
	OwnerID                 string
	Audience                RecipientKind
	Topics                  []Topic
	Enabled                 bool
	Configuration           map[string]string
	CredentialKeys          []string
	CredentialsEncrypted    string
	SubscriptionFingerprint string
	CreatedAt               time.Time
	UpdatedAt               time.Time
}

func (r DestinationRecord) Validate() error {
	if r.CredentialsEncrypted == "" {
		return fmt.Errorf("destination record requires encrypted credentials")
	}
	seen := make(map[string]struct{}, len(r.CredentialKeys))
	for _, key := range r.CredentialKeys {
		if err := identifier("destination credential metadata key", key); err != nil {
			return err
		}
		if _, exists := seen[key]; exists {
			return fmt.Errorf("destination credential metadata key %q is duplicated", key)
		}
		seen[key] = struct{}{}
	}
	return destinationFromRecord(r).Validate()
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
	Settings              []ProviderFieldState
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
		definition, ok := ProviderDefinitionFor(d.Means)
		if !ok || definition.MemberOwned {
			return fmt.Errorf("installation destination cannot use member-owned delivery means %q", d.Means)
		}
	case ScopePerson:
		if err := identifier("destination owner", d.OwnerID); err != nil {
			return err
		}
		if d.Audience != RecipientPerson && d.Audience != RecipientApprovers && d.Audience != RecipientOperators {
			return fmt.Errorf("person destination requires a supported audience")
		}
		definition, ok := ProviderDefinitionFor(d.Means)
		if d.Means != MeansEmail && (!ok || !definition.MemberOwned) {
			return fmt.Errorf("person destination requires email or web push")
		}
	default:
		return fmt.Errorf("invalid destination scope %q", d.Scope)
	}
	if len(d.Topics) == 0 {
		// A migrated SMTP provider may initially serve mandatory Invitation and recovery mail only.
		// Product events remain opt-in, so migration must not invent subscriptions.
		if d.Means != MeansEmail || d.Scope != ScopeInstallation {
			return fmt.Errorf("destination requires at least one topic")
		}
	}
	seen := make(map[Topic]struct{}, len(d.Topics))
	definition, _ := ProviderDefinitionFor(d.Means)
	for _, topic := range d.Topics {
		if _, exists := seen[topic]; exists {
			return fmt.Errorf("destination topic %q is duplicated", topic)
		}
		seen[topic] = struct{}{}
		if !providerSupportsTopic(definition, topic) {
			return fmt.Errorf("provider %q does not support topic %q", d.Means, topic)
		}
	}
	if err := validateDestinationValues("configuration", d.Configuration, 100, 4000); err != nil {
		return err
	}
	if d.SubscriptionFingerprint != "" {
		if d.Means != MeansWebPush {
			return fmt.Errorf("only web push destinations may have a subscription fingerprint")
		}
		if err := identifier("web push subscription fingerprint", d.SubscriptionFingerprint); err != nil {
			return err
		}
	}
	return validateDestinationValues("credentials", d.Credentials, 20, 8000)
}

func (d Destination) Summary() DestinationSummary {
	return d.Metadata().Summary()
}

func (d Destination) Metadata() DestinationMetadata {
	keys := make([]string, 0, len(d.Credentials))
	for key, value := range d.Credentials {
		if value != "" {
			keys = append(keys, key)
		}
	}
	return DestinationMetadata{
		ID: d.ID, Means: d.Means, Label: d.Label, Scope: d.Scope, OwnerID: d.OwnerID,
		Audience: d.Audience, Topics: append([]Topic(nil), d.Topics...), Enabled: d.Enabled,
		Configuration: cloneStringMap(d.Configuration), CredentialKeys: keys,
		SubscriptionFingerprint: d.SubscriptionFingerprint,
		CreatedAt:               d.CreatedAt, UpdatedAt: d.UpdatedAt,
	}
}

func (d DestinationMetadata) Validate() error {
	return d.destination().Validate()
}

func (d DestinationMetadata) Summary() DestinationSummary {
	definition, _ := ProviderDefinitionFor(d.Means)
	configured := make(map[string]string, len(d.CredentialKeys))
	for _, key := range d.CredentialKeys {
		configured[key] = "configured"
	}
	return DestinationSummary{
		ID: d.ID, Means: d.Means, Label: d.Label, Scope: d.Scope, OwnerID: d.OwnerID,
		Audience: d.Audience, Topics: append([]Topic(nil), d.Topics...), Enabled: d.Enabled,
		CredentialsConfigured: len(d.CredentialKeys) > 0, CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
		Settings: definition.Redact(d.Configuration, configured),
	}
}

func (d DestinationMetadata) destination() Destination {
	return Destination{
		ID: d.ID, Means: d.Means, Label: d.Label, Scope: d.Scope, OwnerID: d.OwnerID,
		Audience: d.Audience, Topics: append([]Topic(nil), d.Topics...), Enabled: d.Enabled,
		Configuration: cloneStringMap(d.Configuration), SubscriptionFingerprint: d.SubscriptionFingerprint,
		CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt,
	}
}

func providerSupportsTopic(definition ProviderDefinition, topic Topic) bool {
	for _, supported := range definition.Topics {
		if supported == topic {
			return true
		}
	}
	return false
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
