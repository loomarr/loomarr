package notifications

import "fmt"

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
			ProviderFieldSelect, ProviderFieldToggle:
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
		if field.Sensitive {
			credentials[key] = value
		} else {
			configuration[key] = value
		}
	}
	return configuration, credentials, nil
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
}

var sharedProviderTopics = []Topic{
	TopicProposalSubmitted,
	TopicAcquisitionAvailable,
	TopicAcquisitionGaveUp,
	TopicChannelLive,
	TopicChannelDegraded,
}

var providerDefinitions = []ProviderDefinition{
	{
		Means: MeansEmail, Name: "SMTP", Topics: productProviderTopics,
		Fields: []ProviderField{
			field("host", "SMTP host", ProviderFieldText, true, false),
			{Key: "port", Label: "Port", Kind: ProviderFieldNumber, Required: true, Default: "587"},
			{Key: "security", Label: "Security", Kind: ProviderFieldSelect, Required: true, Default: "starttls", Options: []ProviderFieldOption{
				{Value: "starttls", Label: "STARTTLS"}, {Value: "tls", Label: "TLS"}, {Value: "none", Label: "None"},
			}},
			field("fromAddress", "From address", ProviderFieldText, true, false),
			field("fromName", "From name", ProviderFieldText, false, false),
			field("username", "Username", ProviderFieldText, false, false),
			field("password", "Password", ProviderFieldPassword, false, true),
		},
	},
	{
		Means: MeansWebhook, Name: "Webhook", Topics: sharedProviderTopics,
		Fields: []ProviderField{
			field("url", "Webhook URL", ProviderFieldURL, true, true),
			field("bearerToken", "Bearer token", ProviderFieldPassword, false, true),
			field("hmacSecret", "HMAC signing secret", ProviderFieldPassword, false, true),
		},
	},
	webhookDefinition(MeansDiscord, "Discord", "Discord webhook URL"),
	{
		Means: MeansNtfy, Name: "ntfy", Topics: sharedProviderTopics,
		Fields: []ProviderField{
			field("baseUrl", "Server URL", ProviderFieldURL, true, false),
			field("topic", "Topic", ProviderFieldPassword, true, true),
			field("username", "Username", ProviderFieldText, false, false),
			field("password", "Password or token", ProviderFieldPassword, false, true),
		},
	},
	{
		Means: MeansGotify, Name: "Gotify", Topics: sharedProviderTopics,
		Fields: []ProviderField{
			field("serverUrl", "Server URL", ProviderFieldURL, true, false),
			field("applicationToken", "Application token", ProviderFieldPassword, true, true),
		},
	},
	{
		Means: MeansApprise, Name: "Apprise API", Topics: sharedProviderTopics,
		Fields: []ProviderField{
			field("baseUrl", "Apprise API URL", ProviderFieldURL, true, false),
			field("configurationKey", "Configuration key", ProviderFieldPassword, false, true),
			field("destinationUrl", "Stateless destination URL", ProviderFieldPassword, false, true),
			field("token", "Authentication token", ProviderFieldPassword, false, true),
		},
	},
	{
		Means: MeansPushover, Name: "Pushover", Topics: productProviderTopics, MemberOwned: true,
		Fields: []ProviderField{
			field("applicationToken", "Application token", ProviderFieldPassword, true, true),
			field("recipientKey", "User or group key", ProviderFieldPassword, true, true),
			field("device", "Device", ProviderFieldPassword, false, true),
		},
	},
	{
		Means: MeansTelegram, Name: "Telegram Bot", Topics: sharedProviderTopics,
		Fields: []ProviderField{
			field("botToken", "Bot token", ProviderFieldPassword, true, true),
			field("chatId", "Chat ID", ProviderFieldPassword, true, true),
			field("threadId", "Topic or thread ID", ProviderFieldPassword, false, true),
		},
	},
	webhookDefinition(MeansMattermost, "Mattermost", "Incoming webhook URL"),
	{
		Means: MeansMatrix, Name: "Matrix", Topics: sharedProviderTopics,
		Fields: []ProviderField{
			field("homeserverUrl", "Homeserver URL", ProviderFieldURL, true, false),
			field("roomId", "Room ID", ProviderFieldPassword, true, true),
			field("accessToken", "Access token", ProviderFieldPassword, true, true),
		},
	},
	{Means: MeansWebPush, Name: "Browser Push", Topics: productProviderTopics, MemberOwned: true},
	{
		Means: MeansMQTT, Name: "MQTT", Topics: sharedProviderTopics,
		Fields: []ProviderField{
			field("brokerUrl", "Broker URL", ProviderFieldPassword, true, true),
			field("username", "Username", ProviderFieldText, false, false),
			field("password", "Password", ProviderFieldPassword, false, true),
			field("baseTopic", "Base topic", ProviderFieldText, true, false),
			{Key: "qos", Label: "QoS", Kind: ProviderFieldSelect, Default: "0", Options: []ProviderFieldOption{
				{Value: "0", Label: "At most once"}, {Value: "1", Label: "At least once"},
			}},
			{Key: "retain", Label: "Retain messages", Kind: ProviderFieldToggle, Default: "false"},
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

func field(key, label string, kind ProviderFieldKind, required, sensitive bool) ProviderField {
	return ProviderField{Key: key, Label: label, Kind: kind, Required: required, Sensitive: sensitive}
}

func webhookDefinition(means Means, name, label string) ProviderDefinition {
	return ProviderDefinition{
		Means: means, Name: name, Topics: sharedProviderTopics,
		Fields: []ProviderField{field("webhookUrl", label, ProviderFieldURL, true, true)},
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
