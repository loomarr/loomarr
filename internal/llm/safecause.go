package llm

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// MaxSafeCauseLen bounds SafeCause so a provider cannot fill a jobs row with its own text.
const MaxSafeCauseLen = 300

// secretPattern matches the credential shapes a provider message might echo back:
// bearer tokens, "sk-..." style keys, and key=value pairs.
var secretPattern = regexp.MustCompile(`(?i)(bearer\s+\S+|\bsk-[A-Za-z0-9_\-]+|\b(?:api[_-]?key|token|secret)["'\s:=]+[^\s"',;]+)`)

// SafeCause renders an LLM call failure as a short, persistable explanation: the provider's
// HTTP status and its own error message, or the fired timeout budget. It is what may be
// stored in jobs.last_error and shown to operators, so it never carries prompt or response
// content (a non-JSON or unrecognised error body is dropped, not excerpted), never a
// credential, and is capped at MaxSafeCauseLen.
func SafeCause(err error) string {
	if err == nil {
		return ""
	}
	var statusErr *StatusError
	if errors.As(err, &statusErr) {
		text := fmt.Sprintf("%s: status %d", statusErr.Op, statusErr.Code)
		if msg := providerMessage(statusErr.Body); msg != "" {
			text += ": " + msg
		}
		return clean(text)
	}
	return clean(err.Error())
}

// providerMessage extracts the error message from an OpenAI-style {"error":{"message":…}} body
// (or {"message":…}); anything else yields "" so unstructured bodies cannot leak content.
func providerMessage(body string) string {
	var parsed struct {
		Error   json.RawMessage `json:"error"`
		Message string          `json:"message"`
	}
	if json.Unmarshal([]byte(body), &parsed) != nil {
		return ""
	}
	if len(parsed.Error) > 0 {
		var nested struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(parsed.Error, &nested) == nil && nested.Message != "" {
			return nested.Message
		}
		var flat string
		if json.Unmarshal(parsed.Error, &flat) == nil {
			return flat
		}
	}
	return parsed.Message
}

func clean(s string) string {
	s = secretPattern.ReplaceAllString(s, "[redacted]")
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > MaxSafeCauseLen {
		s = strings.ToValidUTF8(s[:MaxSafeCauseLen], "") + "…"
	}
	return s
}
