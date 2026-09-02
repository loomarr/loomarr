package notifications

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"sort"
	"time"
)

var (
	ErrForbidden        = errors.New("notifications: forbidden")
	ErrMeansUnavailable = errors.New("notifications: delivery means unavailable")
)

type Principal struct {
	PersonID      string
	Administrator bool
}

type DestinationCommand struct {
	Means    Means
	Label    string
	Scope    DestinationScope
	OwnerID  string
	Audience RecipientKind
	Topics   []Topic
	Enabled  bool
	// Settings is the one public provider input object. When supplied, the server-owned provider
	// definition classifies every field into Configuration or Credentials.
	Settings      map[string]string
	Configuration map[string]string
	// A nil Credentials update preserves the stored credentials; a non-nil empty map explicitly
	// clears them. Create treats nil as empty. Credential values never appear in summaries.
	Credentials *map[string]string
}

type DestinationUpdateCommand struct {
	Label         string
	Audience      RecipientKind
	Topics        []Topic
	Enabled       bool
	Settings      *map[string]string
	Configuration *map[string]string
	Credentials   *map[string]string
}

type DestinationValidator interface {
	Means() Means
	ValidateDestination(configuration, credentials map[string]string) error
}

type DestinationRepository interface {
	SaveNotificationDestination(context.Context, Destination) error
	ResolveNotificationDestination(context.Context, string) (Destination, error)
	OpenNotificationDestinationForUpdate(context.Context, string) (Destination, error)
	GetNotificationDestinationMetadata(context.Context, string) (DestinationMetadata, error)
	ListNotificationDestinationMetadata(context.Context) ([]DestinationMetadata, error)
	ListNotificationDestinationHealth(context.Context) (map[string]DestinationHealth, error)
	DeleteNotificationDestination(context.Context, string) error
	RetireNotificationDestination(context.Context, string) error
}

// DestinationRecordRepository is the storage port below credential protection. Implementations
// must never receive or return plaintext provider credentials.
type DestinationRecordRepository interface {
	SaveNotificationDestinationRecord(context.Context, DestinationRecord) error
	GetNotificationDestinationRecord(context.Context, string) (DestinationRecord, error)
	ListNotificationDestinationRecords(context.Context) ([]DestinationRecord, error)
	ListNotificationDestinationHealth(context.Context) (map[string]DestinationHealth, error)
	DeleteNotificationDestination(context.Context, string) error
}

type DestinationTestResult struct {
	IntentID string
	Created  bool
}

type DestinationDeleteResult struct {
	UnsubscribeCurrentBrowser bool
}

type DestinationTester interface {
	PublishDestinationTest(context.Context, DestinationMetadata, string) (DestinationTestResult, error)
}

type DestinationManager struct {
	repository DestinationRepository
	validators map[Means]DestinationValidator
	newID      func() string
	now        func() time.Time
	tester     DestinationTester
}

func (m *DestinationManager) WithTester(tester DestinationTester) *DestinationManager {
	if m != nil {
		m.tester = tester
	}
	return m
}

func NewDestinationManager(
	repository DestinationRepository,
	validators []DestinationValidator,
	newID func() string,
	now func() time.Time,
) *DestinationManager {
	indexed := make(map[Means]DestinationValidator, len(validators))
	for _, validator := range validators {
		if validator != nil {
			indexed[validator.Means()] = validator
		}
	}
	return &DestinationManager{repository: repository, validators: indexed, newID: newID, now: now}
}

func (m *DestinationManager) Create(
	ctx context.Context,
	principal Principal,
	command DestinationCommand,
) (DestinationSummary, error) {
	if err := authorizeDestinationWrite(principal, command.Scope, command.OwnerID); err != nil {
		return DestinationSummary{}, err
	}
	if err := authorizeDestinationAudience(principal, command.Scope, command.Audience); err != nil {
		return DestinationSummary{}, err
	}
	if m == nil || m.repository == nil || m.newID == nil || m.now == nil {
		return DestinationSummary{}, fmt.Errorf("notification destination manager is unavailable")
	}
	now := m.now().UTC().Truncate(time.Second)
	destination := Destination{
		ID: m.newID(), Means: command.Means, Label: command.Label, Scope: command.Scope,
		OwnerID: command.OwnerID, Audience: command.Audience, Topics: append([]Topic(nil), command.Topics...),
		Enabled: command.Enabled, Configuration: cloneStringMap(command.Configuration),
		CreatedAt: now, UpdatedAt: now,
	}
	if command.Settings != nil {
		configuration, credentials, err := classifyDestinationSettings(command.Means, command.Settings)
		if err != nil {
			return DestinationSummary{}, err
		}
		destination.Configuration, destination.Credentials = configuration, credentials
	} else if command.Credentials != nil {
		destination.Credentials = cloneStringMap(*command.Credentials)
	}
	setWebPushSubscriptionFingerprint(&destination)
	if destination.Means == MeansWebPush && destination.SubscriptionFingerprint == "" {
		return DestinationSummary{}, fmt.Errorf("web push destination requires a subscription endpoint")
	}
	if err := destination.Validate(); err != nil {
		return DestinationSummary{}, err
	}
	if err := m.validateEnabled(destination); err != nil {
		return DestinationSummary{}, err
	}
	if destination.Means == MeansWebPush {
		return m.createOrReplaceWebPushDestination(ctx, destination)
	}
	if err := m.repository.SaveNotificationDestination(ctx, destination); err != nil {
		return DestinationSummary{}, err
	}
	return destination.Summary(), nil
}

func (m *DestinationManager) List(ctx context.Context, principal Principal) ([]DestinationSummary, error) {
	if principal.PersonID == "" && !principal.Administrator {
		return nil, ErrForbidden
	}
	if m == nil || m.repository == nil {
		return nil, fmt.Errorf("notification destination manager is unavailable")
	}
	destinations, err := m.repository.ListNotificationDestinationMetadata(ctx)
	if err != nil {
		return nil, err
	}
	health, err := m.repository.ListNotificationDestinationHealth(ctx)
	if err != nil {
		return nil, err
	}
	summaries := make([]DestinationSummary, 0, len(destinations))
	for _, destination := range destinations {
		if destination.Scope == ScopeInstallation && principal.Administrator ||
			destination.Scope == ScopePerson && destination.OwnerID == principal.PersonID {
			summary := destination.Summary()
			if principal.Administrator {
				value := health[destination.ID]
				summary.Health = &value
			}
			summaries = append(summaries, summary)
		}
	}
	return summaries, nil
}

func (m *DestinationManager) Update(
	ctx context.Context,
	principal Principal,
	id string,
	command DestinationUpdateCommand,
) (DestinationSummary, error) {
	if m == nil || m.repository == nil || m.now == nil {
		return DestinationSummary{}, fmt.Errorf("notification destination manager is unavailable")
	}
	metadata, err := m.repository.GetNotificationDestinationMetadata(ctx, id)
	if err != nil {
		return DestinationSummary{}, err
	}
	if err := authorizeDestinationWrite(principal, metadata.Scope, metadata.OwnerID); err != nil {
		return DestinationSummary{}, err
	}
	audience := command.Audience
	if audience == "" {
		audience = metadata.Audience
	}
	if err := authorizeDestinationAudience(principal, metadata.Scope, audience); err != nil {
		return DestinationSummary{}, err
	}
	current, err := m.repository.OpenNotificationDestinationForUpdate(ctx, id)
	if err != nil {
		return DestinationSummary{}, err
	}
	credentials := current.Credentials
	if command.Credentials != nil {
		credentials = cloneStringMap(*command.Credentials)
	}
	configuration := current.Configuration
	if command.Configuration != nil {
		configuration = cloneStringMap(*command.Configuration)
	}
	if command.Settings != nil {
		configuration, credentials, err = mergeDestinationSettings(
			current.Means, configuration, credentials, *command.Settings,
		)
		if err != nil {
			return DestinationSummary{}, err
		}
	}
	updated := Destination{
		ID: current.ID, Means: current.Means, Label: command.Label, Scope: current.Scope,
		OwnerID: current.OwnerID, Audience: audience, Topics: append([]Topic(nil), command.Topics...),
		Enabled: command.Enabled, Configuration: configuration,
		Credentials: credentials, CreatedAt: current.CreatedAt, UpdatedAt: m.now().UTC().Truncate(time.Second),
	}
	setWebPushSubscriptionFingerprint(&updated)
	if updated.Means == MeansWebPush && updated.SubscriptionFingerprint == "" {
		return DestinationSummary{}, fmt.Errorf("web push destination requires a subscription endpoint")
	}
	if err := updated.Validate(); err != nil {
		return DestinationSummary{}, err
	}
	if err := m.validateEnabled(updated); err != nil {
		return DestinationSummary{}, err
	}
	if err := m.repository.SaveNotificationDestination(ctx, updated); err != nil {
		return DestinationSummary{}, err
	}
	return updated.Summary(), nil
}

func classifyDestinationSettings(means Means, settings map[string]string) (map[string]string, map[string]string, error) {
	definition, ok := ProviderDefinitionFor(means)
	if !ok {
		return nil, nil, ErrMeansUnavailable
	}
	return definition.Classify(settings)
}

func mergeDestinationSettings(
	means Means,
	configuration map[string]string,
	credentials map[string]string,
	settings map[string]string,
) (map[string]string, map[string]string, error) {
	definition, ok := ProviderDefinitionFor(means)
	if !ok {
		return nil, nil, ErrMeansUnavailable
	}
	configuration = cloneStringMap(configuration)
	credentials = cloneStringMap(credentials)
	if configuration == nil {
		configuration = make(map[string]string)
	}
	if credentials == nil {
		credentials = make(map[string]string)
	}
	for key, value := range settings {
		field, exists := definition.Field(key)
		if !exists {
			return nil, nil, fmt.Errorf("provider %q does not define field %q", means, key)
		}
		if err := validateProviderFieldStorage(field, value); err != nil {
			return nil, nil, err
		}
		target := configuration
		if field.Sensitive {
			target = credentials
		}
		if value == "" {
			delete(target, key)
		} else {
			target[key] = value
		}
	}
	return configuration, credentials, nil
}

func (m *DestinationManager) Delete(
	ctx context.Context,
	principal Principal,
	id string,
	currentBrowserEndpoint string,
) (DestinationDeleteResult, error) {
	if m == nil || m.repository == nil {
		return DestinationDeleteResult{}, fmt.Errorf("notification destination manager is unavailable")
	}
	current, err := m.repository.GetNotificationDestinationMetadata(ctx, id)
	if err != nil {
		return DestinationDeleteResult{}, err
	}
	if err := authorizeDestinationWrite(principal, current.Scope, current.OwnerID); err != nil {
		return DestinationDeleteResult{}, err
	}
	result := DestinationDeleteResult{}
	if current.Means == MeansWebPush && currentBrowserEndpoint != "" {
		fingerprint := current.SubscriptionFingerprint
		if fingerprint == "" {
			opened, openErr := m.repository.OpenNotificationDestinationForUpdate(ctx, id)
			if openErr != nil {
				return DestinationDeleteResult{}, openErr
			}
			fingerprint = webPushSubscriptionFingerprint(opened.Credentials["endpoint"])
		}
		result.UnsubscribeCurrentBrowser = sameFingerprint(
			fingerprint,
			webPushSubscriptionFingerprint(currentBrowserEndpoint),
		)
	}
	if err := m.repository.DeleteNotificationDestination(ctx, id); err != nil {
		return DestinationDeleteResult{}, err
	}
	return result, nil
}

func (m *DestinationManager) createOrReplaceWebPushDestination(
	ctx context.Context,
	destination Destination,
) (DestinationSummary, error) {
	matches, err := m.matchingWebPushDestinations(ctx, destination)
	if err != nil {
		return DestinationSummary{}, err
	}
	if len(matches) > 0 {
		return m.replaceWebPushDestinations(ctx, destination, matches)
	}
	if err := m.repository.SaveNotificationDestination(ctx, destination); err == nil {
		return destination.Summary(), nil
	} else if !errors.Is(err, ErrConflict) {
		return DestinationSummary{}, err
	}
	// A concurrent create may have inserted the same owner/subscription after the first lookup.
	matches, err = m.matchingWebPushDestinations(ctx, destination)
	if err != nil {
		return DestinationSummary{}, err
	}
	if len(matches) == 0 {
		return DestinationSummary{}, ErrConflict
	}
	return m.replaceWebPushDestinations(ctx, destination, matches)
}

func (m *DestinationManager) matchingWebPushDestinations(
	ctx context.Context,
	destination Destination,
) ([]DestinationMetadata, error) {
	metadata, err := m.repository.ListNotificationDestinationMetadata(ctx)
	if err != nil {
		return nil, err
	}
	matches := make([]DestinationMetadata, 0, 1)
	for _, candidate := range metadata {
		if candidate.Means != MeansWebPush || candidate.OwnerID != destination.OwnerID {
			continue
		}
		fingerprint := candidate.SubscriptionFingerprint
		if fingerprint == "" {
			legacy, openErr := m.repository.OpenNotificationDestinationForUpdate(ctx, candidate.ID)
			if openErr != nil {
				return nil, openErr
			}
			fingerprint = webPushSubscriptionFingerprint(legacy.Credentials["endpoint"])
		}
		if sameFingerprint(fingerprint, destination.SubscriptionFingerprint) {
			matches = append(matches, candidate)
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].CreatedAt.Equal(matches[j].CreatedAt) {
			return matches[i].ID < matches[j].ID
		}
		return matches[i].CreatedAt.Before(matches[j].CreatedAt)
	})
	return matches, nil
}

func (m *DestinationManager) replaceWebPushDestinations(
	ctx context.Context,
	destination Destination,
	matches []DestinationMetadata,
) (DestinationSummary, error) {
	canonical := matches[0]
	destination.ID = canonical.ID
	destination.CreatedAt = canonical.CreatedAt
	if destination.UpdatedAt.Before(destination.CreatedAt) {
		destination.UpdatedAt = destination.CreatedAt
	}
	if err := m.repository.SaveNotificationDestination(ctx, destination); err != nil {
		return DestinationSummary{}, err
	}
	for _, duplicate := range matches[1:] {
		if err := m.repository.DeleteNotificationDestination(ctx, duplicate.ID); err != nil {
			return DestinationSummary{}, fmt.Errorf("remove duplicate web push destination: %w", err)
		}
	}
	return destination.Summary(), nil
}

func setWebPushSubscriptionFingerprint(destination *Destination) {
	if destination != nil && destination.Means == MeansWebPush {
		destination.SubscriptionFingerprint = webPushSubscriptionFingerprint(destination.Credentials["endpoint"])
	}
}

func webPushSubscriptionFingerprint(endpoint string) string {
	if endpoint == "" {
		return ""
	}
	// A Push service generates this high-entropy opaque endpoint; it is not a human password.
	// The deterministic digest supports equality and uniqueness without persisting the bearer URL.
	// codeql[go/weak-sensitive-data-hashing]
	digest := sha256.Sum256([]byte(endpoint))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

func sameFingerprint(left, right string) bool {
	return left != "" && right != "" && subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}

func (m *DestinationManager) Test(
	ctx context.Context,
	principal Principal,
	id string,
	requestID string,
) (DestinationTestResult, error) {
	if m == nil || m.repository == nil || m.tester == nil {
		return DestinationTestResult{}, fmt.Errorf("notification destination testing is unavailable")
	}
	if err := identifier("destination test request id", requestID); err != nil {
		return DestinationTestResult{}, err
	}
	destination, err := m.repository.GetNotificationDestinationMetadata(ctx, id)
	if err != nil {
		return DestinationTestResult{}, err
	}
	if err := authorizeDestinationWrite(principal, destination.Scope, destination.OwnerID); err != nil {
		return DestinationTestResult{}, err
	}
	if !destination.Enabled {
		return DestinationTestResult{}, fmt.Errorf("notification destination must be enabled before testing")
	}
	return m.tester.PublishDestinationTest(ctx, destination, requestID)
}

func (m *DestinationManager) validateEnabled(destination Destination) error {
	if !destination.Enabled {
		return nil
	}
	validator := m.validators[destination.Means]
	if validator == nil {
		return ErrMeansUnavailable
	}
	if err := validator.ValidateDestination(destination.Configuration, destination.Credentials); err != nil {
		return fmt.Errorf("validate %s destination: %w", destination.Means, err)
	}
	return nil
}

func authorizeDestinationWrite(principal Principal, scope DestinationScope, ownerID string) error {
	switch scope {
	case ScopeInstallation:
		if !principal.Administrator {
			return ErrForbidden
		}
	case ScopePerson:
		if principal.PersonID == "" || ownerID != principal.PersonID {
			return ErrForbidden
		}
	default:
		// Let domain validation return the more specific invalid-scope error for an authorized caller.
		if !principal.Administrator {
			return ErrForbidden
		}
	}
	return nil
}

func authorizeDestinationAudience(principal Principal, scope DestinationScope, audience RecipientKind) error {
	if scope == ScopePerson && audience != RecipientPerson && !principal.Administrator {
		return ErrForbidden
	}
	return nil
}

func cloneStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}
