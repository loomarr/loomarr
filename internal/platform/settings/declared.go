package settings

import (
	"fmt"
	"net/mail"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/platform/httpx"
)

func recipientHTTPOrigin(value any) error {
	raw, ok := value.(string)
	if !ok {
		return fmt.Errorf("want an HTTP or HTTPS origin")
	}
	if raw == "" {
		return nil
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("want an HTTP or HTTPS origin without credentials, path, query, or fragment")
	}
	return nil
}

func smtpPort(value any) error {
	port, ok := value.(int)
	if !ok || port < 1 || port > 65535 {
		return fmt.Errorf("want a port from 1 through 65535")
	}
	return nil
}

func mailboxAddress(value any) error {
	raw, ok := value.(string)
	if !ok {
		return fmt.Errorf("want one mailbox address")
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parsed, err := mail.ParseAddress(raw)
	if err != nil || parsed.Name != "" || parsed.Address != raw {
		return fmt.Errorf("want one address such as loomarr@example.com, without a display name")
	}
	return nil
}

// fillerLanguageOptions is the one server-owned capability catalog used by every language picker.
// Values match the 100-language catalog in the vendored whisper.cpp v1.9.1 detector (whose authority is
// OpenAI Whisper's tokenizer). The matching labels are deliberately code-shaped fallbacks:
// clients localize PresentationLanguage values through CLDR.
func fillerLanguageOptions() []EnumOption {
	lang := func(code string) EnumOption { return opt(code, code) }
	return []EnumOption{
		lang("en"), lang("zh"), lang("de"), lang("es"), lang("ru"), lang("ko"),
		lang("fr"), lang("ja"), lang("pt"), lang("tr"), lang("pl"), lang("ca"),
		lang("nl"), lang("ar"), lang("sv"), lang("it"), lang("id"), lang("hi"),
		lang("fi"), lang("vi"), lang("he"), lang("uk"), lang("el"), lang("ms"),
		lang("cs"), lang("ro"), lang("da"), lang("hu"), lang("ta"), lang("no"),
		lang("th"), lang("ur"), lang("hr"), lang("bg"), lang("lt"), lang("la"),
		lang("mi"), lang("ml"), lang("cy"), lang("sk"), lang("te"), lang("fa"),
		lang("lv"), lang("bn"), lang("sr"), lang("az"), lang("sl"), lang("kn"),
		lang("et"), lang("mk"), lang("br"), lang("eu"), lang("is"), lang("hy"),
		lang("ne"), lang("mn"), lang("bs"), lang("kk"), lang("sq"), lang("sw"),
		lang("gl"), lang("mr"), lang("pa"), lang("si"), lang("km"), lang("sn"),
		lang("yo"), lang("so"), lang("af"), lang("oc"), lang("ka"), lang("be"),
		lang("tg"), lang("sd"), lang("gu"), lang("am"), lang("yi"), lang("lo"),
		lang("uz"), lang("fo"), lang("ht"), lang("ps"), lang("tk"), lang("nn"),
		lang("mt"), lang("sa"), lang("lb"), lang("my"), lang("bo"), lang("tl"),
		lang("mg"), lang("as"), lang("tt"), lang("haw"), lang("ln"), lang("ha"),
		lang("ba"), lang("jw"), lang("su"), lang("yue"),
	}
}

// audioLanguagePresets is playout.audio_language's choices: the file's own first track, then the
// same languages every other picker offers. Presets rather than an enum, so a pinned or stored code
// outside the catalog still saves and still matches its track.
func audioLanguagePresets() []Preset {
	out := []Preset{preset("", "First track in the file")}
	for _, o := range fillerLanguageOptions() {
		out = append(out, preset(o.Value, o.Label))
	}
	return out
}

// autoSplitConfidenceRange bounds `filler.autosplit.min_confidence` to 50–95 (§10 V43).
// The lower bound is load-bearing: an ungrounded era is capped below 50, so no reachable
// threshold can accept a cut on the strength of an invented era.
//
// ⚠ The number is repeated here rather than imported: `settings` must not depend on `filler`
// (the dependency runs the other way — the tagger reads settings). `filler`'s own test pins the
// relationship, so a divergence fails there rather than going unnoticed.
//
// The lower bound is a usability floor: below 50 the threshold admits clips whose tags did not
// fully verify, which makes Incoming an empty room and the catalog a surprise.
func autoSplitConfidenceRange(v any) error {
	n, ok := v.(int)
	if !ok {
		return fmt.Errorf("want a whole number")
	}
	if n < 50 || n > 95 {
		return fmt.Errorf("want 50-95 (got %d) — below 50 accepts cuts whose tags did not verify; above 95 accepts no ordinary grounded proposal", n)
	}
	return nil
}

// positiveLimit rejects a zero or negative cap.
//
// ⚠ Zero is refused rather than treated as "unlimited". A limit key silently meaning its own
// opposite is how an operator sets 0 expecting "no fetching" and gets an uncapped crawler.
// Turning auto-fetch OFF is `filler.fetch.every = 0`, which says what it does.
func positiveLimit(v any) error {
	n, ok := v.(int)
	if !ok {
		return fmt.Errorf("want a whole number")
	}
	if n < 1 {
		return fmt.Errorf("want 1 or more (got %d) — to stop fetching automatically, set filler.fetch.every to 0", n)
	}
	return nil
}

func positiveWholeNumber(v any) error {
	n, ok := v.(int)
	if !ok {
		return fmt.Errorf("want a whole number")
	}
	if n < 1 {
		return fmt.Errorf("want 1 or more (got %d)", n)
	}
	return nil
}

func positiveDuration(v any) error {
	d, ok := v.(time.Duration)
	if !ok {
		return fmt.Errorf("want a duration")
	}
	if d <= 0 {
		return fmt.Errorf("want a duration greater than zero (got %s)", d)
	}
	return nil
}

// jobSchedulePresets are the household cron choices every job schedule (each KindCron setting,
// backup.schedule included) offers in the Tasks
// page's Modify Job dialog (§18.1), most to least frequent. Each is a 6-field seconds-leading
// cron; a schedule matching none of them reads as Custom with its raw expression.
var jobSchedulePresets = []Preset{
	preset("0 * * * * *", "Every minute"),
	preset("0 */5 * * * *", "Every 5 minutes"),
	preset("0 */10 * * * *", "Every 10 minutes"),
	preset("0 */15 * * * *", "Every 15 minutes"),
	preset("0 */30 * * * *", "Every 30 minutes"),
	preset("0 0 * * * *", "Every hour"),
	preset("0 0 */6 * * *", "Every 6 hours"),
	preset("0 0 */12 * * *", "Every 12 hours"),
	preset("0 0 3 * * *", "Daily at 3 am"),
	preset("0 0 0 * * *", "Daily at midnight"),
}

func fillerFetchInterval(v any) error {
	d, ok := v.(time.Duration)
	if !ok {
		return fmt.Errorf("want a duration")
	}
	if d == 0 {
		return nil
	}
	if d < time.Minute || d > 7*24*time.Hour {
		return fmt.Errorf("want Never or a duration from 1 minute through 7 days (got %s)", d)
	}
	return nil
}

func fillerIncomingReadyWindow(v any) error {
	d, ok := v.(time.Duration)
	if !ok {
		return fmt.Errorf("want a duration")
	}
	if d < time.Hour || d > 30*24*time.Hour {
		return fmt.Errorf("want a duration from 1 hour through 30 days (got %s)", d)
	}
	return nil
}

// watermarkOpacityRange bounds `playout.watermark_opacity_pct` to the per-channel
// policy.watermark.opacity range (0.1–1), in percent.
func watermarkOpacityRange(v any) error {
	n, ok := v.(int)
	if !ok {
		return fmt.Errorf("want a whole number")
	}
	if n < 10 || n > 100 {
		return fmt.Errorf("want 10-100 (got %d)", n)
	}
	return nil
}

func nonNegativeWholeNumber(v any) error {
	n, ok := v.(int)
	if !ok {
		return fmt.Errorf("want a whole number")
	}
	if n < 0 {
		return fmt.Errorf("want 0 or more (got %d)", n)
	}
	return nil
}

func breakDuration(v any) error {
	d, ok := v.(time.Duration)
	if !ok {
		return fmt.Errorf("want a duration")
	}
	if d < 30*time.Second {
		return fmt.Errorf("want at least 30s (got %s) — shorter values are clamped by Tunarr and would make playout backends disagree", d)
	}
	return nil
}

// countryCodeList validates filler.region_fallback: every entry an ISO 3166-1 alpha-2 code.
func countryCodeList(v any) error {
	list, ok := v.([]string)
	if !ok {
		return fmt.Errorf("want a list of country codes")
	}
	for _, c := range list {
		if c == "" {
			continue
		}
		if err := optionalCountryCode(c); err != nil {
			return fmt.Errorf("%q: %w", c, err)
		}
	}
	return nil
}

// ianaZone validates guide.timezone: empty (each viewer's device) or a zone name the server can
// load. time.LoadLocation also answers for "Local", the server's own zone, which is no name a
// browser's Intl knows and which would hand every client a zone that differs from the one the
// window grid is laid on, so it is refused beside everything that does not load.
func ianaZone(v any) error {
	name, ok := v.(string)
	if !ok {
		return fmt.Errorf("want an IANA timezone name such as America/New_York")
	}
	if name == "" {
		return nil
	}
	if name == "Local" {
		return fmt.Errorf("%q is the server's own zone, not a timezone name: use an IANA name such as America/New_York, or leave it empty", name)
	}
	if _, err := time.LoadLocation(name); err != nil {
		return fmt.Errorf("%q is not an IANA timezone name such as America/New_York", name)
	}
	return nil
}

// trustedProxyList validates security.trusted_proxies: every entry an address or a CIDR range.
// An entry that isn't is refused, not skipped, so a typo can't leave a proxy silently untrusted.
func trustedProxyList(v any) error {
	list, ok := v.([]string)
	if !ok {
		return fmt.Errorf("want a list of addresses or ranges")
	}
	_, err := httpx.ParseProxyPrefixes(list)
	return err
}

// optionalPlaceID accepts an empty place or a city id from the embedded location index (numeric
// GeoNames ids). Whether the id exists is the location search's business; an unknown id resolves
// to no place, so local filler fails closed.
func optionalPlaceID(v any) error {
	s, ok := v.(string)
	if !ok {
		return fmt.Errorf("want a place id")
	}
	for _, r := range strings.TrimSpace(s) {
		if r < '0' || r > '9' {
			return fmt.Errorf("want a place id from the location search")
		}
	}
	return nil
}

func localRadiusKM(v any) error {
	n, ok := v.(int)
	if !ok {
		return fmt.Errorf("want a whole number of kilometres")
	}
	if n < 10 || n > 400 {
		return fmt.Errorf("want 10-400 km (got %d)", n)
	}
	return nil
}

func optionalCountryCode(v any) error {
	s, ok := v.(string)
	if !ok {
		return fmt.Errorf("want a country code")
	}
	s = strings.ToUpper(strings.TrimSpace(s))
	if s == "" {
		return nil
	}
	if len(s) != 2 || s[0] < 'A' || s[0] > 'Z' || s[1] < 'A' || s[1] > 'Z' {
		return fmt.Errorf("want an ISO 3166-1 alpha-2 country code (for example US or CA)")
	}
	return nil
}

// optionalMetadataLanguage validates metadata.language: empty, an ISO 639-1 code, or one with a
// region ("pt-BR"), as TMDB takes it.
func optionalMetadataLanguage(v any) error {
	s, ok := v.(string)
	if !ok {
		return fmt.Errorf("want a language code")
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	lang, region, hasRegion := strings.Cut(s, "-")
	letters := func(x string, lo, hi byte) bool {
		for i := range len(x) {
			if x[i] < lo || x[i] > hi {
				return false
			}
		}
		return len(x) == 2
	}
	if !letters(lang, 'a', 'z') || (hasRegion && !letters(region, 'A', 'Z')) {
		return fmt.Errorf("want a language code such as en, fr or pt-BR")
	}
	return nil
}

// metadataCountry validates metadata.country: only the US ratings ladder exists (#2151 decision 21).
func metadataCountry(v any) error {
	if err := optionalCountryCode(v); err != nil {
		return err
	}
	if s := strings.ToUpper(strings.TrimSpace(v.(string))); s != "" && s != "US" {
		return fmt.Errorf("only US ratings are supported for now")
	}
	return nil
}

func credentialFreeHTTPSEndpoint(v any) error {
	raw, ok := v.(string)
	if !ok {
		return fmt.Errorf("want an HTTPS endpoint")
	}
	if raw == "" {
		return nil
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil ||
		parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("want an HTTPS endpoint without credentials, query, or fragment")
	}
	return nil
}

// storagePath validates generation-scoped storage roots without importing their owning modules
// back into settings. Absolute paths keep one generation anchored to an unambiguous root.
// Refusing the filesystem root prevents a bad edit from turning a bounded subsystem into a walk
// or output sink over the whole mounted host/container filesystem.
func storagePath(optional bool) ValidateFunc {
	return func(v any) error {
		path, ok := v.(string)
		if !ok {
			return fmt.Errorf("want a filesystem path")
		}
		path = strings.TrimSpace(path)
		if path == "" {
			if optional {
				return nil
			}
			return fmt.Errorf("want a non-empty absolute path")
		}
		clean := filepath.Clean(path)
		if !filepath.IsAbs(clean) {
			return fmt.Errorf("want an absolute path (got %q)", path)
		}
		root := filepath.VolumeName(clean) + string(filepath.Separator)
		if clean == root {
			return fmt.Errorf("the filesystem root cannot be used as a storage directory")
		}
		return nil
	}
}

// declared is the canonical registry content: every app-managed setting, in the
// order it appears in design.md §15. This list IS the contract — design.md §15
// is its human mirror and `make config-docs` its generated reference. A key added
// here without a matching §15 row (or vice versa) is the drift AGENTS.md forbids.
//
// Env-only bootstrap keys (DATABASE_URL, AUTO_MIGRATE, LISTEN_ADDR, LOG_LEVEL, TZ)
// are NOT here — they stay in config.Config (config-design §1 classification).
// Generated tokens (API_TOKEN, PLAYOUT_TOKEN) live in secrets.go
// (minted, not demanded — §4), not the app-managed registry.
//
// The settings sit in one function per group, each in its own file (declared_*.go). The order is
// the registry's, the documentation's and the settings page's, so a group is appended whole and
// never interleaved.
func declared() []Setting {
	settings := slices.Concat(
		declaredConnections(),
		declaredPlayout(),
		declaredAI(),
		declaredChannelsAndFiller(),
		declaredAccess(),
		declaredAdvanced(),
	)
	for i := range settings {
		if !settings[i].MigrationOnly {
			settings[i].Owner = ownerFor(settings[i])
		}
	}
	return settings
}
