package diagnostics

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxNameBytes      = 128
	maxMessageBytes   = 4096
	maxAttributeBytes = 2048
	maxAttributes     = 64
	maxAttributeDepth = 4
	redacted          = "[redacted]"
)

func normalize(event Event, received time.Time) Record {
	level := normalizeLevel(event.Level)
	source := normalizeSource(event.Source)
	occurred := event.OccurredAt
	if occurred.IsZero() {
		occurred = received
	}
	attributes, _ := json.Marshal(sanitizeAttributes(event.Attributes))
	if len(attributes) == 0 {
		attributes = []byte("{}")
	}
	record := Record{
		ID: newID(received), OccurredAt: occurred.UnixMilli(), ReceivedAt: received.UnixMilli(),
		Level: level, Source: source,
		Subsystem: truncate(strings.TrimSpace(event.Subsystem), maxNameBytes),
		Event:     truncate(strings.TrimSpace(event.Name), maxNameBytes),
		Message:   sanitizeString(event.Message, maxMessageBytes),
		RequestID: identifier(event.RequestID), PlaybackSessionID: identifier(event.PlaybackSessionID),
		ChannelID: identifier(event.ChannelID), ScheduleBlockID: identifier(event.ScheduleBlockID),
		JobID: identifier(event.JobID), ProcessRunID: identifier(event.ProcessRunID),
		ActorID: identifier(event.ActorID), InstanceID: identifier(event.InstanceID),
		AttributesJSON: string(attributes),
	}
	if record.Event == "" {
		record.Event = "diagnostic.unknown"
	}
	record.SizeBytes = retainedSize(record)
	return record
}

func normalizeLevel(level Level) Level {
	switch level {
	case LevelDebug, LevelInfo, LevelWarn, LevelError:
		return level
	default:
		return LevelInfo
	}
}

func normalizeSource(source Source) Source {
	switch source {
	case SourceServer, SourceWeb, SourceAndroidTV:
		return source
	default:
		return SourceServer
	}
}

func identifier(value string) string {
	return truncate(strings.TrimSpace(value), maxNameBytes)
}

func sanitizeAttributes(attributes map[string]any) map[string]any {
	if len(attributes) == 0 {
		return map[string]any{}
	}
	keys := make([]string, 0, len(attributes))
	for key := range attributes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) > maxAttributes {
		keys = keys[:maxAttributes]
	}
	out := make(map[string]any, len(keys))
	for _, key := range keys {
		cleanKey := truncate(strings.TrimSpace(key), maxNameBytes)
		if cleanKey == "" {
			continue
		}
		if sensitiveKey(cleanKey) {
			out[cleanKey] = redacted
			continue
		}
		out[cleanKey] = sanitizeValue(cleanKey, attributes[key], 0)
	}
	return out
}

func sanitizeValue(key string, value any, depth int) any {
	if depth >= maxAttributeDepth {
		return "[depth-limited]"
	}
	switch typed := value.(type) {
	case nil, bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64,
		float32, float64:
		return typed
	case string:
		if pathKey(key) && filepath.IsAbs(typed) {
			return "[path]"
		}
		return sanitizeString(typed, maxAttributeBytes)
	case error:
		return sanitizeString(typed.Error(), maxAttributeBytes)
	case time.Time:
		return typed.UTC().Format(time.RFC3339Nano)
	case time.Duration:
		return typed.String()
	case map[string]any:
		out := make(map[string]any, min(len(typed), maxAttributes))
		keys := make([]string, 0, len(typed))
		for nestedKey := range typed {
			keys = append(keys, nestedKey)
		}
		sort.Strings(keys)
		if len(keys) > maxAttributes {
			keys = keys[:maxAttributes]
		}
		for _, nestedKey := range keys {
			cleanKey := truncate(strings.TrimSpace(nestedKey), maxNameBytes)
			if cleanKey == "" {
				continue
			}
			if sensitiveKey(cleanKey) {
				out[cleanKey] = redacted
			} else {
				out[cleanKey] = sanitizeValue(cleanKey, typed[nestedKey], depth+1)
			}
		}
		return out
	case []string:
		limit := min(len(typed), maxAttributes)
		out := make([]any, 0, limit)
		for _, item := range typed[:limit] {
			out = append(out, sanitizeString(item, maxAttributeBytes))
		}
		return out
	case []any:
		limit := min(len(typed), maxAttributes)
		out := make([]any, 0, limit)
		for _, item := range typed[:limit] {
			out = append(out, sanitizeValue(key, item, depth+1))
		}
		return out
	default:
		// Do not marshal arbitrary objects: a struct may contain a secret field the caller did
		// not realize was part of its diagnostic interface.
		return truncate(fmt.Sprintf("[%T]", value), maxAttributeBytes)
	}
}

func sanitizeString(value string, limit int) string {
	value = strings.TrimSpace(value)
	if parsed, err := url.Parse(value); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		parsed.User = nil
		query := parsed.Query()
		for key := range query {
			if sensitiveKey(key) {
				query.Set(key, redacted)
			}
		}
		parsed.RawQuery = query.Encode()
		value = parsed.String()
	}
	return truncate(value, limit)
}

func sensitiveKey(key string) bool {
	normalized := strings.NewReplacer("-", "_", ".", "_", " ", "_").Replace(strings.ToLower(key))
	switch normalized {
	case "authorization", "proxy_authorization", "cookie", "set_cookie", "password", "passwd",
		"secret", "token", "api_key", "apikey", "access_token", "refresh_token", "session_token",
		"playout_token", "library_token", "signature", "sig", "x_amz_signature", "x_amz_credential",
		"x_amz_security_token":
		return true
	}
	return strings.HasSuffix(normalized, "_password") || strings.HasSuffix(normalized, "_secret") ||
		strings.HasSuffix(normalized, "_api_key")
}

func pathKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	return normalized == "path" || strings.HasSuffix(normalized, "_path") || strings.HasSuffix(normalized, ".path")
}

func truncate(value string, limit int) string {
	value = strings.ToValidUTF8(value, "�")
	if len(value) <= limit {
		return value
	}
	const suffix = "…[truncated]"
	if limit <= len(suffix) {
		return validPrefix(value, limit)
	}
	return validPrefix(value, limit-len(suffix)) + suffix
}

func validPrefix(value string, limit int) string {
	value = value[:limit]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}

func retainedSize(record Record) int64 {
	return int64(len(record.ID) + len(record.Level) + len(record.Source) + len(record.Subsystem) +
		len(record.Event) + len(record.Message) + len(record.RequestID) + len(record.PlaybackSessionID) +
		len(record.ChannelID) + len(record.ScheduleBlockID) + len(record.JobID) + len(record.ProcessRunID) +
		len(record.ActorID) + len(record.InstanceID) + len(record.AttributesJSON) + 24)
}
