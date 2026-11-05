package notifications

// plainKey names a provider field the definition stores as ordinary configuration.
type plainKey string

// secretKey names a provider field the definition stores as a sealed credential.
type secretKey string

// Provider field keys. The key's type is its storage class, so the definition table and every
// adapter read agree on where a value lives; a secret can never be read from configuration.
const (
	keyHost        plainKey = "host"
	keyPort        plainKey = "port"
	keySecurity    plainKey = "security"
	keyFromAddress plainKey = "fromAddress"
	keyFromName    plainKey = "fromName"
	keyUsername    plainKey = "username"
	keyBaseURL     plainKey = "baseUrl"
	keyServerURL   plainKey = "serverUrl"
	keyHomeserver  plainKey = "homeserverUrl"
	keyClientID    plainKey = "clientId"
	keyBaseTopic   plainKey = "baseTopic"
	keyQoS         plainKey = "qos"
	keyRetain      plainKey = "retain"

	keyPassword             secretKey = "password"
	keyURL                  secretKey = "url"
	keyBearerToken          secretKey = "bearerToken"
	keyHMACSecret           secretKey = "hmacSecret"
	keyWebhookURL           secretKey = "webhookUrl"
	keyTopic                secretKey = "topic"
	keyApplicationToken     secretKey = "applicationToken"
	keyConfigurationKey     secretKey = "configurationKey"
	keyDestinationURL       secretKey = "destinationUrl"
	keyToken                secretKey = "token"
	keyRecipientKey         secretKey = "recipientKey"
	keyDevice               secretKey = "device"
	keyBotToken             secretKey = "botToken"
	keyChatID               secretKey = "chatId"
	keyThreadID             secretKey = "threadId"
	keyRoomID               secretKey = "roomId"
	keyAccessToken          secretKey = "accessToken"
	keyEndpoint             secretKey = "endpoint"
	keyP256dh               secretKey = "p256dh"
	keyAuth                 secretKey = "auth"
	keyBrokerURL            secretKey = "brokerUrl"
	keyTLSCACertificate     secretKey = "tlsCaCertificate"
	keyTLSClientCertificate secretKey = "tlsClientCertificate"
	keyTLSClientKey         secretKey = "tlsClientKey"
)

// ProviderSettings is one destination's provider fields as the definition classified them.
// Adapters and validators read it only through typed keys.
type ProviderSettings struct {
	configuration map[string]string
	credentials   map[string]string
}

func NewProviderSettings(configuration, credentials map[string]string) ProviderSettings {
	return ProviderSettings{configuration: configuration, credentials: credentials}
}

// ProviderSettings returns the destination's classified provider fields.
func (d Destination) ProviderSettings() ProviderSettings {
	return NewProviderSettings(d.Configuration, d.Credentials)
}

func (s ProviderSettings) plain(key plainKey) string { return s.configuration[string(key)] }

func (s ProviderSettings) secret(key secretKey) string { return s.credentials[string(key)] }

// field returns the value of one definition field from the bucket its sensitivity names.
func (s ProviderSettings) field(field ProviderField) string {
	if field.Sensitive {
		return s.credentials[field.Key]
	}
	return s.configuration[field.Key]
}
