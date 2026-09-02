package notifications_test

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/notifications"
)

func TestMQTTAdapterPublishesVersionedEventAndWaitsForQoSOneAck(t *testing.T) {
	t.Parallel()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()

	type captured struct {
		connect []byte
		header  byte
		publish []byte
	}
	got := make(chan captured, 1)
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer func() { _ = connection.Close() }()
		reader := bufio.NewReader(connection)
		_, connect, readErr := readMQTTPacket(reader)
		if readErr != nil {
			return
		}
		_, _ = connection.Write([]byte{0x20, 0x02, 0x00, 0x00})
		header, publish, readErr := readMQTTPacket(reader)
		if readErr != nil {
			return
		}
		topicLength := int(binary.BigEndian.Uint16(publish[:2]))
		packetIDAt := 2 + topicLength
		packetID := publish[packetIDAt : packetIDAt+2]
		_, _ = connection.Write([]byte{0x40, 0x02, packetID[0], packetID[1]})
		got <- captured{connect: connect, header: header, publish: publish}
	}()

	adapter := notifications.NewMQTTAdapter(func() string { return "https://loomarr.example.test" })
	destination := providerDestination(notifications.MeansMQTT, map[string]string{
		"clientId": "living-room-loomarr", "username": "loomarr",
		"baseTopic": "home/loomarr", "qos": "1", "retain": "true",
	}, map[string]string{
		"brokerUrl": "mqtt://" + listener.Addr().String(), "password": "broker-secret",
	})
	result := adapter.Deliver(t.Context(), providerDelivery(&destination))
	if result.Status != notifications.StatusDelivered {
		t.Fatalf("result = %+v", result)
	}
	select {
	case packet := <-got:
		if packet.header != 0x33 { // PUBLISH, QoS 1, retain, DUP false.
			t.Fatalf("PUBLISH header = %#x", packet.header)
		}
		if !strings.Contains(string(packet.connect), "MQTT") ||
			!strings.Contains(string(packet.connect), "living-room-loomarr") ||
			!strings.Contains(string(packet.connect), "loomarr") ||
			!strings.Contains(string(packet.connect), "broker-secret") {
			t.Fatalf("CONNECT packet is incomplete: %q", packet.connect)
		}
		topicLength := int(binary.BigEndian.Uint16(packet.publish[:2]))
		topic := string(packet.publish[2 : 2+topicLength])
		if topic != "home/loomarr/channel_degraded" {
			t.Fatalf("topic = %q", topic)
		}
		var event map[string]any
		if err := json.Unmarshal(packet.publish[2+topicLength+2:], &event); err != nil {
			t.Fatal(err)
		}
		if event["version"] != float64(1) || event["eventId"] != "intent-provider" ||
			event["eventType"] != "channel_degraded" {
			t.Fatalf("event = %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("broker did not receive MQTT publication")
	}
}

func TestMQTTAdapterConnectsWithVerifiedMutualTLS(t *testing.T) {
	t.Parallel()
	broker := newMQTTTLSBroker(t)
	gotConnect := make(chan []byte, 1)
	go func() {
		connection, acceptErr := broker.listener.Accept()
		if acceptErr != nil {
			return
		}
		defer func() { _ = connection.Close() }()
		reader := bufio.NewReader(connection)
		_, connect, readErr := readMQTTPacket(reader)
		if readErr != nil {
			return
		}
		gotConnect <- connect
		_, _ = connection.Write([]byte{0x20, 0x02, 0x00, 0x00})
		_, _, _ = readMQTTPacket(reader)
	}()

	adapter := notifications.NewMQTTAdapter(nil)
	destination := providerDestination(notifications.MeansMQTT, map[string]string{
		"baseTopic": "loomarr", "qos": "0", "retain": "false",
	}, map[string]string{
		"brokerUrl":            "mqtts://" + broker.listener.Addr().String(),
		"tlsCaCertificate":     broker.caCertificate,
		"tlsClientCertificate": broker.clientCertificate,
		"tlsClientKey":         broker.clientKey,
	})
	result := adapter.Deliver(t.Context(), providerDelivery(&destination))
	if result.Status != notifications.StatusDelivered {
		t.Fatalf("result = %+v", result)
	}
	select {
	case packet := <-gotConnect:
		if !strings.Contains(string(packet), "loomarr-") {
			t.Fatalf("CONNECT packet does not contain the derived client ID: %q", packet)
		}
	case <-time.After(time.Second):
		t.Fatal("TLS broker did not receive MQTT CONNECT")
	}
}

func TestMQTTAdapterClassifiesConnectionAndProtocolFailures(t *testing.T) {
	t.Parallel()
	adapter := notifications.NewMQTTAdapter(nil)
	destination := providerDestination(notifications.MeansMQTT, map[string]string{
		"baseTopic": "loomarr", "qos": "0", "retain": "false",
	}, map[string]string{"brokerUrl": "mqtt://127.0.0.1:1"})
	result := adapter.Deliver(t.Context(), providerDelivery(&destination))
	if result.FailureClass != notifications.FailureTransientPreAcceptance ||
		result.OutcomeCode != notifications.OutcomeTransportUnavailable {
		t.Fatalf("connection result = %+v", result)
	}

	validator := notifications.DestinationValidator(adapter)
	for _, brokerURL := range []string{
		"http://broker.example.test", "mqtt://user:pass@broker.example.test", "mqtt://broker.example.test/path",
	} {
		if err := validator.ValidateDestination(map[string]string{
			"baseTopic": "loomarr", "qos": "0", "retain": "false",
		}, map[string]string{"brokerUrl": brokerURL}); err == nil {
			t.Errorf("accepted invalid broker URL %q", brokerURL)
		}
	}
	if err := validator.ValidateDestination(map[string]string{
		"baseTopic": "bad/+/topic", "qos": "2", "retain": "false",
	}, map[string]string{"brokerUrl": "mqtts://broker.example.test"}); err == nil {
		t.Fatal("accepted unsupported QoS and wildcard topic")
	}
	for name, configuration := range map[string]map[string]string{
		"null client ID": {"baseTopic": "loomarr", "clientId": "bad\x00id"},
		"long client ID": {"baseTopic": "loomarr", "clientId": strings.Repeat("x", 65536)},
	} {
		if err := validator.ValidateDestination(configuration, map[string]string{
			"brokerUrl": "mqtt://broker.example.test",
		}); err == nil {
			t.Errorf("accepted %s", name)
		}
	}
	for name, credentials := range map[string]map[string]string{
		"TLS material over plaintext": {
			"brokerUrl": "mqtt://broker.example.test", "tlsCaCertificate": "certificate",
		},
		"unpaired client certificate": {
			"brokerUrl": "mqtts://broker.example.test", "tlsClientCertificate": "certificate",
		},
		"invalid CA PEM": {
			"brokerUrl": "mqtts://broker.example.test", "tlsCaCertificate": "not PEM",
		},
	} {
		if err := validator.ValidateDestination(map[string]string{"baseTopic": "loomarr"}, credentials); err == nil {
			t.Errorf("accepted %s", name)
		}
	}
}

func TestMQTTAdapterClassifiesBrokerAuthenticationRejection(t *testing.T) {
	t.Parallel()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer func() { _ = connection.Close() }()
		_, _, _ = readMQTTPacket(bufio.NewReader(connection))
		_, _ = connection.Write([]byte{0x20, 0x02, 0x00, 0x05}) // not authorized
	}()
	adapter := notifications.NewMQTTAdapter(nil)
	destination := providerDestination(notifications.MeansMQTT, map[string]string{
		"username": "loomarr", "baseTopic": "loomarr", "qos": "1", "retain": "false",
	}, map[string]string{"brokerUrl": "mqtt://" + listener.Addr().String(), "password": "wrong"})
	result := adapter.Deliver(t.Context(), providerDelivery(&destination))
	if result.FailureClass != notifications.FailurePermanent ||
		result.OutcomeCode != notifications.OutcomeRecipientRejected {
		t.Fatalf("result = %+v", result)
	}
}

func readMQTTPacket(reader *bufio.Reader) (byte, []byte, error) {
	header, err := reader.ReadByte()
	if err != nil {
		return 0, nil, err
	}
	remaining := 0
	multiplier := 1
	for {
		encoded, readErr := reader.ReadByte()
		if readErr != nil {
			return 0, nil, readErr
		}
		remaining += int(encoded&0x7f) * multiplier
		if encoded&0x80 == 0 {
			break
		}
		multiplier *= 128
	}
	payload := make([]byte, remaining)
	_, err = io.ReadFull(reader, payload)
	return header, payload, err
}

type mqttTLSBroker struct {
	listener          net.Listener
	caCertificate     string
	clientCertificate string
	clientKey         string
}

func newMQTTTLSBroker(t *testing.T) mqttTLSBroker {
	t.Helper()
	caCertificate, caKeyPEM := issueMQTTCertificate(t, nil, nil, true, false)
	caKeyBlock, _ := pem.Decode(caKeyPEM)
	if caKeyBlock == nil {
		t.Fatal("could not decode test CA key")
	}
	parsedCAKey, err := x509.ParsePKCS8PrivateKey(caKeyBlock.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	caKey, ok := parsedCAKey.(*ecdsa.PrivateKey)
	if !ok {
		t.Fatal("test CA key is not ECDSA")
	}
	serverCertificate, serverKey := issueMQTTCertificate(t, caCertificate, caKey, false, false)
	clientCertificate, clientKey := issueMQTTCertificate(t, caCertificate, caKey, false, true)
	serverPair, err := tls.X509KeyPair(serverCertificate, serverKey)
	if err != nil {
		t.Fatal(err)
	}
	clientRoots := x509.NewCertPool()
	if !clientRoots.AppendCertsFromPEM(caCertificate) {
		t.Fatal("could not load test CA")
	}
	listener, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		MinVersion: tls.VersionTLS12, Certificates: []tls.Certificate{serverPair},
		ClientAuth: tls.RequireAndVerifyClientCert, ClientCAs: clientRoots,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	return mqttTLSBroker{
		listener: listener, caCertificate: string(caCertificate),
		clientCertificate: string(clientCertificate), clientKey: string(clientKey),
	}
}

func issueMQTTCertificate(
	t *testing.T,
	caCertificatePEM []byte,
	caKey *ecdsa.PrivateKey,
	isCA bool,
	isClient bool,
) ([]byte, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serial, Subject: pkix.Name{CommonName: "Loomarr MQTT test"},
		NotBefore: now.Add(-time.Minute), NotAfter: now.Add(time.Hour),
		BasicConstraintsValid: true, IsCA: isCA,
		KeyUsage: x509.KeyUsageDigitalSignature,
	}
	if isCA {
		template.KeyUsage |= x509.KeyUsageCertSign
	} else if isClient {
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	} else {
		template.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
		template.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
	}
	parent := template
	signer := key
	if caCertificatePEM != nil {
		block, _ := pem.Decode(caCertificatePEM)
		if block == nil {
			t.Fatal("could not decode test CA")
		}
		parent, err = x509.ParseCertificate(block.Bytes)
		if err != nil {
			t.Fatal(err)
		}
		signer = caKey
	}
	der, err := x509.CreateCertificate(rand.Reader, template, parent, &key.PublicKey, signer)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
}
