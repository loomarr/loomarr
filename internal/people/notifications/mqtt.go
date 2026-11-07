package notifications

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/eclipse/paho.mqtt.golang/packets"
)

const (
	mqttDeliveryTimeout   = 15 * time.Second
	mqttDisconnectQuiesce = 250 // milliseconds
)

// MQTTAdapter publishes one MQTT 3.1.1 message per claimed notification attempt over a
// short-lived paho client. It owns only QoS 0 and QoS 1: those are the two choices exposed by the
// provider definition, and QoS 1 gives the worker an explicit broker-ownership boundary (PUBACK).
type MQTTAdapter struct {
	publicURL func() string
}

func NewMQTTAdapter(publicURL func() string) *MQTTAdapter {
	return &MQTTAdapter{publicURL: publicURL}
}

func (*MQTTAdapter) Means() Means { return MeansMQTT }

func (*MQTTAdapter) ValidateDestination(settings ProviderSettings) error {
	_, err := mqttProviderConfig(settings)
	return err
}

func (a *MQTTAdapter) Deliver(ctx context.Context, delivery Delivery) Result {
	if a == nil || delivery.Destination == nil || delivery.Destination.Means != MeansMQTT {
		return providerConfigurationFailure()
	}
	config, err := mqttProviderConfig(delivery.Destination.ProviderSettings())
	if err != nil {
		return providerConfigurationFailure()
	}
	message := providerMessage(delivery.Intent, a.publicURL)
	body, err := json.Marshal(providerEventPayload(message))
	if err != nil || len(body) > providerBodyLimit {
		return providerConfigurationFailure()
	}
	topic := strings.TrimRight(config.baseTopic, "/") + "/" + string(message.EventType)
	if len(topic) > 65535 {
		return providerConfigurationFailure()
	}

	deliveryCtx, cancel := context.WithTimeout(ctx, mqttDeliveryTimeout)
	defer cancel()
	client := mqtt.NewClient(mqttClientOptions(deliveryCtx, config, delivery.Destination.ID))
	connect := client.Connect()
	if err := mqttWait(deliveryCtx, connect); err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return Result{Status: StatusFailed, FailureClass: FailureCancelled, OutcomeCode: OutcomeCancelled}
		}
		return mqttConnectFailure(connect)
	}
	defer client.Disconnect(mqttDisconnectQuiesce)

	publish := client.Publish(topic, config.qos, config.retain, body)
	if err := mqttWait(deliveryCtx, publish); err != nil {
		return Result{Status: StatusFailed, FailureClass: FailureAmbiguous, OutcomeCode: OutcomeAcceptanceAmbiguous}
	}
	providerID := "mqtt-qos0"
	if token, ok := publish.(*mqtt.PublishToken); ok && config.qos == 1 {
		providerID = fmt.Sprintf("mqtt-%d", token.MessageID())
	}
	return Result{Status: StatusDelivered, ProviderMessageID: providerID}
}

// mqttClientOptions configures one clean MQTT 3.1.1 session with no reconnect, retry, or
// persistence: the notification worker owns retries, so the client never redelivers on its own.
func mqttClientOptions(ctx context.Context, config mqttConfig, destinationID string) *mqtt.ClientOptions {
	clientID := config.clientID
	if clientID == "" {
		clientID = mqttClientID(destinationID)
	}
	options := mqtt.NewClientOptions().
		AddBroker(config.target.String()).
		SetClientID(clientID).
		SetProtocolVersion(4).
		SetCleanSession(true).
		SetKeepAlive(30 * time.Second).
		SetConnectTimeout(mqttDeliveryTimeout).
		SetWriteTimeout(mqttDeliveryTimeout).
		SetAutoReconnect(false).
		SetConnectRetry(false).
		SetCustomOpenConnectionFn(func(*url.URL, mqtt.ClientOptions) (net.Conn, error) {
			return dialMQTT(ctx, config)
		})
	if config.username != "" {
		options.SetUsername(config.username)
	}
	if config.password != "" {
		options.SetPassword(config.password)
	}
	return options
}

func mqttWait(ctx context.Context, token mqtt.Token) error {
	select {
	case <-token.Done():
		return token.Error()
	case <-ctx.Done():
		return ctx.Err()
	}
}

// mqttConnectFailure classifies a refused or failed CONNECT. A broker that rejects the
// credentials is a permanent recipient rejection; an unreachable broker is transient.
func mqttConnectFailure(token mqtt.Token) Result {
	connect, ok := token.(*mqtt.ConnectToken)
	if !ok {
		return providerTransientFailure()
	}
	switch connect.ReturnCode() {
	case packets.ErrRefusedBadUsernameOrPassword, packets.ErrRefusedNotAuthorised:
		return Result{Status: StatusFailed, FailureClass: FailurePermanent, OutcomeCode: OutcomeRecipientRejected}
	case packets.ErrRefusedBadProtocolVersion, packets.ErrRefusedIDRejected, packets.ErrRefusedServerUnavailable:
		return providerConfigurationFailure()
	default:
		return providerTransientFailure()
	}
}

type mqttConfig struct {
	target    *url.URL
	username  string
	password  string
	baseTopic string
	qos       byte
	retain    bool
	clientID  string
	tlsConfig *tls.Config
}

func mqttProviderConfig(settings ProviderSettings) (mqttConfig, error) {
	rawURL := strings.TrimSpace(settings.secret(keyBrokerURL))
	target, err := url.Parse(rawURL)
	if err != nil || target.Hostname() == "" || (target.Scheme != "mqtt" && target.Scheme != "mqtts") ||
		target.User != nil || (target.Path != "" && target.Path != "/") || target.RawQuery != "" || target.Fragment != "" {
		return mqttConfig{}, fmt.Errorf("MQTT requires an mqtt:// or mqtts:// broker URL without credentials or a path")
	}
	if target.Port() != "" {
		port, portErr := strconv.Atoi(target.Port())
		if portErr != nil || port < 1 || port > 65535 {
			return mqttConfig{}, fmt.Errorf("MQTT broker port must be from 1 through 65535")
		}
	}
	baseTopic := strings.Trim(strings.TrimSpace(settings.plain(keyBaseTopic)), "/")
	if baseTopic == "" || strings.ContainsAny(baseTopic, "+#\x00") || len(baseTopic) > 512 {
		return mqttConfig{}, fmt.Errorf("MQTT base topic must be non-empty and cannot contain wildcards")
	}
	qosValue := settings.plain(keyQoS)
	if qosValue == "" {
		qosValue = "0"
	}
	if qosValue != "0" && qosValue != "1" {
		return mqttConfig{}, fmt.Errorf("MQTT QoS must be 0 or 1")
	}
	retainValue := settings.plain(keyRetain)
	if retainValue == "" {
		retainValue = "false"
	}
	retain, err := strconv.ParseBool(retainValue)
	if err != nil {
		return mqttConfig{}, fmt.Errorf("MQTT retain must be true or false")
	}
	username, password := settings.plain(keyUsername), settings.secret(keyPassword)
	if len(username) > 65535 || len(password) > 65535 {
		return mqttConfig{}, fmt.Errorf("MQTT credentials are too long")
	}
	if username == "" && password != "" {
		return mqttConfig{}, fmt.Errorf("an MQTT password requires a username")
	}
	clientID := strings.TrimSpace(settings.plain(keyClientID))
	if clientID != "" && !validMQTTString(clientID) {
		return mqttConfig{}, fmt.Errorf("MQTT client ID must be valid UTF-8 without null bytes and at most 65535 bytes")
	}
	tlsConfig, err := mqttTLSConfig(target, settings)
	if err != nil {
		return mqttConfig{}, err
	}
	return mqttConfig{
		target: target, username: username, password: password,
		baseTopic: baseTopic, qos: qosValue[0] - '0', retain: retain, clientID: clientID, tlsConfig: tlsConfig,
	}, nil
}

func mqttTLSConfig(target *url.URL, settings ProviderSettings) (*tls.Config, error) {
	caPEM := strings.TrimSpace(settings.secret(keyTLSCACertificate))
	certificatePEM := strings.TrimSpace(settings.secret(keyTLSClientCertificate))
	keyPEM := strings.TrimSpace(settings.secret(keyTLSClientKey))
	if target.Scheme != "mqtts" {
		if caPEM != "" || certificatePEM != "" || keyPEM != "" {
			return nil, fmt.Errorf("MQTT TLS certificates require an mqtts:// broker URL")
		}
		return nil, nil
	}
	if (certificatePEM == "") != (keyPEM == "") {
		return nil, fmt.Errorf("MQTT TLS client certificate and key must be configured together")
	}
	config := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: target.Hostname()}
	if caPEM != "" {
		roots, err := x509.SystemCertPool()
		if err != nil || roots == nil {
			roots = x509.NewCertPool()
		}
		if !roots.AppendCertsFromPEM([]byte(caPEM)) {
			return nil, fmt.Errorf("MQTT TLS CA certificate is not valid PEM")
		}
		config.RootCAs = roots
	}
	if certificatePEM != "" {
		certificate, err := tls.X509KeyPair([]byte(certificatePEM), []byte(keyPEM))
		if err != nil {
			return nil, fmt.Errorf("MQTT TLS client certificate or key is invalid: %w", err)
		}
		config.Certificates = []tls.Certificate{certificate}
	}
	return config, nil
}

func dialMQTT(ctx context.Context, config mqttConfig) (net.Conn, error) {
	port := config.target.Port()
	if port == "" {
		if config.target.Scheme == "mqtts" {
			port = "8883"
		} else {
			port = "1883"
		}
	}
	address := net.JoinHostPort(config.target.Hostname(), port)
	dialer := &net.Dialer{Timeout: mqttDeliveryTimeout, KeepAlive: 30 * time.Second}
	if config.target.Scheme == "mqtts" {
		tlsDialer := &tls.Dialer{NetDialer: dialer, Config: config.tlsConfig.Clone()}
		return tlsDialer.DialContext(ctx, "tcp", address)
	}
	return dialer.DialContext(ctx, "tcp", address)
}

func validMQTTString(value string) bool {
	return value != "" && len(value) <= 65535 && utf8.ValidString(value) && !strings.ContainsRune(value, '\x00')
}

func mqttClientID(destinationID string) string {
	digest := sha256.Sum256([]byte(destinationID))
	return fmt.Sprintf("loomarr-%x", digest[:6])
}
