package notifications

import (
	"context"
	"errors"
	"fmt"
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
	GetNotificationDestination(context.Context, string) (Destination, error)
	ListNotificationDestinations(context.Context) ([]Destination, error)
	ListNotificationDestinationHealth(context.Context) (map[string]DestinationHealth, error)
	DeleteNotificationDestination(context.Context, string) error
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

type DestinationTester interface {
	PublishDestinationTest(context.Context, Destination, string) (DestinationTestResult, error)
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
	if err := destination.Validate(); err != nil {
		return DestinationSummary{}, err
	}
	if err := m.validateEnabled(destination); err != nil {
		return DestinationSummary{}, err
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
	destinations, err := m.repository.ListNotificationDestinations(ctx)
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
	current, err := m.repository.GetNotificationDestination(ctx, id)
	if err != nil {
		return DestinationSummary{}, err
	}
	if err := authorizeDestinationWrite(principal, current.Scope, current.OwnerID); err != nil {
		return DestinationSummary{}, err
	}
	audience := command.Audience
	if audience == "" {
		audience = current.Audience
	}
	if err := authorizeDestinationAudience(principal, current.Scope, audience); err != nil {
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

func (m *DestinationManager) Delete(ctx context.Context, principal Principal, id string) error {
	if m == nil || m.repository == nil {
		return fmt.Errorf("notification destination manager is unavailable")
	}
	current, err := m.repository.GetNotificationDestination(ctx, id)
	if err != nil {
		return err
	}
	if err := authorizeDestinationWrite(principal, current.Scope, current.OwnerID); err != nil {
		return err
	}
	return m.repository.DeleteNotificationDestination(ctx, id)
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
	destination, err := m.repository.GetNotificationDestination(ctx, id)
	if err != nil {
		return DestinationTestResult{}, err
	}
	if err := authorizeDestinationWrite(principal, destination.Scope, destination.OwnerID); err != nil {
		return DestinationTestResult{}, err
	}
	if !destination.Enabled {
		return DestinationTestResult{}, fmt.Errorf("notification destination must be enabled before testing")
	}
	if err := m.validateEnabled(destination); err != nil {
		return DestinationTestResult{}, err
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
