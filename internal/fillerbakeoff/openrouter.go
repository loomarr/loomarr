package fillerbakeoff

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strings"
)

const (
	OpenRouterBaseURL         = "https://openrouter.ai/api/v1"
	maxOpenRouterOutputTokens = 512
)

func loopbackHost(host string) bool {
	return host == "localhost" || (net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback())
}

func decodeProviderJSON(data []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return fmt.Errorf("trailing JSON value")
		}
		return err
	}
	return nil
}

func boundedMessage(data []byte) string {
	message := strings.TrimSpace(string(data))
	if len(message) > 512 {
		return message[:512]
	}
	return message
}
