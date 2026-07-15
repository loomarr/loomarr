package settings

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// parse turns a raw string (from env or the DB) into the typed value for a
// setting's Kind, then runs the setting's Validate hook (config-design §2, §3).
// It is the single parse path for both tiers, so an env value and a db value of
// the same key are validated identically — the asymmetric handling (§3) is about
// what the CALLER does on error (env → boot fail, db → self-heal), not about
// parsing differently.
func (s Setting) parse(raw string) (any, error) {
	v, err := s.parseKind(raw)
	if err != nil {
		return nil, err
	}
	if s.Kind == KindEnum {
		if err := s.validateEnum(v); err != nil {
			return nil, err
		}
	}
	if s.Validate != nil {
		if err := s.Validate(v); err != nil {
			return nil, err
		}
	}
	return v, nil
}

// parseKind does the Kind-specific string→typed conversion, with no Validate
// hook. Secrets and plain strings pass through verbatim; url normalizes.
func (s Setting) parseKind(raw string) (any, error) {
	switch s.Kind {
	case KindString, KindSecret, KindEnum:
		return raw, nil
	case KindInt:
		n, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil {
			return nil, fmt.Errorf("%s: %q is not an integer", s.Key, raw)
		}
		return n, nil
	case KindBool:
		b, err := strconv.ParseBool(strings.TrimSpace(raw))
		if err != nil {
			return nil, fmt.Errorf("%s: %q is not a boolean", s.Key, raw)
		}
		return b, nil
	case KindDuration:
		d, err := time.ParseDuration(strings.TrimSpace(raw))
		if err != nil {
			return nil, fmt.Errorf("%s: %q is not a duration (e.g. 168h, 30m)", s.Key, raw)
		}
		return d, nil
	case KindURL:
		return normalizeURL(s.Key, raw)
	case KindStringList:
		return parseList(raw), nil
	default:
		return nil, fmt.Errorf("%s: unknown kind %q", s.Key, s.Kind)
	}
}

// normalizeURL enforces config-design §9: a scheme is required, and the trailing
// slash is stripped so http://emby:8096/ and http://emby:8096 are ONE value.
// Returns the normalized string (Kind stays "url" → stored as text).
func normalizeURL(key, raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil // empty is "unset"; required-ness is a separate concern (feature gating)
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("%s: %q is not a valid URL", key, raw)
	}
	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("%s: %q needs a scheme and host (e.g. http://emby:8096)", key, raw)
	}
	u.Path = strings.TrimRight(u.Path, "/")
	return u.String(), nil
}

// parseList splits a comma-separated string_list into trimmed, non-empty items.
func parseList(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
