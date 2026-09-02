package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/notifications"
)

func (s *Server) registerNotificationDestinations(api huma.API) {
	huma.Register(api, withRole(huma.Operation{
		OperationID: "notification-provider-types-list", Method: http.MethodGet,
		Path: "/v1/notifications/provider-types", Summary: "List notification provider types",
		Description: "Returns the server-owned provider fields and compatible events used to render the Add provider form. No configured values or credentials are returned.",
		Tags:        []string{"notifications"},
	}, RoleMember), s.notificationProviderTypesList)
	huma.Register(api, withRole(huma.Operation{
		OperationID: "notification-providers-list", Method: http.MethodGet,
		Path: "/v1/notifications/providers", Summary: "List notification providers",
		Description: "Returns the configured provider list with safe field values, write-only secret status, selected events, and delivery health.",
		Tags:        []string{"notifications"},
	}, RoleMember), s.notificationDestinationsList)
	huma.Register(api, withRole(huma.Operation{
		OperationID: "notification-providers-create", Method: http.MethodPost,
		Path: "/v1/notifications/providers", Summary: "Add a notification provider",
		Description: "Adds one provider from its server-defined settings and selected events. Sensitive fields are classified and stored by the server and remain write-only.",
		Tags:        []string{"notifications"}, DefaultStatus: http.StatusCreated,
	}, RoleMember), s.notificationDestinationCreate)
	huma.Register(api, withRole(huma.Operation{
		OperationID: "notification-providers-update", Method: http.MethodPut,
		Path: "/v1/notifications/providers/{id}", Summary: "Update a notification provider",
		Description: "Omitting a sensitive setting preserves it; sending that field with an empty value clears it.",
		Tags:        []string{"notifications"},
	}, RoleMember), s.notificationDestinationUpdate)
	huma.Register(api, withRole(huma.Operation{
		OperationID: "notification-providers-delete", Method: http.MethodDelete,
		Path: "/v1/notifications/providers/{id}", Summary: "Delete a notification provider",
		Description: "Queued attempts suppress at execution when their destination no longer exists. Browser Push may include this browser's endpoint in the request body; the response says whether the client should unsubscribe that local subscription.",
		Tags:        []string{"notifications"},
	}, RoleMember), s.notificationDestinationDelete)
	huma.Register(api, withRole(huma.Operation{
		OperationID: "notification-providers-test", Method: http.MethodPost,
		Path: "/v1/notifications/providers/{id}/test", Summary: "Queue a notification provider test",
		Description: "Queues a distinct test-delivery intent. Acceptance means Loomarr durably accepted the handoff, not that the provider confirmed final delivery.",
		Tags:        []string{"notifications"}, DefaultStatus: http.StatusAccepted,
	}, RoleMember), s.notificationDestinationTest)
}

type NotificationProviderFieldOptionDTO struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type NotificationProviderFieldDTO struct {
	Key         string                               `json:"key"`
	Label       string                               `json:"label"`
	Kind        notifications.ProviderFieldKind      `json:"kind" enum:"text,password,url,number,select,toggle,textarea"`
	Required    bool                                 `json:"required"`
	Sensitive   bool                                 `json:"sensitive"`
	Default     string                               `json:"default,omitempty"`
	Options     []NotificationProviderFieldOptionDTO `json:"options,omitempty"`
	Description string                               `json:"description,omitempty"`
}

type NotificationProviderTypeDTO struct {
	Type        notifications.Means            `json:"type"`
	Name        string                         `json:"name"`
	Fields      []NotificationProviderFieldDTO `json:"fields"`
	Events      []notifications.Topic          `json:"events"`
	MemberOwned bool                           `json:"memberOwned"`
}

type notificationProviderTypesListOutput struct {
	Body struct {
		Providers        []NotificationProviderTypeDTO `json:"providers"`
		WebPushPublicKey string                        `json:"webPushPublicKey,omitempty"`
	}
}

func (s *Server) notificationProviderTypesList(
	ctx context.Context,
	_ *struct{},
) (*notificationProviderTypesListOutput, error) {
	out := &notificationProviderTypesListOutput{}
	out.Body.WebPushPublicKey = s.webPushPublicKey
	definitions := notifications.ProviderDefinitions()
	out.Body.Providers = make([]NotificationProviderTypeDTO, 0, len(definitions))
	for _, definition := range definitions {
		if roleFrom(ctx) != RoleAdmin && !definition.MemberOwned {
			continue
		}
		provider := NotificationProviderTypeDTO{
			Type: definition.Means, Name: definition.Name,
			Events:      append([]notifications.Topic(nil), definition.Topics...),
			MemberOwned: definition.MemberOwned,
			Fields:      make([]NotificationProviderFieldDTO, 0, len(definition.Fields)),
		}
		for _, field := range definition.Fields {
			item := NotificationProviderFieldDTO{
				Key: field.Key, Label: field.Label, Kind: field.Kind, Required: field.Required,
				Sensitive: field.Sensitive, Default: field.Default, Description: field.Description,
				Options: make([]NotificationProviderFieldOptionDTO, 0, len(field.Options)),
			}
			for _, option := range field.Options {
				item.Options = append(item.Options, NotificationProviderFieldOptionDTO{
					Value: option.Value, Label: option.Label,
				})
			}
			provider.Fields = append(provider.Fields, item)
		}
		out.Body.Providers = append(out.Body.Providers, provider)
	}
	return out, nil
}

type notificationDestinationWrite struct {
	Type     notifications.Means   `json:"type" enum:"email,webhook,discord,ntfy,gotify,apprise,pushover,telegram,mattermost,matrix,web_push,mqtt,slack"`
	Label    string                `json:"label" minLength:"1" maxLength:"120"`
	Events   []notifications.Topic `json:"events" minItems:"1"`
	Enabled  bool                  `json:"enabled"`
	Settings map[string]string     `json:"settings"`
}

type NotificationProviderSettingDTO struct {
	Key              string `json:"key"`
	Value            string `json:"value,omitempty"`
	SecretConfigured bool   `json:"secretConfigured"`
}

type NotificationProviderDTO struct {
	ID        string                            `json:"id"`
	Type      notifications.Means               `json:"type"`
	Label     string                            `json:"label"`
	Events    []notifications.Topic             `json:"events"`
	Enabled   bool                              `json:"enabled"`
	Settings  []NotificationProviderSettingDTO  `json:"settings"`
	CreatedAt time.Time                         `json:"createdAt"`
	UpdatedAt time.Time                         `json:"updatedAt"`
	Health    *NotificationDestinationHealthDTO `json:"health,omitempty"`
}

type NotificationDestinationHealthDTO struct {
	LastSuccessAt        *time.Time                `json:"lastSuccessAt,omitempty"`
	LastFailureAt        *time.Time                `json:"lastFailureAt,omitempty"`
	LastFailureOutcome   notifications.OutcomeCode `json:"lastFailureOutcome,omitempty"`
	QueuedCount          int                       `json:"queuedCount"`
	TerminalFailureCount int                       `json:"terminalFailureCount"`
}

type notificationDestinationsListOutput struct {
	Body struct {
		Providers []NotificationProviderDTO `json:"providers"`
	}
}

type notificationDestinationCreateInput struct{ Body notificationDestinationWrite }
type notificationDestinationCreateOutput struct{ Body NotificationProviderDTO }

type notificationDestinationUpdateWrite struct {
	Label    string                `json:"label" minLength:"1" maxLength:"120"`
	Events   []notifications.Topic `json:"events" minItems:"1"`
	Enabled  bool                  `json:"enabled"`
	Settings *map[string]string    `json:"settings,omitempty" doc:"Changed provider fields. Omit sensitive fields to preserve them; send an empty value to clear one."`
}

type notificationDestinationUpdateInput struct {
	ID   string `path:"id"`
	Body notificationDestinationUpdateWrite
}
type notificationDestinationUpdateOutput struct{ Body NotificationProviderDTO }
type notificationDestinationDeleteInput struct {
	ID   string `path:"id"`
	Body *struct {
		CurrentBrowserEndpoint string `json:"currentBrowserEndpoint,omitempty" maxLength:"8000" doc:"The current browser's protected Push endpoint, used only to decide whether this local subscription matches the selected row."`
	}
}
type notificationDestinationDeleteOutput struct {
	Body struct {
		UnsubscribeCurrentBrowser bool `json:"unsubscribeCurrentBrowser"`
	}
}
type notificationDestinationTestInput struct {
	ID   string `path:"id"`
	Body struct {
		RequestID string `json:"requestId" minLength:"1" maxLength:"100" doc:"Caller-generated key that makes a repeated test request idempotent."`
	}
}
type notificationDestinationTestOutput struct {
	Body struct {
		IntentID string `json:"intentId"`
		Queued   bool   `json:"queued"`
		Hint     string `json:"hint"`
	}
}

func (s *Server) notificationDestinationsList(
	ctx context.Context,
	_ *struct{},
) (*notificationDestinationsListOutput, error) {
	if s.notificationDestinations == nil {
		return nil, errNotImplemented("Notification destinations unavailable", "Destination management isn't available in this Loomarr process.")
	}
	summaries, err := s.notificationDestinations.List(ctx, notificationPrincipal(ctx))
	if err != nil {
		return nil, notificationDestinationError(err)
	}
	out := &notificationDestinationsListOutput{}
	out.Body.Providers = make([]NotificationProviderDTO, 0, len(summaries))
	for _, summary := range summaries {
		out.Body.Providers = append(out.Body.Providers, notificationDestinationDTO(summary))
	}
	return out, nil
}

func (s *Server) notificationDestinationCreate(
	ctx context.Context,
	in *notificationDestinationCreateInput,
) (*notificationDestinationCreateOutput, error) {
	if s.notificationDestinations == nil {
		return nil, errNotImplemented("Notification destinations unavailable", "Destination management isn't available in this Loomarr process.")
	}
	principal := notificationPrincipal(ctx)
	scope := notifications.ScopeInstallation
	ownerID := ""
	audience := notifications.RecipientOperators
	if definition, ok := notifications.ProviderDefinitionFor(in.Body.Type); ok && definition.MemberOwned {
		scope = notifications.ScopePerson
		ownerID = principal.PersonID
		audience = notifications.RecipientPerson
	}
	summary, err := s.notificationDestinations.Create(ctx, principal, notifications.DestinationCommand{
		Means: in.Body.Type, Label: in.Body.Label, Scope: scope, OwnerID: ownerID,
		Audience: audience, Topics: in.Body.Events, Enabled: in.Body.Enabled,
		Settings: in.Body.Settings,
	})
	if err != nil {
		return nil, notificationDestinationError(err)
	}
	return &notificationDestinationCreateOutput{Body: notificationDestinationDTO(summary)}, nil
}

func (s *Server) notificationDestinationUpdate(
	ctx context.Context,
	in *notificationDestinationUpdateInput,
) (*notificationDestinationUpdateOutput, error) {
	if s.notificationDestinations == nil {
		return nil, errNotImplemented("Notification destinations unavailable", "Destination management isn't available in this Loomarr process.")
	}
	summary, err := s.notificationDestinations.Update(ctx, notificationPrincipal(ctx), in.ID, notifications.DestinationUpdateCommand{
		Label:  in.Body.Label,
		Topics: in.Body.Events, Enabled: in.Body.Enabled, Settings: in.Body.Settings,
	})
	if err != nil {
		return nil, notificationDestinationError(err)
	}
	return &notificationDestinationUpdateOutput{Body: notificationDestinationDTO(summary)}, nil
}

func (s *Server) notificationDestinationDelete(
	ctx context.Context,
	in *notificationDestinationDeleteInput,
) (*notificationDestinationDeleteOutput, error) {
	if s.notificationDestinations == nil {
		return nil, errNotImplemented("Notification destinations unavailable", "Destination management isn't available in this Loomarr process.")
	}
	currentBrowserEndpoint := ""
	if in.Body != nil {
		currentBrowserEndpoint = in.Body.CurrentBrowserEndpoint
	}
	result, err := s.notificationDestinations.Delete(
		ctx,
		notificationPrincipal(ctx),
		in.ID,
		currentBrowserEndpoint,
	)
	if err != nil {
		return nil, notificationDestinationError(err)
	}
	out := &notificationDestinationDeleteOutput{}
	out.Body.UnsubscribeCurrentBrowser = result.UnsubscribeCurrentBrowser
	return out, nil
}

func (s *Server) notificationDestinationTest(
	ctx context.Context,
	in *notificationDestinationTestInput,
) (*notificationDestinationTestOutput, error) {
	if s.notificationDestinations == nil {
		return nil, errNotImplemented("Notification destinations unavailable", "Destination testing isn't available in this Loomarr process.")
	}
	result, err := s.notificationDestinations.Test(ctx, notificationPrincipal(ctx), in.ID, in.Body.RequestID)
	if err != nil {
		return nil, notificationDestinationError(err)
	}
	out := &notificationDestinationTestOutput{}
	out.Body.IntentID = result.IntentID
	out.Body.Queued = true
	out.Body.Hint = "Test notification queued. Check delivery health for the final provider result."
	return out, nil
}

func notificationPrincipal(ctx context.Context) notifications.Principal {
	principal := notifications.Principal{Administrator: roleFrom(ctx) == RoleAdmin}
	if user, ok := userFrom(ctx); ok {
		principal.PersonID = user.ID
	}
	return principal
}

func notificationDestinationDTO(summary notifications.DestinationSummary) NotificationProviderDTO {
	dto := NotificationProviderDTO{
		ID: summary.ID, Type: summary.Means, Label: summary.Label, Events: summary.Topics,
		Enabled: summary.Enabled, CreatedAt: summary.CreatedAt, UpdatedAt: summary.UpdatedAt,
		Settings: make([]NotificationProviderSettingDTO, 0, len(summary.Settings)),
	}
	for _, setting := range summary.Settings {
		dto.Settings = append(dto.Settings, NotificationProviderSettingDTO{
			Key: setting.Key, Value: setting.Value, SecretConfigured: setting.SecretConfigured,
		})
	}
	if summary.Health != nil {
		health := NotificationDestinationHealthDTO{
			LastFailureOutcome: summary.Health.LastFailureOutcome, QueuedCount: summary.Health.QueuedCount,
			TerminalFailureCount: summary.Health.TerminalFailureCount,
		}
		if !summary.Health.LastSuccessAt.IsZero() {
			value := summary.Health.LastSuccessAt
			health.LastSuccessAt = &value
		}
		if !summary.Health.LastFailureAt.IsZero() {
			value := summary.Health.LastFailureAt
			health.LastFailureAt = &value
		}
		dto.Health = &health
	}
	return dto
}

func notificationDestinationError(err error) error {
	switch {
	case errors.Is(err, notifications.ErrForbidden):
		return apiErr(http.StatusForbidden, "Notification destination unavailable", "You can manage only installation destinations allowed for your role and personal destinations owned by your account.")
	case errors.Is(err, notifications.ErrNotFound):
		return errNotFound("Notification destination not found", "That notification destination doesn't exist or has already been deleted.")
	case errors.Is(err, notifications.ErrMeansUnavailable):
		return errUnprocessable("Notification provider unavailable", "That provider cannot be enabled until its adapter is available and fully configured.")
	case errors.Is(err, notifications.ErrConflict):
		return apiErr(http.StatusConflict, "Notification provider conflicts with an existing destination", "This browser subscription is already assigned to another provider row.")
	default:
		return errUnprocessable("Notification destination is invalid", "Check the provider, audience, selected events, and required configuration, then try again.")
	}
}
