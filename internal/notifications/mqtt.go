package notifications

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const mqttDeliveryTimeout = 15 * time.Second

// MQTTAdapter publishes one MQTT 3.1.1 message per claimed notification attempt. It intentionally
// owns only QoS 0 and QoS 1: those are the two choices exposed by the provider definition, and QoS
// 1 gives the worker an explicit broker-ownership boundary through PUBACK.
type MQTTAdapter struct {
	publicURL func() string
}

func NewMQTTAdapter(publicURL func() string) *MQTTAdapter {
	return &MQTTAdapter{publicURL: publicURL}
}

func (*MQTTAdapter) Means() Means { return MeansMQTT }

func (*MQTTAdapter) ValidateDestination(configuration, credentials map[string]string) error {
	_, err := mqttProviderConfig(configuration, credentials)
	return err
}

func (a *MQTTAdapter) Deliver(ctx context.Context, delivery Delivery) Result {
	if a == nil || delivery.Destination == nil || delivery.Destination.Means != MeansMQTT {
		return providerConfigurationFailure()
	}
	config, err := mqttProviderConfig(delivery.Destination.Configuration, delivery.Destination.Credentials)
	if err != nil {
		return providerConfigurationFailure()
	}
	message := providerMessage(delivery.Intent, a.publicURL)
	body, err := json.Marshal(providerEventPayload(message))
	if err != nil || len(body) > providerBodyLimit {
		return providerConfigurationFailure()
	}
	topic := strings.TrimRight(config.baseTopic, "/") + "/" + string(message.EventType)
	packetID := mqttPacketID(delivery.Intent.ID)

	deliveryCtx, cancel := context.WithTimeout(ctx, mqttDeliveryTimeout)
	defer cancel()
	connection, err := dialMQTT(deliveryCtx, config)
	if err != nil {
		if deliveryCtx.Err() == context.Canceled || ctx.Err() == context.Canceled {
			return Result{Status: StatusFailed, FailureClass: FailureCancelled, OutcomeCode: OutcomeCancelled}
		}
		return providerTransientFailure()
	}
	defer func() { _ = connection.Close() }()
	if deadline, ok := deliveryCtx.Deadline(); ok {
		_ = connection.SetDeadline(deadline)
	}

	if _, err = connection.Write(mqttConnectPacket(config, delivery.Destination.ID)); err != nil {
		return providerTransientFailure()
	}
	reader := bufio.NewReader(connection)
	if result, accepted := mqttReadConnack(reader); !accepted {
		return result
	}
	publish, err := mqttPublishPacket(topic, body, config.qos, config.retain, packetID)
	if err != nil {
		return providerConfigurationFailure()
	}
	if _, err = connection.Write(publish); err != nil {
		return Result{Status: StatusFailed, FailureClass: FailureAmbiguous, OutcomeCode: OutcomeAcceptanceAmbiguous}
	}
	if config.qos == 1 {
		ack := make([]byte, 4)
		if _, err = io.ReadFull(reader, ack); err != nil {
			return Result{Status: StatusFailed, FailureClass: FailureAmbiguous, OutcomeCode: OutcomeAcceptanceAmbiguous}
		}
		if ack[0] != 0x40 || ack[1] != 0x02 || binary.BigEndian.Uint16(ack[2:]) != packetID {
			return providerConfigurationFailure()
		}
	}
	_, _ = connection.Write([]byte{0xe0, 0x00})
	return Result{Status: StatusDelivered, ProviderMessageID: fmt.Sprintf("mqtt-%d", packetID)}
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

func mqttProviderConfig(configuration, credentials map[string]string) (mqttConfig, error) {
	rawURL := strings.TrimSpace(credentials["brokerUrl"])
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
	baseTopic := strings.Trim(strings.TrimSpace(configuration["baseTopic"]), "/")
	if baseTopic == "" || strings.ContainsAny(baseTopic, "+#\x00") || len(baseTopic) > 512 {
		return mqttConfig{}, fmt.Errorf("MQTT base topic must be non-empty and cannot contain wildcards")
	}
	qosValue := configuration["qos"]
	if qosValue == "" {
		qosValue = "0"
	}
	if qosValue != "0" && qosValue != "1" {
		return mqttConfig{}, fmt.Errorf("MQTT QoS must be 0 or 1")
	}
	retainValue := configuration["retain"]
	if retainValue == "" {
		retainValue = "false"
	}
	retain, err := strconv.ParseBool(retainValue)
	if err != nil {
		return mqttConfig{}, fmt.Errorf("MQTT retain must be true or false")
	}
	if len(configuration["username"]) > 65535 || len(credentials["password"]) > 65535 {
		return mqttConfig{}, fmt.Errorf("MQTT credentials are too long")
	}
	if configuration["username"] == "" && credentials["password"] != "" {
		return mqttConfig{}, fmt.Errorf("an MQTT password requires a username")
	}
	clientID := strings.TrimSpace(configuration["clientId"])
	if clientID != "" && !validMQTTString(clientID) {
		return mqttConfig{}, fmt.Errorf("MQTT client ID must be valid UTF-8 without null bytes and at most 65535 bytes")
	}
	tlsConfig, err := mqttTLSConfig(target, credentials)
	if err != nil {
		return mqttConfig{}, err
	}
	return mqttConfig{
		target: target, username: configuration["username"], password: credentials["password"],
		baseTopic: baseTopic, qos: qosValue[0] - '0', retain: retain, clientID: clientID, tlsConfig: tlsConfig,
	}, nil
}

func mqttTLSConfig(target *url.URL, credentials map[string]string) (*tls.Config, error) {
	caPEM := strings.TrimSpace(credentials["tlsCaCertificate"])
	certificatePEM := strings.TrimSpace(credentials["tlsClientCertificate"])
	keyPEM := strings.TrimSpace(credentials["tlsClientKey"])
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

func mqttConnectPacket(config mqttConfig, destinationID string) []byte {
	flags := byte(0x02) // clean session
	if config.username != "" {
		flags |= 0x80
	}
	if config.password != "" {
		flags |= 0x40
	}
	variable := []byte{0x00, 0x04, 'M', 'Q', 'T', 'T', 0x04, flags, 0x00, 0x1e}
	clientID := config.clientID
	if clientID == "" {
		clientID = mqttClientID(destinationID)
	}
	payload := mqttString(clientID)
	if config.username != "" {
		payload = append(payload, mqttString(config.username)...)
	}
	if config.password != "" {
		payload = append(payload, mqttString(config.password)...)
	}
	return mqttPacket(0x10, append(variable, payload...))
}

func mqttPublishPacket(topic string, body []byte, qos byte, retain bool, packetID uint16) ([]byte, error) {
	if len(topic) > 65535 || len(body) > providerBodyLimit {
		return nil, fmt.Errorf("MQTT publication is too large")
	}
	header := byte(0x30) | qos<<1
	if retain {
		header |= 0x01
	}
	payload := mqttString(topic)
	if qos == 1 {
		identifier := make([]byte, 2)
		binary.BigEndian.PutUint16(identifier, packetID)
		payload = append(payload, identifier...)
	}
	payload = append(payload, body...)
	return mqttPacket(header, payload), nil
}

func mqttReadConnack(reader io.Reader) (Result, bool) {
	packet := make([]byte, 4)
	if _, err := io.ReadFull(reader, packet); err != nil {
		return providerTransientFailure(), false
	}
	if packet[0] != 0x20 || packet[1] != 0x02 || packet[2]&0xfe != 0 {
		return providerConfigurationFailure(), false
	}
	switch packet[3] {
	case 0:
		return Result{}, true
	case 4, 5:
		return Result{Status: StatusFailed, FailureClass: FailurePermanent, OutcomeCode: OutcomeRecipientRejected}, false
	default:
		return providerConfigurationFailure(), false
	}
}

func mqttPacket(header byte, payload []byte) []byte {
	packet := bytes.NewBuffer(make([]byte, 0, len(payload)+5))
	packet.WriteByte(header)
	remaining := len(payload)
	for {
		encoded := byte(remaining % 128)
		remaining /= 128
		if remaining > 0 {
			encoded |= 0x80
		}
		packet.WriteByte(encoded)
		if remaining == 0 {
			break
		}
	}
	packet.Write(payload)
	return packet.Bytes()
}

func mqttString(value string) []byte {
	encoded := make([]byte, 2, len(value)+2)
	binary.BigEndian.PutUint16(encoded, uint16(len(value)))
	return append(encoded, value...)
}

func validMQTTString(value string) bool {
	return value != "" && len(value) <= 65535 && utf8.ValidString(value) && !strings.ContainsRune(value, '\x00')
}

func mqttPacketID(intentID string) uint16 {
	digest := sha256.Sum256([]byte(intentID))
	value := binary.BigEndian.Uint16(digest[:2])
	if value == 0 {
		return 1
	}
	return value
}

func mqttClientID(destinationID string) string {
	digest := sha256.Sum256([]byte(destinationID))
	return fmt.Sprintf("loomarr-%x", digest[:6])
}
