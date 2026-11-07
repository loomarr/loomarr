package settings

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/adhocore/gronx"
)

// cronParser validates KindCron values (§18.1). gronx is stateless + goroutine-safe for
// IsValid, so one shared instance is fine.
var cronParser = gronx.New()

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

// minSecretLen is the shortest a stored secret may be (config-design §9 sanity
// guard). Deliberately LOW: this guard also runs on the resolve/read path (a stored
// value is re-parsed and self-heals to default if it no longer parses — resolve.go),
// so too high a floor would retroactively invalidate a real short key already in a
// user's store on upgrade. 4 catches a 1–3 char fragment while passing any plausible
// key; the whitespace check is what actually catches the error-string corruption.
const minSecretLen = 4

// parseKind does the Kind-specific string→typed conversion, with no Validate
// hook. Plain strings and enums pass through verbatim; url normalizes; a secret
// gets a shape sanity-check (§9) so garbage like a stray error string can't be
// stored as a token.
func (s Setting) parseKind(raw string) (any, error) {
	switch s.Kind {
	case KindString, KindEnum:
		return raw, nil
	case KindSecret:
		return parseSecret(s.Key, raw)
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
	case KindCron:
		c := strings.TrimSpace(raw)
		if !cronParser.IsValid(c) {
			return nil, fmt.Errorf("%s: %q is not a valid cron expression (e.g. 0 */5 * * * *)", s.Key, raw)
		}
		return c, nil
	case KindStringList:
		return parseList(raw), nil
	default:
		return nil, fmt.Errorf("%s: unknown kind %q", s.Key, s.Kind)
	}
}

// parseSecret is the KindSecret sanity guard (config-design §9). Secrets have no
// universal format, so this checks SHAPE, not content: trim surrounding whitespace
// (a paste artifact), then reject internal whitespace or a too-short value. It never
// echoes the value in the error — the message names the problem only (§4). An empty
// secret is left to the caller's replace-only rule (an empty PATCH is `invalid`
// upstream), so "" passes through here rather than tripping the length floor.
func parseSecret(key, raw string) (string, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return "", nil // replace-only is enforced in Patch; not this guard's job
	}
	if strings.ContainsAny(v, " \t\r\n") {
		return "", fmt.Errorf("%s: a token or key can't contain spaces — check for a stray paste", key)
	}
	if len(v) < minSecretLen {
		return "", fmt.Errorf("%s: that looks too short for a real token or key", key)
	}
	return v, nil
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

// ValueString renders a resolved typed value back to its canonical string form
// for transport (the API/docs). It mirrors parse: durations as "168h", lists
// comma-joined, ints/bools plainly. A nil value (unset) → empty string.
func ValueString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case int:
		return strconv.Itoa(t)
	case bool:
		return strconv.FormatBool(t)
	case time.Duration:
		return t.String()
	case []string:
		return strings.Join(t, ",")
	default:
		return fmt.Sprintf("%v", t)
	}
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
