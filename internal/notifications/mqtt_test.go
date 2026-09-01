package notifications_test

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"io"
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
	defer listener.Close()

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
		defer connection.Close()
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
		"username": "loomarr", "baseTopic": "home/loomarr", "qos": "1", "retain": "true",
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
