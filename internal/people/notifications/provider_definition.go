package notifications

import (
	"fmt"
	"net/url"
	"strings"
)

// ProviderFieldKind is a presentation hint for one server-defined provider field. It does not
// control persistence; Sensitive is authoritative and is interpreted only by the server.
type ProviderFieldKind string

const (
	ProviderFieldText     ProviderFieldKind = "text"
	ProviderFieldPassword ProviderFieldKind = "password"
	ProviderFieldURL      ProviderFieldKind = "url"
	ProviderFieldNumber   ProviderFieldKind = "number"
	ProviderFieldSelect   ProviderFieldKind = "select"
	ProviderFieldToggle   ProviderFieldKind = "toggle"
	ProviderFieldTextarea ProviderFieldKind = "textarea"
)

type ProviderFieldOption struct {
	Value string
	Label string
}

// ProviderField defines one provider-specific input. Sensitive fields are always classified into
// encrypted credential material regardless of how a client submits them.
type ProviderField struct {
	Key         string
	Label       string
	Kind        ProviderFieldKind
	Required    bool
	Sensitive   bool
	Default     string
	Options     []ProviderFieldOption
	Description string
}

type ProviderFieldState struct {
	Key              string
	Value            string
	SecretConfigured bool
}

// ProviderDefinition is the server-owned contract used by validation, persistence classification,
// API metadata, and the provider-specific Settings form.
type ProviderDefinition struct {
	Means       Means
	Name        string
	Fields      []ProviderField
	Topics      []Topic
	MemberOwned bool
}

func (d ProviderDefinition) Validate() error {
	if !validMeans(d.Means) {
		return fmt.Errorf("provider definition has invalid means %q", d.Means)
	}
	if d.Name == "" {
		return fmt.Errorf("provider %q requires a display name", d.Means)
	}
	seen := make(map[string]struct{}, len(d.Fields))
	for _, field := range d.Fields {
		if err := identifier("provider field key", field.Key); err != nil {
			return err
		}
		if _, exists := seen[field.Key]; exists {
			return fmt.Errorf("provider %q repeats field %q", d.Means, field.Key)
		}
		seen[field.Key] = struct{}{}
		if field.Label == "" {
			return fmt.Errorf("provider %q field %q requires a label", d.Means, field.Key)
		}
		switch field.Kind {
		case ProviderFieldText, ProviderFieldPassword, ProviderFieldURL, ProviderFieldNumber,
			ProviderFieldSelect, ProviderFieldToggle, ProviderFieldTextarea:
		default:
			return fmt.Errorf("provider %q field %q has invalid kind %q", d.Means, field.Key, field.Kind)
		}
		if field.Sensitive && field.Default != "" {
			return fmt.Errorf("provider %q sensitive field %q cannot have a default", d.Means, field.Key)
		}
	}
	if len(d.Topics) == 0 {
		return fmt.Errorf("provider %q requires supported events", d.Means)
	}
	return nil
}

func (d ProviderDefinition) Field(key string) (ProviderField, bool) {
	for _, field := range d.Fields {
		if field.Key == key {
			return field, true
		}
	}
	return ProviderField{}, false
}

// Classify splits one provider-settings object using the server definition. Unknown keys fail
// closed, preventing a client from inventing a plaintext bucket for secret material.
func (d ProviderDefinition) Classify(settings map[string]string) (map[string]string, map[string]string, error) {
	configuration := make(map[string]string)
	credentials := make(map[string]string)
	for key, value := range settings {
		field, ok := d.Field(key)
		if !ok {
			return nil, nil, fmt.Errorf("provider %q does not define field %q", d.Means, key)
		}
		if err := validateProviderFieldStorage(field, value); err != nil {
			return nil, nil, err
		}
		if field.Sensitive {
			credentials[key] = value
		} else {
			configuration[key] = value
		}
	}
	return configuration, credentials, nil
}

func validateProviderFieldStorage(field ProviderField, value string) error {
	if field.Sensitive || field.Kind != ProviderFieldURL || strings.TrimSpace(value) == "" {
		return nil
	}
	parsed, err := url.Parse(strings.TrimSpace(value))
	if strings.Contains(value, "?") || (err == nil && parsed.User != nil) {
		return fmt.Errorf("ordinary provider URL field %q cannot contain credentials or query parameters", field.Key)
	}
	return nil
}

// Redact returns ordered form state without ever projecting sensitive values.
func (d ProviderDefinition) Redact(configuration, credentials map[string]string) []ProviderFieldState {
	states := make([]ProviderFieldState, 0, len(d.Fields))
	for _, field := range d.Fields {
		state := ProviderFieldState{Key: field.Key}
		if field.Sensitive {
			state.SecretConfigured = credentials[field.Key] != ""
		} else {
			state.Value = configuration[field.Key]
		}
		states = append(states, state)
	}
	return states
}

var productProviderTopics = []Topic{
	TopicProposalSubmitted,
	TopicProposalApproved,
	TopicProposalDeclined,
	TopicAcquisitionAvailable,
	TopicAcquisitionGaveUp,
	TopicChannelLive,
	TopicChannelDegraded,
	TopicLibraryFolderOffline,
	TopicLibraryFolderBack,
	TopicLibraryChangesDegraded,
	TopicLibraryNeedsYou,
}

var sharedProviderTopics = []Topic{
	TopicProposalSubmitted,
	TopicAcquisitionAvailable,
	TopicAcquisitionGaveUp,
	TopicChannelLive,
	TopicChannelDegraded,
	TopicLibraryFolderOffline,
	TopicLibraryFolderBack,
	TopicLibraryChangesDegraded,
	TopicLibraryNeedsYou,
}

var providerDefinitions = []ProviderDefinition{
	{
		Means: MeansEmail, Name: "SMTP", Topics: productProviderTopics,
		Fields: []ProviderField{
			plain(keyHost, "SMTP host", ProviderFieldText, true),
			withDefault(plain(keyPort, "Port", ProviderFieldNumber, true), "587"),
			withOptions(withDefault(plain(keySecurity, "Security", ProviderFieldSelect, true), "starttls"),
				ProviderFieldOption{Value: "starttls", Label: "STARTTLS"},
				ProviderFieldOption{Value: "tls", Label: "TLS"},
				ProviderFieldOption{Value: "none", Label: "None"}),
			plain(keyFromAddress, "From address", ProviderFieldText, true),
			plain(keyFromName, "From name", ProviderFieldText, false),
			plain(keyUsername, "Username", ProviderFieldText, false),
			secret(keyPassword, "Password", ProviderFieldPassword, false),
		},
	},
	{
		Means: MeansWebhook, Name: "Webhook", Topics: sharedProviderTopics,
		Fields: []ProviderField{
			secret(keyURL, "Webhook URL", ProviderFieldURL, true),
			secret(keyBearerToken, "Bearer token", ProviderFieldPassword, false),
			secret(keyHMACSecret, "HMAC signing secret", ProviderFieldPassword, false),
		},
	},
	webhookDefinition(MeansDiscord, "Discord", "Discord webhook URL"),
	{
		Means: MeansNtfy, Name: "ntfy", Topics: sharedProviderTopics,
		Fields: []ProviderField{
			plain(keyBaseURL, "Server URL", ProviderFieldURL, true),
			secret(keyTopic, "Topic", ProviderFieldPassword, true),
			plain(keyUsername, "Username", ProviderFieldText, false),
			secret(keyPassword, "Password or token", ProviderFieldPassword, false),
		},
	},
	{
		Means: MeansGotify, Name: "Gotify", Topics: sharedProviderTopics,
		Fields: []ProviderField{
			plain(keyServerURL, "Server URL", ProviderFieldURL, true),
			secret(keyApplicationToken, "Application token", ProviderFieldPassword, true),
		},
	},
	{
		Means: MeansApprise, Name: "Apprise API", Topics: sharedProviderTopics,
		Fields: []ProviderField{
			plain(keyBaseURL, "Apprise API URL", ProviderFieldURL, true),
			secret(keyConfigurationKey, "Configuration key", ProviderFieldPassword, false),
			secret(keyDestinationURL, "Stateless destination URL", ProviderFieldPassword, false),
			secret(keyToken, "Authentication token", ProviderFieldPassword, false),
		},
	},
	{
		Means: MeansPushover, Name: "Pushover", Topics: sharedProviderTopics,
		Fields: []ProviderField{
			secret(keyApplicationToken, "Application token", ProviderFieldPassword, true),
			secret(keyRecipientKey, "User or group key", ProviderFieldPassword, true),
			secret(keyDevice, "Device", ProviderFieldPassword, false),
		},
	},
	{
		Means: MeansTelegram, Name: "Telegram Bot", Topics: sharedProviderTopics,
		Fields: []ProviderField{
			secret(keyBotToken, "Bot token", ProviderFieldPassword, true),
			secret(keyChatID, "Chat ID", ProviderFieldPassword, true),
			secret(keyThreadID, "Topic or thread ID", ProviderFieldPassword, false),
		},
	},
	webhookDefinition(MeansMattermost, "Mattermost", "Incoming webhook URL"),
	{
		Means: MeansMatrix, Name: "Matrix", Topics: sharedProviderTopics,
		Fields: []ProviderField{
			plain(keyHomeserver, "Homeserver URL", ProviderFieldURL, true),
			secret(keyRoomID, "Room ID", ProviderFieldPassword, true),
			secret(keyAccessToken, "Access token", ProviderFieldPassword, true),
		},
	},
	{
		Means: MeansWebPush, Name: "Browser Push", Topics: productProviderTopics, MemberOwned: true,
		Fields: []ProviderField{
			secret(keyEndpoint, "Push endpoint", ProviderFieldPassword, true),
			secret(keyP256dh, "Browser public key", ProviderFieldPassword, true),
			secret(keyAuth, "Browser authentication secret", ProviderFieldPassword, true),
		},
	},
	{
		Means: MeansMQTT, Name: "MQTT", Topics: sharedProviderTopics,
		Fields: []ProviderField{
			secret(keyBrokerURL, "Broker URL", ProviderFieldPassword, true),
			described(plain(keyClientID, "Client ID", ProviderFieldText, false),
				"Optional. Loomarr derives a stable client ID when blank."),
			plain(keyUsername, "Username", ProviderFieldText, false),
			secret(keyPassword, "Password", ProviderFieldPassword, false),
			plain(keyBaseTopic, "Base topic", ProviderFieldText, true),
			withOptions(withDefault(plain(keyQoS, "QoS", ProviderFieldSelect, false), "0"),
				ProviderFieldOption{Value: "0", Label: "At most once"},
				ProviderFieldOption{Value: "1", Label: "At least once"}),
			withDefault(plain(keyRetain, "Retain messages", ProviderFieldToggle, false), "false"),
			described(secret(keyTLSCACertificate, "TLS CA certificate", ProviderFieldTextarea, false),
				"Optional PEM certificate authority for a private broker."),
			described(secret(keyTLSClientCertificate, "TLS client certificate", ProviderFieldTextarea, false),
				"Optional PEM certificate for mutual TLS; requires a client key."),
			described(secret(keyTLSClientKey, "TLS client key", ProviderFieldTextarea, false),
				"Optional PEM private key for mutual TLS; requires a client certificate."),
		},
	},
	webhookDefinition(MeansSlack, "Slack", "Slack webhook URL"),
}

func ProviderDefinitions() []ProviderDefinition {
	definitions := make([]ProviderDefinition, len(providerDefinitions))
	for i, definition := range providerDefinitions {
		definitions[i] = cloneProviderDefinition(definition)
	}
	return definitions
}

func ProviderDefinitionFor(means Means) (ProviderDefinition, bool) {
	for _, definition := range providerDefinitions {
		if definition.Means == means {
			return cloneProviderDefinition(definition), true
		}
	}
	return ProviderDefinition{}, false
}

func plain(key plainKey, label string, kind ProviderFieldKind, required bool) ProviderField {
	return ProviderField{Key: string(key), Label: label, Kind: kind, Required: required}
}

func secret(key secretKey, label string, kind ProviderFieldKind, required bool) ProviderField {
	return ProviderField{Key: string(key), Label: label, Kind: kind, Required: required, Sensitive: true}
}

func withDefault(field ProviderField, value string) ProviderField {
	field.Default = value
	return field
}

func withOptions(field ProviderField, options ...ProviderFieldOption) ProviderField {
	field.Options = options
	return field
}

func described(field ProviderField, description string) ProviderField {
	field.Description = description
	return field
}

func webhookDefinition(means Means, name, label string) ProviderDefinition {
	return ProviderDefinition{
		Means: means, Name: name, Topics: sharedProviderTopics,
		Fields: []ProviderField{secret(keyWebhookURL, label, ProviderFieldURL, true)},
	}
}

func cloneProviderDefinition(definition ProviderDefinition) ProviderDefinition {
	cloned := definition
	cloned.Fields = append([]ProviderField(nil), definition.Fields...)
	for i := range cloned.Fields {
		cloned.Fields[i].Options = append([]ProviderFieldOption(nil), cloned.Fields[i].Options...)
	}
	cloned.Topics = append([]Topic(nil), definition.Topics...)
	return cloned
}
