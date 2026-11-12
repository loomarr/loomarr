package settings

import (
	"fmt"
	"net/mail"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/platform/buildinfo"
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

// declared is the canonical registry content: every app-managed setting, in the
// order it appears in design.md §15. This list IS the contract — design.md §15
// is its human mirror and `make config-docs` its generated reference. A key added
// here without a matching §15 row (or vice versa) is the drift AGENTS.md forbids.
//
// Env-only bootstrap keys (DATABASE_URL, AUTO_MIGRATE, LISTEN_ADDR, LOG_LEVEL, TZ)
// are NOT here — they stay in config.Config (config-design §1 classification).
// Generated tokens (API_TOKEN, PLAYOUT_TOKEN) live in secrets.go
// (minted, not demanded — §4), not the app-managed registry.

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

func declared() []Setting {
	settings := []Setting{
		// --- Connections: media server (§15, Phase 5) ---
		{
			Key: "library.flavor", EnvVar: "LIBRARY_FLAVOR", Group: GroupMediaServer,
			Kind: KindEnum, Enum: []EnumOption{opt("emby", "Emby"), opt("jellyfin", "Jellyfin")}, Default: "",
			Doc:    "Which media server you run, Emby or Jellyfin.",
			Detail: "They sign in differently, so Loomarr needs to know which one you run.",
		},
		{
			Key: "library.url", EnvVar: "LIBRARY_URL", Group: GroupMediaServer,
			Kind: KindURL, Default: "",
			Doc:    "The address of your media server.",
			Detail: "For example http://emby:8096.",
		},
		{
			Key: "library.token", EnvVar: "LIBRARY_TOKEN", Group: GroupMediaServer,
			Kind: KindSecret, Default: "",
			Doc:    "An API key from your media server.",
			Detail: "It lets Loomarr read your library and set up the TV guide.",
		},
		{
			// Translates the media server's folder paths to where Loomarr reads the same mount, so
			// folder discovery can offer them as library folders (#2151). Playout never uses it: it
			// plays the files Loomarr's own scan found (step 16).
			Key: "library.path_map", EnvVar: "LIBRARY_PATH_MAP", Group: GroupMediaServer,
			Kind: KindString, Default: "", Advanced: true,
			Doc:    "Where your media server's folders are on this machine, so Loomarr can find your media folders.",
			Detail: "Your media server names each folder by its OWN path (e.g. /data/tv); if the same folder is mounted somewhere else on the machine running Loomarr (e.g. /mnt/media/tv), map one to the other as \"/data=>/mnt/media\". Multiple rules are separated by commas or newlines. Leave empty when the paths already match, or add your folders by hand in Settings → Library.",
		},
		{
			// The scanner's kill switch (#2151): passes, folder watches and header reads stop.
			Key: "library.scanner.enabled", Label: "Scan media folders", EnvVar: "LIBRARY_SCANNER_ENABLED",
			Group: GroupMediaServer, Kind: KindBool, Default: true, Advanced: true,
			Doc:    "Whether Loomarr reads your media folders itself to learn what each file is.",
			Detail: "Turn this off to stop it reading them; what it already learned is kept.",
		},
		// --- Connections: requester (§15, Phase 6) ---
		// How Loomarr acquires missing titles: through Seerr (default), or Sonarr + Radarr
		// directly. The provider gates which fields show (ShowWhen), mirroring llm.provider.
		{
			Key: "requester.provider", EnvVar: "REQUESTER_PROVIDER", Group: GroupRequester,
			Kind: KindEnum, Enum: []EnumOption{opt("seerr", "Seerr / Jellyseerr"), opt("arr", "Sonarr + Radarr (direct)")}, Default: "seerr",
			Doc: "How Loomarr downloads missing titles: through Seerr, or Sonarr and Radarr directly.",
		},
		{
			Key: "seerr.url", EnvVar: "SEERR_URL", Group: GroupRequester,
			Kind: KindURL, Default: "", Required: FeatureAcquisition,
			Doc:      "Your Seerr address, which is how Loomarr downloads missing titles.",
			Detail:   "For example http://seerr:5055.",
			ShowWhen: map[string][]string{"requester.provider": {"seerr"}},
		},
		{
			Key: "seerr.api_key", EnvVar: "SEERR_API_KEY", Group: GroupRequester,
			Kind: KindSecret, Default: "",
			Doc:      "Your Seerr API key.",
			ShowWhen: map[string][]string{"requester.provider": {"seerr"}},
		},
		// Where Sonarr and Radarr live is in their own connections below; these overrides only
		// shape what the direct requester asks them for.
		{
			Key: "sonarr.quality_profile", Label: "Sonarr quality profile override", EnvVar: "SONARR_QUALITY_PROFILE", Group: GroupRequester,
			Kind: KindString, Default: "", Advanced: true,
			Doc:      "Which Sonarr quality profile new shows use, or blank for Sonarr's first.",
			Detail:   "Optional Sonarr quality profile (name or id). Blank = Sonarr's first profile.",
			ShowWhen: map[string][]string{"requester.provider": {"arr"}},
		},
		{
			Key: "sonarr.root_folder", Label: "Sonarr root folder override", EnvVar: "SONARR_ROOT_FOLDER", Group: GroupRequester,
			Kind: KindString, Default: "", Advanced: true,
			Doc:      "Which Sonarr folder new shows go in, or blank for Sonarr's first.",
			Detail:   "Optional Sonarr root folder path. Blank = Sonarr's first root folder.",
			ShowWhen: map[string][]string{"requester.provider": {"arr"}},
		},
		{
			Key: "radarr.quality_profile", Label: "Radarr quality profile override", EnvVar: "RADARR_QUALITY_PROFILE", Group: GroupRequester,
			Kind: KindString, Default: "", Advanced: true,
			Doc:      "Which Radarr quality profile new movies use, or blank for Radarr's first.",
			Detail:   "Optional Radarr quality profile (name or id). Blank = Radarr's first profile.",
			ShowWhen: map[string][]string{"requester.provider": {"arr"}},
		},
		{
			Key: "radarr.root_folder", Label: "Radarr root folder override", EnvVar: "RADARR_ROOT_FOLDER", Group: GroupRequester,
			Kind: KindString, Default: "", Advanced: true,
			Doc:      "Which Radarr folder new movies go in, or blank for Radarr's first.",
			Detail:   "Optional Radarr root folder path. Blank = Radarr's first root folder.",
			ShowWhen: map[string][]string{"requester.provider": {"arr"}},
		},

		// --- Connections: Sonarr and Radarr (#2151 decisions 2 and 29) ---
		// Their own connections, whichever requester is chosen: the library reads their folders
		// and what each file is, and they tell Loomarr about new downloads.
		{
			Key: "sonarr.url", EnvVar: "SONARR_URL", Group: GroupSonarr,
			Kind: KindURL, Default: "",
			Doc:    "Your Sonarr address.",
			Detail: "For example http://sonarr:8989. Loomarr learns your shows from it and hears about new episodes.",
		},
		{
			Key: "sonarr.api_key", EnvVar: "SONARR_API_KEY", Group: GroupSonarr,
			Kind: KindSecret, Default: "",
			Doc: "Your Sonarr API key (Settings → General in Sonarr).",
		},
		{
			Key: "radarr.url", EnvVar: "RADARR_URL", Group: GroupRadarr,
			Kind: KindURL, Default: "",
			Doc:    "Your Radarr address.",
			Detail: "For example http://radarr:7878. Loomarr learns your movies from it and hears about new ones.",
		},
		{
			Key: "radarr.api_key", EnvVar: "RADARR_API_KEY", Group: GroupRadarr,
			Kind: KindSecret, Default: "",
			Doc: "Your Radarr API key (Settings → General in Radarr).",
		},

		// --- Connections: Tunarr (§15, Phase 10) ---
		{
			Key: "tunarr.url", EnvVar: "TUNARR_URL", Group: GroupTunarr,
			Kind: KindURL, Default: "",
			Doc:    "Your Tunarr address, where Loomarr builds your channels.",
			Detail: "For example http://tunarr:8000.",
		},
		{
			Key: "tunarr.transcode_config_id", Label: "Transcode profile override", EnvVar: "TUNARR_TRANSCODE_CONFIG_ID", Group: GroupTunarr,
			Kind: KindString, Default: "", Advanced: true,
			Doc:    "Which Tunarr transcode profile new channels use.",
			Detail: "Leave empty to use Tunarr's default.",
		},
		{
			// RE-SCOPED by V4 (§9.1), not duplicated. This was a Tunarr-group, Advanced
			// knob documented as "only needed for uploaded channel icons". Internal
			// playout makes it the base every SEGMENT request resolves against, so it
			// moves to Playout and stops being Advanced — "get it wrong and channels
			// appear in the guide but never play" is not an advanced failure mode.
			//
			// Deliberately ONE key, not a second `playout.public_url`: it is genuinely
			// the server's own public address, and both callers (icon fetch, segment
			// fetch) need the same value. Two keys could drift, and an operator would
			// have to know which one Live TV reads.
			Key: "server.public_url", Label: "Loomarr address", EnvVar: "SERVER_PUBLIC_URL", Group: GroupPlayout,
			Kind: KindURL, Default: "",
			Doc:    "The address your media server and Tunarr use to reach Loomarr's channel streams.",
			Detail: "Loomarr's own address as your media server and Tunarr can reach it, e.g. http://loomarr:8080. Your media server and Tunarr fetch every stream URL from this base, so a wrong value means channels appear in the guide there and never play. Loomarr's own in-app playback does not use it (it reaches itself on its listen address), and Current Health flags an address this server cannot reach. Also used for uploaded channel icons.",
		},

		{
			Key: "access.public_url", Label: "Recipient-facing Loomarr address", EnvVar: "ACCESS_PUBLIC_URL", Group: GroupGeneral,
			Kind: KindURL, Default: "", Validate: recipientHTTPOrigin,
			Doc:    "The address people open from an invitation or sign-in recovery link.",
			Detail: "The absolute browser address invitation and recovery recipients can reach, e.g. https://loomarr.example.com. General suggests the current browser origin when this is empty; save a different address when recipients reach Loomarr elsewhere. The server never infers it from request headers.",
		},
		{
			Key: "notifications.email.enabled", Label: "Send email notifications", EnvVar: "NOTIFICATIONS_EMAIL_ENABLED", Group: GroupNotifications,
			Kind: KindBool, Presentation: PresentationSwitch, Default: false, MigrationOnly: true,
			Doc:    "Send account invitations and recovery messages by email.",
			Detail: "An incomplete setup suppresses email without affecting copied links, QR codes, or direct account creation.",
		},
		{
			Key: "notifications.smtp.host", Label: "SMTP host", EnvVar: "NOTIFICATIONS_SMTP_HOST", Group: GroupNotifications,
			Kind: KindString, Default: "", MigrationOnly: true, ShowWhen: map[string][]string{"notifications.email.enabled": {"true"}},
			Doc:    "The mail server Loomarr sends email through.",
			Detail: "Hostname of the SMTP submission server. Required when email delivery is enabled.",
		},
		{
			Key: "notifications.smtp.port", Label: "SMTP port", EnvVar: "NOTIFICATIONS_SMTP_PORT", Group: GroupNotifications,
			Kind: KindInt, Default: 587, MigrationOnly: true, Validate: smtpPort, ShowWhen: map[string][]string{"notifications.email.enabled": {"true"}},
			Doc: "Port of the SMTP submission server, from 1 through 65535.",
		},
		{
			Key: "notifications.smtp.security", Label: "SMTP security", EnvVar: "NOTIFICATIONS_SMTP_SECURITY", Group: GroupNotifications,
			Kind: KindEnum, Enum: []EnumOption{
				opt("starttls", "STARTTLS (required)"),
				opt("tls", "TLS"),
				opt("none", "None (insecure local relay)"),
			},
			Default: "starttls", MigrationOnly: true, ShowWhen: map[string][]string{"notifications.email.enabled": {"true"}},
			Doc:    "How the connection to the mail server is encrypted.",
			Detail: "STARTTLS requires encryption and never downgrades; TLS connects encrypted immediately. None is only for an explicitly trusted local relay. Certificate verification is always enabled.",
		},
		{
			Key: "notifications.smtp.username", Label: "SMTP username", EnvVar: "NOTIFICATIONS_SMTP_USERNAME", Group: GroupNotifications,
			Kind: KindString, Default: "", MigrationOnly: true, ShowWhen: map[string][]string{"notifications.email.enabled": {"true"}},
			Doc:    "The username for signing in to the mail server.",
			Detail: "Username for SMTP authentication. Leave empty only for an unauthenticated relay.",
		},
		{
			Key: "notifications.smtp.password", Label: "SMTP password", EnvVar: "NOTIFICATIONS_SMTP_PASSWORD", Group: GroupNotifications,
			Kind: KindSecret, Default: "", MigrationOnly: true, ShowWhen: map[string][]string{"notifications.email.enabled": {"true"}},
			Doc:    "The password for signing in to the mail server.",
			Detail: "Password for SMTP authentication. It is write-only, masked on read, and must remain empty for an unauthenticated relay.",
		},
		{
			Key: "notifications.email.from_address", Label: "Sender address", EnvVar: "NOTIFICATIONS_EMAIL_FROM_ADDRESS", Group: GroupNotifications,
			Kind: KindString, Default: "", MigrationOnly: true, Validate: mailboxAddress, ShowWhen: map[string][]string{"notifications.email.enabled": {"true"}},
			Doc:    "The email address Loomarr sends from.",
			Detail: "Mailbox Loomarr sends from, such as loomarr@example.com. Required when email delivery is enabled.",
		},
		{
			Key: "notifications.email.from_name", Label: "Sender name", EnvVar: "NOTIFICATIONS_EMAIL_FROM_NAME", Group: GroupNotifications,
			Kind: KindString, Default: "Loomarr", MigrationOnly: true, ShowWhen: map[string][]string{"notifications.email.enabled": {"true"}},
			Doc: "Display name shown beside the sender address.",
		},

		// --- Playout (§9.1, §15 — added by V4) ---
		// Loomarr serves its own channels. `playout.backend` is the ONLY key here with a
		// per-channel override: it rides `policy_json` as `policy.playout` (nil = inherit
		// this global), the same shape `rules`/`filler`/`window`/`autoCurate` already use,
		// so there is no migration. Inheriting channels resolve this value live, so changing
		// the default moves that fleet; a channel with an explicit policy remains pinned to
		// its selected backend.
		{
			Key: "playout.backend", Label: "Playback engine", EnvVar: "PLAYOUT_BACKEND", Group: GroupPlayout,
			Kind: KindEnum, Enum: []EnumOption{
				opt("internal", "Loomarr (internal)"),
				opt("tunarr", "Tunarr"),
			},
			Default: "internal",
			Doc:     "Whether Loomarr streams your channels itself or hands them to Tunarr.",
			Detail:  "Who streams a channel. Internal playout is the default: it supports mid-roll breaks, reports real transcode telemetry, and needs no extra service. Tunarr is an optional backend for an install that already runs Tunarr — not a requirement. An unrecognised value falls back to internal. Overridable per channel.",
		},
		{
			Key: "playout.encoder", Label: "Encoder override", EnvVar: "PLAYOUT_ENCODER", Group: GroupPlayout,
			Kind: KindString, Default: "", Advanced: true,
			Doc:    "Forces a particular video encoder instead of the one Loomarr picked.",
			Detail: "ffmpeg encoder for internal playout (e.g. libx264, h264_vaapi, h264_nvenc). Empty = pick the best one the transcode check found. Set it only to override that choice.",
		},
		{
			Key: "playout.audio_language", Label: "Preferred audio language", EnvVar: "PLAYOUT_AUDIO_LANGUAGE", Group: GroupPlayout,
			Kind: KindString, Presentation: PresentationLanguage, Presets: audioLanguagePresets(), Default: "en",
			Doc:    "Which audio language channels play when a film has more than one.",
			Detail: "Preferred audio language for internal playout. Any ISO 639 code saves (en, eng and fra all work). A preference, not a requirement: a film with no track in this language plays its first track rather than failing. Empty = play whichever track comes first in the file, which is how a foreign-language dub ends up playing instead of the original. A channel can override this on its Channel info page.",
		},
		{
			Key: "playout.quality_tier", Label: "Maximum playback quality", EnvVar: "PLAYOUT_QUALITY_TIER", Group: GroupPlayout,
			Kind: KindEnum, Enum: []EnumOption{
				opt("efficient", "Efficient — 720p, lowest bandwidth"),
				opt("balanced", "Balanced — 1080p"),
				opt("quality", "Quality — 1080p, best picture"),
			},
			Default: "balanced",
			Doc:     "The best picture quality channels aim for, traded against bandwidth.",
			Detail:  "The picture-versus-bandwidth target. Efficient is 720p and roughly half the bitrate — the right answer for a NAS running several channels, or for watching away from home. Balanced is 1080p and the default. Quality is 1080p at a higher frame rate and bitrate, which on grainy or dark film can be visibly cleaner but costs noticeably more bandwidth per channel. Whichever you pick, playout still steps down automatically as more channels start, so the choice is a ceiling rather than a promise.",
		},
		{
			// One curve on every family (G11, #1512); playout.ToneCurve maps it to each tone-mapper.
			// Read once when a stream starts and pinned for its life, so a change never alters the
			// look mid-stream. Costs are the household Arc's, per HDR stream at 1080p (#1516).
			Key: "playout.tone_curve", Label: "HDR tone curve", EnvVar: "PLAYOUT_TONE_CURVE", Group: GroupPlayout,
			Kind: KindEnum, Enum: []EnumOption{
				opt("hable", "Hable (filmic) — default"),
				opt("mobius", "Mobius"),
				opt("reinhard", "Reinhard"),
				opt("bt2390", "BT.2390 — more CPU on Intel"),
				opt("bt2446a", "BT.2446 Method A — more CPU on Intel"),
				opt("spline", "Spline — more CPU on Intel"),
			},
			Default: "hable",
			Doc:     "How HDR films are adjusted to look right on an ordinary screen.",
			Detail: "How an HDR film is turned into the SDR picture a channel broadcasts. Hable (the default) rolls bright highlights off gently, like film; Mobius and Reinhard keep more of the mid-tones and highlights. These three run on the GPU on Intel and NVIDIA, about 0.1 CPU cores per HDR stream on Intel. " +
				"BT.2390, BT.2446 Method A and Spline come from libplacebo. On Intel the picture makes a trip through system memory for them, which roughly doubles the CPU cost to about 0.25 cores per HDR stream. Where libplacebo cannot run (no Vulkan GPU, for example an NVIDIA container without NVIDIA's Vulkan driver), those three use the CPU tone-mapper's Mobius curve instead. " +
				"Without a GPU tone-mapper at all, every curve runs on the CPU after the GPU has scaled the picture down; the picture is correct but costs more CPU. A change applies to streams that start afterwards; a channel already playing keeps its curve until it restarts.",
		},
		{
			// The middle tier of channel policy.watermark.opacity > this > built-in (#1617); the
			// maintainer chose 40% from a real 1080p preview. Read per programme item, and the
			// rendered-bug cache is keyed by opacity, so a change re-renders the bugs.
			Key: "playout.watermark_opacity_pct", Label: "On-screen logo opacity", EnvVar: "PLAYOUT_WATERMARK_OPACITY_PCT", Group: GroupPlayout,
			Kind: KindInt, Unit: UnitPercent, Default: "40", Validate: watermarkOpacityRange,
			Doc:    "How solid the on-screen logo in the corner of every programme looks, from 10 (faint) to 100 (fully opaque).",
			Detail: "A channel with its own opacity keeps it. A change shows from each channel's next programme.",
		},
		{
			// The maintainer's four styles (#1617, picked from real-frame previews), Text by default,
			// then the approved design's six (#1659); the middle tier of channel
			// policy.watermark.look > this > built-in. Values match schedule.WatermarkLooks and
			// watermark.DesignStyles (pinned by an internal/app test). Read per programme item; the
			// rendered-bug cache is keyed by look, so a change re-renders.
			Key: "playout.watermark_look", Label: "On-screen logo style", EnvVar: "PLAYOUT_WATERMARK_LOOK", Group: GroupPlayout,
			Kind: KindEnum, Enum: []EnumOption{
				opt("plate", "Plate — the name cut out of a rounded badge"),
				opt("text", "Text — the name alone, with a soft shadow"),
				opt("outline", "Outline — the name as hollow letters"),
				opt("small-plate", "Small plate — a smaller badge with no shadow"),
				opt("tint", "Colour plate — the name cut out of a coloured badge"),
				opt("twotone", "Number + name — the channel number in a coloured tab beside the name"),
				opt("badge", "Number badge — the channel number in a coloured rounded square"),
				opt("lockup", "Number | name — number, thin line, name, in white"),
				opt("glass", "Glass — the name on a frosted see-through plate"),
				opt("stripe", "Era stripe — the name over a three-colour stripe"),
			},
			Default: "text",
			Doc:     "How the on-screen logo Loomarr draws from a channel's name looks.",
			Detail:  "Coloured styles use each channel's colour (by default, colours from the channel's decade). A channel with its own style keeps it, and an uploaded logo keeps its own shape. A change shows from each channel's next programme.",
		},
		{
			// ⚠ Separate from ingest.ffmpeg_path because these two callers fail differently.
			// Playout's ffmpeg is a runtime dependency of a channel that is ON AIR, while
			// ingest's is a build dependency of a download nobody is watching — so an operator pointing ingest at
			// a newer yt-dlp-compatible ffmpeg must not be able to break playout by doing it.
			Key: "playout.ffmpeg_path", Label: "FFmpeg path", EnvVar: "PLAYOUT_FFMPEG_PATH", Group: GroupPlayout,
			Kind: KindString, Presentation: PresentationPath, Default: "ffmpeg", Advanced: true,
			Doc:    "Where the ffmpeg program lives.",
			Detail: "The default works whenever ffmpeg is on the system PATH; set it only if yours is somewhere unusual.",
		},
		{
			// Where the in-app HLS player's segments are written (§9.1 Watch, V46). Disk-backed
			// beside the database by default, NOT the OS temp dir (#1512 spike requirement 8):
			// the channel packager keeps a DVR window per watched channel (~8 Mbit/s × 15 min,
			// about 1 GB), which a RAM-backed /tmp would hold in memory. Each process removes its
			// own scratch root when it stops and sweeps a crashed predecessor's at start.
			// Advanced: a wrong value degrades in-app playback, never the media-server streams
			// (those never touch this dir).
			Key: "playout.hls_dir", Label: "Browser playback cache", EnvVar: "PLAYOUT_HLS_DIR", Group: GroupPlayout,
			Kind: KindString, Presentation: PresentationPath, DataSubdir: "hls", Advanced: true,
			Doc:    "Where watching in the browser keeps a few minutes of temporary video.",
			Detail: "Directory where in-app browser playback writes its temporary HLS segments. Defaults to a directory beside the database (/data/hls in the container). Each channel being watched keeps up to 15 minutes of video here (about 1 GB), deleted when you stop watching, so use a disk, not a RAM-backed tmpfs like /dev/shm. Only affects in-app playback; your media server's streams never use it. Changing it takes effect after restart.",
		},
		{
			// Host measurements get their own small root: the capability evidence, the per-class cost
			// table and the synthetic probe clips. Read once at composition.
			Key: "playout.state_dir", Label: "Playout host measurements", EnvVar: "PLAYOUT_STATE_DIR", Group: GroupPlayout,
			Kind: KindString, Presentation: PresentationPath, DataSubdir: "playout-state", Advanced: true,
			Doc:    "Where Loomarr keeps what it learned about this server's video power.",
			Detail: "Where Loomarr keeps what it measured about this server's transcoding: the chosen encoder, the cost of each kind of stream and a few seconds of generated test video (about 20 MB). Defaults to a directory beside the database (/data in the container). Deleting it makes Loomarr measure again at the next start. Changing it takes effect after restart.",
		},
		{
			Key: "playout.max_channels", Label: "Live transcode safety cap", EnvVar: "PLAYOUT_MAX_CHANNELS", Group: GroupPlayout,
			Kind: KindInt, Default: "0", Validate: nonNegativeWholeNumber,
			Doc:    "The most channels Loomarr may stream at the same time, or 0 to decide for itself.",
			Detail: "Optional safety cap for simultaneous internal transcodes. Leave at 0 for Loomarr to use measured capacity automatically. A positive value can lower that measurement but cannot raise it.",
		},
		{
			// Host memory is a per-lease gate on hardware encodes, not a capacity term (§9.1): each
			// encoder's device context and pipeline hold ~0.5–1 GiB of host RAM. Hot-applied because
			// the ResourceBudget re-reads it on every admission.
			Key: "playout.memory_reserve_mb", Label: "Host memory reserve for encodes", EnvVar: "PLAYOUT_MEMORY_RESERVE_MB", Group: GroupPlayout,
			Kind: KindInt, Unit: UnitMiB, Default: "2048", Advanced: true, Validate: nonNegativeWholeNumber,
			Doc:    "Memory that channel streaming always leaves free for the rest of the machine.",
			Detail: "Host memory, in MiB, that hardware encoding must leave available. Loomarr does not start a hardware transcode that would take free memory below this. Set 0 to disable the host-memory check. Changes apply to the next encode without restart.",
		},
		{
			Key: "playout.encode_memory_mb", Label: "Host memory per encode", EnvVar: "PLAYOUT_ENCODE_MEMORY_MB", Group: GroupPlayout,
			Kind: KindInt, Unit: UnitMiB, Default: "0", Advanced: true, Validate: nonNegativeWholeNumber,
			Doc:    "How much memory one streaming channel is expected to use, or 0 to work it out.",
			Detail: "Host memory, in MiB, one hardware encode is expected to use. Leave at 0 to use 1024 MiB, or more if the encoder capability check measured more on this machine; set it when real encodes here use more, for example 4K or HDR tone mapping. Changes apply to the next encode without restart.",
		},
		{
			// The ResourceBudget's CPU term (#1512 G5): total playout CPU at full GPU capacity.
			// Hot-applied because the budget re-reads its facts on every admission.
			Key: "playout.gpu_cpu_millicores", Label: "Playout CPU on GPU hosts", EnvVar: "PLAYOUT_GPU_CPU_MILLICORES", Group: GroupPlayout,
			Kind: KindInt, Default: "1000", Advanced: true, Validate: positiveWholeNumber,
			Doc:    "How much processor time all channels together may use when a graphics card does the encoding.",
			Detail: "On a host that encodes on its GPU, the CPU all live channels together may use, in thousandths of a core (1000 = one core). Loomarr admits no more channels than fit this allowance at each channel's measured CPU cost. Never more than the container's CPU limit. Changes apply to the next channel start without restart.",
		},
		{
			Key: "playout.app_reserve_millicores", Label: "CPU kept for the app on software hosts", EnvVar: "PLAYOUT_APP_RESERVE_MILLICORES", Group: GroupPlayout,
			Kind: KindInt, Default: "1000", Advanced: true, Validate: nonNegativeWholeNumber,
			Doc:    "Processor time kept free for the rest of Loomarr when channels are encoded without a graphics card.",
			Detail: "On a host with no hardware encoder, the CPU kept free for the rest of Loomarr, in thousandths of a core (1000 = one core). Live channels may use the container's CPU limit (or the CPU count) minus this reserve. Changes apply to the next channel start without restart.",
		},

		{
			// The guide's display timezone (§12, V13b gap 7). The API always speaks absolute
			// epoch ms — a timezone is a RENDERING choice, and putting it on the wire would
			// invite a client to reinterpret instants it should merely format.
			//
			// Empty = the viewer's own browser timezone, which is right for the household
			// case. An operator sets it when the server and its viewers are elsewhere, or
			// when they want the guide to read in the channels' "broadcast" timezone.
			//
			// It is also the wall clock the rolling-window grid is laid on (#1675), so a daily
			// window turns at midnight in this zone. Empty there = the container's own zone.
			Key: "guide.timezone", Label: "Timezone", EnvVar: "GUIDE_TIMEZONE", Group: GroupPlayout,
			Kind: KindString, Presentation: PresentationTimezone, Presets: timezonePresets(), Default: "",
			Doc:    "The timezone the TV guide shows times in and each channel's day starts in.",
			Detail: "Which timezone the TV guide's times are shown in, as an IANA name like America/New_York, and where each channel's day starts: a daily schedule turns over at midnight in this timezone. Leave empty to show each viewer's own device timezone; the schedule then turns over at midnight in the server's timezone.",
		},
		{
			// How far back the guide will look (§12, V13b gap 8).
			//
			// A real bound, not a cosmetic one: the past is recomputed from the channel's
			// CURRENT lineup, so a distant "as aired" view would be fiction — the lineup has
			// been reconciled since. A day is honest; a month would be invention presented as
			// history.
			Key: "guide.retention_hours", Label: "Past listings", EnvVar: "GUIDE_RETENTION_HOURS", Group: GroupPlayout,
			Kind: KindInt, Unit: UnitHours, Default: "24",
			Doc:    "How far back the TV guide lets you scroll.",
			Detail: "Past listings are recomputed from each channel's current lineup, so going too far back would show a schedule that never actually aired.",
		},

		// --- Backup (§16, §15 — added by V4) ---
		{
			Key: "backup.schedule", Label: "Automatic backup schedule", EnvVar: "BACKUP_SCHEDULE", Group: GroupBackup,
			Kind: KindCron, Presets: jobSchedulePresets, Default: "0 30 3 * * *",
			Doc:    "When Loomarr backs up its settings, channels and people.",
			Detail: "When to write the nightly database backup. It contains settings, channels, people, encrypted secrets, and wrapped data keys, but not the external installation key. It does not contain filler, cached artwork, or operator image uploads.",
		},
		{
			Key: "backup.retain", Label: "Backups to keep", EnvVar: "BACKUP_RETAIN", Group: GroupBackup,
			Kind: KindInt, Default: "7",
			Doc: "How many backups to keep before pruning the oldest.",
		},
		{
			Key: "backup.dir", Label: "Backup location", EnvVar: "BACKUP_DIR", Group: GroupBackup,
			Kind: KindString, Presentation: PresentationPath, DataSubdir: "backups",
			Doc:    "Where backups are written.",
			Detail: "Defaults to a directory beside the database (/data in the container) so its volume carries them; point it elsewhere to keep backups off the same disk as the database.",
		},

		// --- Images (§15, §22, V52) ---
		{
			// ⚠ Defaults inside /data for the same reason `filler.dir` and `backup.dir` do — the
			// documented volume carries it — but with a consequence neither of those has: the
			// application backup is a DATABASE backup, and no image bytes are in the database
			// (§22). Everything here is regenerable or re-fetchable EXCEPT operator uploads, which
			// is why image maintenance counts unrecoverable-missing rows as a warning rather than
			// pretending it can repair them.
			Key: "images.dir", Label: "Image library location", EnvVar: "IMAGES_DIR", Group: GroupImages,
			Kind: KindString, Presentation: PresentationPath, DataSubdir: "images",
			Doc:    "Where Loomarr stores images, both originals and the resized copies it serves.",
			Detail: "Defaults to a directory beside the database (/data in the container) so its volume carries it. Not covered by the database backup: back up the volume.",
		},
		{
			Key: "images.max_upload_bytes", Label: "Maximum image upload", EnvVar: "IMAGES_MAX_UPLOAD_BYTES", Group: GroupImages,
			Kind: KindInt, Presentation: PresentationBytes, Default: "8388608",
			Doc:    "The largest image someone may upload.",
			Detail: "In bytes (8 MiB by default). Enforced while reading the upload, not from the size the client declares.",
		},
		{
			Key: "images.remote_fetch_enabled", Label: "Download remote artwork", EnvVar: "IMAGES_REMOTE_FETCH_ENABLED", Group: GroupImages,
			Kind: KindBool, Default: true,
			Doc:    "Whether Loomarr may download artwork from TMDB and your media server.",
			Detail: "Turn this off to keep to locally-produced images only — no outbound image requests are made.",
		},
		{
			Key: "images.cache_budget_mb", Label: "Resized-image cache", EnvVar: "IMAGES_CACHE_BUDGET_MB", Group: GroupImages,
			Kind: KindInt, Unit: UnitMiB, Default: "2048", Advanced: true,
			Doc:    "How much disk the resized copies of images may use.",
			Detail: "Past this, Loomarr starts removing the least recently used ones. They are always regenerable, so this costs a little latency, never an image.",
		},

		// --- Connections: TMDB (§15, Phase 11) ---
		{
			Key: "tmdb.api_key", EnvVar: "TMDB_API_KEY", Group: GroupTMDB,
			// The default is the key the release build ships with (decision 51); List reports
			// it as builtIn, never as set, and never previews it.
			Kind: KindSecret, Default: buildinfo.TMDBKey(), Required: FeatureSuggestions,
			Doc:    "Your own TMDB API key, which most households can leave empty.",
			Detail: "Your own TMDB API key, if you have one. Loomarr comes with its own, so most households can leave this empty. TMDB powers title search, channel icon suggestions, and grounding for AI channel suggestions.",
		},
		{
			// #2151 decision 21: one installation-wide metadata language and country, defaulting to
			// the media server's.
			Key: "metadata.language", Label: "Title language", EnvVar: "METADATA_LANGUAGE", Group: GroupTMDB,
			Kind: KindString, Default: "", Advanced: true, Validate: optionalMetadataLanguage,
			Doc:    "The language titles, overviews and trailers are shown in.",
			Detail: "As a language code (en, fr, pt-BR). Empty uses your media server's metadata language, or English. Changing it fetches every title's details again.",
		},
		{
			Key: "metadata.country", Label: "Ratings country", EnvVar: "METADATA_COUNTRY", Group: GroupTMDB,
			Kind: KindString, Default: "", Advanced: true, Validate: metadataCountry,
			Doc:    "The country whose content ratings Loomarr uses.",
			Detail: "Only US ratings are supported for now, so this can be empty or US.",
		},

		// --- AI (§15, §8.1; in-app selection persists to llm.* and overrides these env pins) ---
		{
			Key: "llm.provider", Label: "Lineup AI provider", EnvVar: "LLM_PROVIDER", Group: GroupAI,
			Kind: KindEnum, Enum: []EnumOption{opt("ollama", "Ollama"), opt("openai", "OpenAI-compatible")}, Default: "ollama", Required: FeatureSuggestions,
			Doc:    "Which AI to use: a local Ollama, or an OpenAI-compatible service.",
			Detail: "You can also pick a model in the AI settings.",
		},
		{
			// Both providers need an endpoint: the Ollama host for local AI, or the
			// OpenAI-compatible base URL for hosted AI. The default host is conventional, not
			// universal, so hiding this for Ollama made remote Ollama impossible to configure.
			Key: "llm.url", Label: "AI service address", EnvVar: "LLM_URL", Group: GroupAI,
			Kind: KindURL, Default: "",
			Doc:    "The address of your AI service.",
			Detail: "For Ollama, its host such as http://ollama:11434. For a hosted provider, the exact OpenAI-compatible API base; Loomarr fills this for OpenRouter, while Custom remains editable.",
		},
		{
			// The normal AI page uses the ranked picker for both provider kinds so two
			// controls never compete. This declaration remains for env compatibility and
			// the explicit All Settings escape hatch.
			Key: "llm.model", Label: "Lineup model", EnvVar: "LLM_MODEL", Group: GroupAI,
			Kind: KindString, Default: "", Required: FeatureSuggestions,
			Doc:      "The AI model that builds channel lineups.",
			Detail:   "The active model used to build channel lineups. Prefer the guided picker on the AI page; OpenRouter ids use provider/model (for example openai/gpt-4o-mini).",
			ShowWhen: map[string][]string{"llm.provider": {"openai"}},
		},
		{
			// Ollama is local and needs no key — this only applies to a hosted service.
			Key: "llm.api_key", Label: "Hosted AI API key", EnvVar: "LLM_API_KEY", Group: GroupAI,
			Kind: KindSecret, Default: "",
			Doc:      "The API key for your hosted AI service.",
			Detail:   "Never shown again after saving.",
			ShowWhen: map[string][]string{"llm.provider": {"openai"}},
		},
		{
			// Local-only (§8.2): a hosted service has no model to hold in memory, so this
			// is hidden for the openai provider rather than shown as an inert control.
			Key: "llm.keep_alive", Label: "Keep local model loaded", EnvVar: "LLM_KEEP_ALIVE", Group: GroupAI,
			Kind: KindDuration, Default: "2m", Advanced: true,
			Doc:      "How long the local AI model stays loaded between requests.",
			Detail:   "How long to keep the local AI model loaded in memory between requests. Loading it takes several seconds, so keeping it ready makes suggestions much faster — but the model shares GPU memory with channel playback, so the default is short (2m) to free that memory for streaming. Raise it if you rarely stream and want faster suggestions; set 0 to free memory as soon as each request finishes.",
			ShowWhen: map[string][]string{"llm.provider": {"ollama"}},
		},
		{
			Key: "asr.provider", Label: "Speech recognition", EnvVar: "ASR_PROVIDER", Group: GroupAI,
			Kind: KindEnum, Enum: []EnumOption{
				opt("whisper", "Built in on this device"), opt("hosted", "Connected speech service"),
			},
			Default: "whisper",
			Doc:     "How Loomarr understands speech in clips.",
			Detail:  "The built-in option stays on this device; a connected service is faster on low-power hardware and can use a separate address and key under Advanced.",
		},
		{
			Key: "asr.url", Label: "Speech service address", EnvVar: "ASR_URL", Group: GroupAI,
			Kind: KindURL, Default: "", Advanced: true,
			Doc:      "The address of a separate speech service, or blank to use your AI service.",
			Detail:   "OpenAI-compatible speech-to-text API base, such as http://ai-server:8083/v1. Leave blank to reuse the active hosted AI service.",
			ShowWhen: map[string][]string{"asr.provider": {"hosted"}},
		},
		{
			Key: "asr.api_key", Label: "Speech service API key", EnvVar: "ASR_API_KEY", Group: GroupAI,
			Kind: KindSecret, Default: "", Advanced: true,
			Doc:      "The API key for the separate speech service.",
			Detail:   "It is never shown again after saving and the main AI key is never sent to a custom speech address.",
			ShowWhen: map[string][]string{"asr.provider": {"hosted"}},
		},
		{
			Key: "asr.model", Label: "Speech model", EnvVar: "ASR_MODEL", Group: GroupAI,
			Kind: KindString, Default: "openai/whisper-large-v3", Advanced: true,
			Doc:      "The speech model the connected service uses.",
			Detail:   "Speech-to-text model used by the connected service. It is separate from the lineup model because it must return timed transcript segments and the detected language.",
			ShowWhen: map[string][]string{"asr.provider": {"hosted"}},
		},
		{
			Key: "suggest.max_acquisitions", Label: "Pending-download limit per person", EnvVar: "SUGGEST_MAX_ACQUISITIONS", Group: GroupAI,
			Kind: KindInt, Default: 10,
			Doc: "The most titles a single suggestion may download.",
		},

		// --- Self-updating channels / re-curation (programming-design §8.2) ---
		{
			Key: "job.recurate.schedule", EnvVar: "JOB_RECURATE_SCHEDULE", Group: GroupAdvanced,
			Kind: KindCron, Presets: jobSchedulePresets, Default: "0 0 4 * * 0", Advanced: true,
			Doc:    "How often auto-curate channels look again at what your library offers.",
			Detail: "How often auto-curate channels re-evaluate their intent against the library (cron). Weekly by default — this runs the AI, so keep it infrequent.",
		},
		{
			Key: "recurate.min_score_pct", Label: "Auto-curation request threshold", EnvVar: "RECURATE_MIN_SCORE_PCT", Group: GroupAI,
			Kind: KindInt, Unit: UnitPercent, Default: 60, Advanced: true,
			Doc:    "How good a match a title you don't have must be before auto-curate downloads it.",
			Detail: "Quality bar (0–100) a not-in-library title must clear for auto-curate to REQUEST it. In-library matches are added regardless. A per-channel override may be stricter or looser.",
		},
		{
			Key: "recurate.max_titles", Label: "Auto-curation channel limit", EnvVar: "RECURATE_MAX_TITLES", Group: GroupAI,
			Kind: KindInt, Default: 40, Advanced: true,
			Doc:    "The most titles an auto-curate channel may grow to.",
			Detail: "Re-curation won't request net-new titles past this cap. A per-channel override may be stricter or looser.",
		},

		// --- Channels & playback (§15, Phase 10; policy defaults = programming-design §2) ---
		{
			Key: "channel.reconcile_every", Label: "Channel rebuild cooldown", EnvVar: "CHANNEL_RECONCILE_EVERY", Group: GroupChannels,
			Kind: KindDuration, Default: "10m", Advanced: true,
			Doc:    "How long a channel rests after a rebuild before it can be rebuilt again.",
			Detail: "Minimum delay after a successful rebuild before that channel is eligible for another scheduled sweep. Change the sweep cadence under System → Tasks.",
		},
		{
			Key: "sched.window_hours", Label: "Schedule ahead", EnvVar: "SCHED_WINDOW_HOURS", Group: GroupChannels,
			Kind: KindDuration, Default: "24h",
			Doc:    "How far ahead each channel plans its schedule.",
			Detail: "How far ahead each channel schedules — the rolling window it materializes and rolls forward, instead of the whole series run (per-channel/-rule overridable; 0 = schedule everything).",
		},

		// --- Filler / commercials (§15, Phase 12; §10 redesign — Tunarr-owned) ---
		{
			Key: "filler.home_country", Label: "Country", EnvVar: "FILLER_HOME_COUNTRY", Group: GroupFiller,
			Kind: KindString, Default: "", Validate: optionalCountryCode,
			Doc:    "The country your channels are watched in, for holidays and filler.",
			Detail: "Where your channels are watched. Loomarr uses this country for channels and filler sources, and keeps its holidays on its dates (Canada's Thanksgiving in October, Britain's Mothering Sunday in spring). Empty, or a country Loomarr has no holidays for yet, keeps the US calendar.",
		},
		{
			// #2062: a picked place (an embedded location index id), never a typed name — a name
			// could not be measured against a broadcast station's community of license.
			Key: "filler.home_place", Label: "Home city", EnvVar: "FILLER_HOME_PLACE", Group: GroupFiller,
			Kind: KindString, Default: "", Validate: optionalPlaceID,
			Doc:    "Your city, so local station news and weather can air between programmes.",
			Detail: "Optional. Pick your city to get local TV material, such as station news and weather, from stations near you. Without it, only national filler airs.",
		},
		{
			Key: "filler.local_radius_km", Label: "Local station radius", EnvVar: "FILLER_LOCAL_RADIUS_KM", Group: GroupFiller,
			Kind: KindInt, Default: "100", Validate: localRadiusKM,
			Doc: "How far from your city a TV station can be and still count as local, in kilometres.",
		},
		{
			Key: "filler.region_fallback", Label: "Other countries", EnvVar: "FILLER_REGION_FALLBACK", Group: GroupFiller,
			Kind: KindStringList, Default: "", Validate: countryCodeList,
			Doc:    "Other countries whose commercials may fill in when your own run short.",
			Detail: "Optional, in order of preference: countries whose national commercials may air and whose sources may download after your own country's, such as GB, CA, AU. Your country's commercials always come first within an era; these fill in when its supply for that era runs thin.",
		},
		{
			// ⚠ Defaults inside /data, like database.url and backup.dir — the documented
			// volume carries it, so a zero-env `docker run` has a working drop-folder
			// instead of a Filler page that is one empty state telling the operator to go
			// and configure something. It was "" for no recorded reason while its two
			// neighbours both defaulted; that asymmetry made the whole feature opt-in by
			// accident. Still overridable to point at an existing library on another disk.
			Key: "filler.dir", Label: "Clip library", EnvVar: "FILLER_DIR", Group: GroupFiller,
			Kind: KindString, Presentation: PresentationPath, Apply: ApplyRestart, DataSubdir: "filler", Required: FeatureFiller, Validate: storagePath(false),
			Doc:    "Where Loomarr stores clips.",
			Detail: "Each is filed under its content hash with its metadata beside it. Defaults to a directory beside the database (/data in the container) so its volume carries it; point it elsewhere to use an existing clip library.",
		},
		{
			// The watch folder (§10 V38c, "Two folders, one pipeline"). Clips ARRIVE here —
			// downloads land here, operators drop files here — and every sync drains it into the
			// clip folder above.
			//
			// ⚠ **The default is EMPTY, resolved to `<filler.dir>/_watch` by the reader**, not a
			// literal `/data/filler/_watch`. A literal would silently stop tracking the moment an
			// operator pointed `filler.dir` at an existing library on another disk: arrivals would
			// keep landing under `/data` while the catalog looked elsewhere, and the drop-folder
			// would appear broken with both settings looking correct.
			//
			// ⚠ **Inside the clip folder rather than a sibling.** A sibling needs its own mounted
			// volume, and an unmounted watch folder loses anything not yet filed on the next
			// restart — silently, because an empty folder is also what success looks like. The
			// scan skips `_watch` by name so a waiting file is never catalogued from its arrival
			// path (which would then be pruned the moment intake moved it).
			Key: "filler.watch_dir", Label: "Drop folder", EnvVar: "FILLER_WATCH_DIR", Group: GroupFiller,
			Kind: KindString, Presentation: PresentationPath, Apply: ApplyRestart, Default: "", Validate: storagePath(true),
			Doc:    "A folder where any clip you drop in is added to your library.",
			Detail: "Folder Loomarr watches for new clips. Anything dropped here is filed into your clip folder and then removed. Leave blank to use a '_watch' folder inside the clip folder.",
		},
		{
			Key: "filler.sync_every", Label: "Scan for dropped clips every", EnvVar: "FILLER_SYNC_EVERY", Group: GroupFiller,
			Kind: KindDuration, Default: "15m", Advanced: true,
			Doc: "How often Loomarr drains the drop folder and reconciles its own clip library.",
		},
		{
			// The drop-folder's on/off switch on the Sources tab (§10 V35). A setting rather
			// than a row because the folder is DERIVED from `filler.dir` — a remote
			// collection's switch is a column on its own row, so a row and a setting never
			// describe the same source.
			//
			// ⚠ Default TRUE, and it must stay that way: an install that has never seen this
			// key has a working drop-folder, and defaulting to false would silently stop the
			// scan on upgrade.
			//
			// ⚠ There is deliberately no `filler.source.library.enabled`. Nothing scans a
			// media-server library for clips (§10 took the media server out of the filler
			// path), so that key would gate no work — a control that dims a row and changes
			// nothing. The Sources tab renders that row as provenance, without a switch.
			Key: "filler.source.folder.enabled", Label: "Watch the drop folder", EnvVar: "FILLER_SOURCE_FOLDER_ENABLED", Group: GroupFiller,
			Kind: KindBool, Default: true,
			Doc:    "Pick up clips dropped into the drop folder.",
			Detail: "Scan the drop-folder for clips. Switching it off stops the catalog sync; clips already in the catalog stay.",
		},
		{
			// Selective speech details (§10 V44). Enabled by default because it only listens when a
			// source did not provide enough identity, and the local engine has no egress or usage cost.
			// Choosing a connected service remains the explicit egress/cost decision.
			Key: "filler.transcribe.enabled", Label: "Listen for missing clip details", EnvVar: "FILLER_TRANSCRIBE_ENABLED", Group: GroupFiller,
			Kind: KindBool, Default: true, Advanced: true,
			Doc:    "Listen to clips with little description for spoken brand and product names.",
			Detail: "When a source provides very little information, listen for spoken brand and product details. Loomarr skips this for clips that already have enough context. Choose the speech service under Settings → AI.",
		},
		{
			// Vision tagging (§10 V44). ⚠ OFF by default AND gated on a vision-capable LLM: the
			// hosted path spends multimodal tokens per clip and sends frames off the box, the local
			// path needs an Ollama vision model. Off, or with no vision model, the job is inert.
			Key: "filler.vision.enabled", Label: "Inspect unclear clips with vision AI", EnvVar: "FILLER_VISION_ENABLED", Group: GroupFiller,
			Kind: KindBool, Default: false, Advanced: true,
			Doc:    "Look at clips Loomarr still can't identify to read logos and on-screen text.",
			Detail: "Look at a few frames of clips Loomarr still can't identify — reading on-screen logos and text — to work out the brand, even for clips with no speech. Needs a vision-capable AI model.",
		},
		{
			// ⚠ Its OWN model knob, exactly like filler.language_model — and the live test that
			// added it found why: the tagging model (`llm.model`) is often a TEXT model with no
			// vision path (qwen3 in dev), while the box has a separate vision-capable one (gemma-4).
			// Tying vision to `llm.model` would force an operator to switch their whole LLM to a
			// vision model just to tag clips. Empty ⇒ fall back to `llm.model`, so an install whose
			// main model already sees images needs no second setting.
			Key: "filler.vision.model", Label: "Vision model", EnvVar: "FILLER_VISION_MODEL", Group: GroupFiller,
			Kind: KindString, Default: "", Advanced: true,
			Doc:    "Which AI model looks at clip pictures, or empty to use your main model.",
			Detail: "Which AI model reads clip frames (must be vision-capable). Leave empty to reuse your main model — set it only when that model can't see images.",
		},
		{
			// ⚠ **The model knob above was only half a separation, and the half it was missing is
			// the one that fails silently.** `filler.vision.model` let an operator name a vision
			// model, but the provider was still built from `llm.provider`/`llm.url`/`llm.api_key`
			// — so naming a LOCAL model while the main LLM was hosted sent an Ollama tag to the
			// hosted endpoint. Measured: `llava:7b` → `https://openrouter.ai/api/v1` → 401 on every
			// segment, which `ground` reports as zero looks and the gate refuses as "a segment
			// could not be classified". The model name is useless without the host that serves it.
			//
			// Empty ⇒ inherit the main provider.
			// ⚠ `inherit` is a REAL value, not an empty string: the registry invariant requires
			// every enum option to carry a value and a label, and an explicit word reads better in
			// the picker than a blank row. The resolver treats "" the same way, so an env var set
			// to empty means inherit rather than "no provider".
			Key: "filler.vision.provider", Label: "Vision service", EnvVar: "FILLER_VISION_PROVIDER", Group: GroupFiller,
			Kind: KindEnum, Default: "inherit", Advanced: true,
			Enum: []EnumOption{
				opt("inherit", "Same as your main AI"),
				opt("ollama", "Ollama"),
				opt("openai", "OpenAI-compatible"),
			},
			Doc:    "Which AI service looks at clip pictures.",
			Detail: "Leave as “same as your main AI” unless your vision model lives somewhere else — a local Ollama, say, while your main AI is a hosted service.",
		},
		{
			// Empty + `ollama` resolves to the conventional local host, the same rule `ollamaBase`
			// already applies to probes and pulls — so the common case (hosted text, local vision)
			// needs the provider knob alone.
			Key: "filler.vision.url", Label: "Vision service address", EnvVar: "FILLER_VISION_URL", Group: GroupFiller,
			Kind: KindURL, Default: "", Advanced: true,
			Doc:      "The address of that picture service.",
			Detail:   "Leave empty for a local Ollama on this machine.",
			ShowWhen: map[string][]string{"filler.vision.provider": {"ollama", "openai"}},
		},
		{
			// ⚠ **Never falls back to `llm.api_key`, and that is the point rather than an
			// omission.** Declaring a separate vision service means declaring its own credentials:
			// inheriting would send the operator's hosted key to whatever host they just named,
			// including `localhost`. A local Ollama needs no key, so the common case leaves this
			// empty and nothing is sent.
			Key: "filler.vision.api_key", Label: "Vision service API key", EnvVar: "FILLER_VISION_API_KEY", Group: GroupFiller,
			Kind: KindSecret, Default: "", Advanced: true,
			Doc:      "The API key for that picture service, if it needs one.",
			Detail:   "Your main AI's key is never reused here. Never shown again after saving.",
			ShowWhen: map[string][]string{"filler.vision.provider": {"openai"}},
		},
		{
			// The defect floor's judge (#2034) reuses the vision model above. Each look is reserved
			// at this ceiling before the call and settled at what it cost; an operator-run endpoint
			// reserves nothing. Whole cents, because a setting has no decimal kind.
			Key: "filler.defects.judge_look_cents", Label: "Most one picture-quality check may cost (US cents)", EnvVar: "FILLER_DEFECTS_JUDGE_LOOK_CENTS", Group: GroupFiller,
			Kind: KindInt, Default: 2, Advanced: true,
			Doc:    "The most one picture-quality check on a paid AI service may cost.",
			Detail: "Ads whose picture is close to the quality limit are shown to the vision model above for a ruling. This is the most one look may cost on a hosted model. 0 stops hosted looks, and those ads wait.",
		},
		{
			// ⚠ The ledger's day is shared: it sums every metered filler model call that day, not
			// only the judge's, so the doc says "paid filler AI" rather than "these checks".
			Key: "filler.defects.judge_daily_cents", Label: "Daily ceiling for picture-quality checks (US cents)", EnvVar: "FILLER_DEFECTS_JUDGE_DAILY_CENTS", Group: GroupFiller,
			Kind: KindInt, Default: 50, Advanced: true,
			Doc:    "The most picture-quality checks on a paid AI service may cost in one day.",
			Detail: "Picture-quality checks on a hosted model stop for the day (UTC) once Loomarr's paid filler AI spend that day would pass this. Ads past the limit wait for the next day.",
		},

		// Automatic compilation splitting (§10 V43). Detection ran only on a button press and
		// its result always required a human, which made compilations the most manual part of a
		// system whose claim is that it maintains itself. Cut acceptance still creates held
		// children; it does not grant any child permission to air.
		{
			// Splitting has no sweep cadence of its own: it is a rung every long recording reaches
			// as it is ingested, and detection is bounded by `filler.pipeline.max_splits` per pass.
			//
			// ⚠ **ON by default (V51b).** Cutting is destructive in a way tagging is not — a
			// mis-cut clip plays HALF AN ADVERT and the source is consumed either way — and that
			// risk has not changed; what makes ON safe is the evidence. The gate is
			// strict — after known duplicates and short fragments are removed, the remaining reel
			// qualifies as a whole or none of it does; an ungrounded era disqualifies at every
			// threshold, and any segment the detector admits it could not resolve sends the
			// remaining reel to a human — and the measured failure mode is the gate REFUSING good
			// reels, not admitting bad ones. Off by default meant every compilation waited for a
			// click that the design says should be unnecessary.
			Key: "filler.autosplit.enabled", Label: "Accept confident cuts automatically", EnvVar: "FILLER_AUTOSPLIT_ENABLED", Group: GroupFiller,
			Kind: KindBool, Default: true,
			Doc:    "Cut up recordings of several ads without asking when Loomarr is sure of every cut.",
			Detail: "Accept the cuts automatically when Loomarr is confident about every one of them. Anything less certain still waits for you under Filler → Incoming.",
		},
		{
			// ⚠ ONE key doing two jobs on purpose. It selects which clips the split job even
			// looks at (longer than this ⇒ a compilation worth detecting) AND it is the ceiling
			// every segment must clear to auto-confirm. Two keys could disagree — a clip the job
			// considers too long to be an advert must not then auto-confirm as one.
			Key: "filler.autosplit.max_duration", Label: "Longest expected single clip", EnvVar: "FILLER_AUTOSPLIT_MAX_DURATION",
			Group: GroupFiller, Kind: KindDuration, Default: "120s",
			Doc:    "The longest a single advert is expected to be.",
			Detail: "Recordings longer than this are treated as compilations worth splitting, and any piece longer than this is one Loomarr will ask you about.",
		},
		{
			Key: "filler.structure_window_authority_path", Label: "Long-reel authority file", EnvVar: "FILLER_STRUCTURE_WINDOW_AUTHORITY_PATH", Group: GroupFiller,
			Kind: KindString, Presentation: PresentationPath, Default: "", Validate: storagePath(true), Advanced: true,
			Doc:      "A reviewed file that allows certain long recordings to be split, for advanced installs.",
			Detail:   "Optional absolute path to a separately reviewed long-reel materialization authority. Empty or invalid evidence enables no certified slice; a valid authority may create held children but cannot make them airable. A change applies to the next split; a split already running finishes under the files it started with.",
			ShowWhen: map[string][]string{"filler.autosplit.enabled": {"true"}},
		},
		{
			Key: "filler.structure_window_deployment_path", Label: "Long-reel deployment file", EnvVar: "FILLER_STRUCTURE_WINDOW_DEPLOYMENT_PATH", Group: GroupFiller,
			Kind: KindString, Presentation: PresentationPath, Default: "", Validate: storagePath(true), Advanced: true,
			Doc:      "A reviewed file naming the paid AI route long-recording splits may use, for advanced installs.",
			Detail:   "Optional absolute path to the reviewed authority's OpenRouter route and spend deployment. Empty, invalid, or mismatched evidence performs no structure inference and enables no certified slice. A change applies to the next split; a split already running finishes under the files it started with.",
			ShowWhen: map[string][]string{"filler.autosplit.enabled": {"true"}},
		},
		// The ingest pipeline's per-run budget (§10 V51b). Every one of these bounds ONE PASS, not
		// the catalog: a backlog drains over cycles, which is the property the per-job batch
		// constants they replace (LanguageBatch 25, TranscribeBatch 10, VisionBatch 5,
		// defaultSplitsPerRun 3) were chosen to defend. The numbers are carried forward unchanged.
		//
		// ⚠ **Zero means NONE, and that is a distinct state from the default.** It is the only way
		// an operator can say "never do this kind of work on this box" — the same three-state
		// encoding `filler.fetch.every` uses, and the reason these are integers rather than
		// booleans-plus-a-rate.
		{
			Key: "filler.pipeline.max_clips", Label: "Clips prepared per pass", EnvVar: "FILLER_PIPELINE_MAX_CLIPS", Group: GroupFiller,
			Kind: KindInt, Default: 25, Advanced: true,
			Doc:    "How many new clips Loomarr prepares at a time.",
			Detail: "How many clips Loomarr advances through preparation in one pass. A large import drains over several passes rather than occupying the machine in one.",
		},
		{
			// ⚠ THREE, the tightest budget here, because a transcode competes with playout for
			// the GPU and this is what makes the existing catalog backfill converge over a day
			// instead of pinning the box. Zero switches re-encoding off entirely — an escape
			// hatch that matters, because this is the one rung that rewrites the operator's file.
			Key: "filler.transcode.max_per_run", Label: "Clips re-encoded per pass", EnvVar: "FILLER_TRANSCODE_MAX_PER_RUN", Group: GroupFiller,
			Kind: KindInt, Default: 3, Advanced: true,
			Doc:    "How many clips Loomarr converts to its standard format at a time.",
			Detail: "How many clips Loomarr re-encodes to its standard format in one pass. Set to 0 to never re-encode — clips then play in whatever format they arrived in.",
		},
		{
			Key: "filler.pipeline.max_whisper", Label: "Clips transcribed per pass", EnvVar: "FILLER_PIPELINE_MAX_WHISPER", Group: GroupFiller,
			Kind: KindInt, Default: 10, Advanced: true,
			Doc:    "How many clips Loomarr listens to at a time.",
			Detail: "How many clips Loomarr listens to in one pass, for language and transcription together. Listening is slow — minutes per clip on some machines — so this keeps a pass from running away.",
		},
		{
			Key: "filler.pipeline.max_vision", Label: "Clips inspected visually per pass", EnvVar: "FILLER_PIPELINE_MAX_VISION", Group: GroupFiller,
			Kind: KindInt, Default: 5, Advanced: true,
			Doc:    "How many clips Loomarr looks at with AI at a time.",
			Detail: "How many clips Loomarr looks at with a vision model in one pass. The smallest budget, because on a hosted model each one is a charge.",
		},
		{
			// ⚠ Counted in SEGMENTS, not clips — the only budget here that is, and §10 V51g is
			// why it must exist. A rung's cost may not scale unboundedly with a clip's content:
			// the live catalog holds proposals of 235, 222, 142 and 133 segments, so an unbounded
			// per-segment pass is exactly the shape that burned 377s against a 120s budget.
			//
			// A reel with more segments than this continues on the next pass. The proposal persists
			// Looked per segment, so this is a resource budget rather than a review threshold.
			Key: "filler.pipeline.max_split_vision", Label: "Compilation segments inspected per pass", EnvVar: "FILLER_PIPELINE_MAX_SPLIT_VISION", Group: GroupFiller,
			Kind: KindInt, Default: 60, Advanced: true,
			Doc:    "How many pieces of one long recording Loomarr looks at a time.",
			Detail: "How many segments of one recording Loomarr looks at in a single pass. A longer recording is judged over several passes rather than made to wait for you — this bounds how much looking happens at once, not how big a recording can be.",
		},
		{
			Key: "filler.pipeline.max_splits", Label: "Compilations split per pass", EnvVar: "FILLER_PIPELINE_MAX_SPLITS", Group: GroupFiller,
			Kind: KindInt, Default: 3, Advanced: true,
			Doc:    "How many long recordings Loomarr looks inside at a time.",
			Detail: "How many long recordings Loomarr looks inside in one pass. Finding the adverts in one recording takes minutes.",
		},
		// Auto-fetch and its limits (§10 V38b). A registered source is polled on a schedule, which
		// supersedes §15's "there is no unattended crawler" — the superseded rule's concern
		// survives as these bounds rather than as a prohibition.
		//
		// ⚠ Every one of them fails toward doing LESS. An operator who never opens this page gets
		// a trickle they can live with; the failure mode being designed against is "add a source,
		// wake up to 8,000 files".
		{
			Key: "filler.fetch.every", Label: "Look for new clips", EnvVar: "FILLER_FETCH_EVERY", Group: GroupFiller,
			Kind: KindDuration, Presentation: PresentationFillerDownloadSchedule, Default: "6h", Validate: fillerFetchInterval,
			Presets: []Preset{
				preset("0s", "Never"), preset("6h", "Every 6 hours"), preset("12h", "Every 12 hours"),
				preset("24h", "Daily"), preset("168h", "Weekly"),
			},
			Doc:    "How often Loomarr checks enabled sources for new clips.",
			Detail: "Never stops sources that use this default; you can still check one yourself.",
		},
		{
			Key: "filler.fetch.max_per_run", Label: "Add up to", EnvVar: "FILLER_FETCH_MAX_PER_RUN", Group: GroupFiller,
			Kind: KindInt, Default: 10, Validate: positiveLimit,
			Doc: "The most clips each enabled source may add in one automatic check.",
		},
		{
			// ⚠ OFF until automatic splitting is certified (#952, #963; #1773). Every filler item
			// waiting on a person was a compilation reel whose cut points needed confirming, and
			// review should be a last resort. Off holds back only NEW reels from sources and
			// planned pulls (a known runtime over filler.autosplit.max_duration, the same signal
			// probe uses); nothing already downloaded is dropped, and the drop folder is exempt.
			Key: "filler.acquisition.compilations", Label: "Download compilation reels", EnvVar: "FILLER_ACQUISITION_COMPILATIONS", Group: GroupFiller,
			Kind: KindBool, Default: false,
			Doc:    "Also download recordings of several adverts in one video.",
			Detail: "Off until Loomarr can split them without asking you; sources then add single adverts only. Your drop folder is never affected.",
		},
		{
			// ⚠ Bounds the UNATTENDED path only. An admin queueing a clip or approving a pull is
			// a deliberate act and is not stopped by this — a ceiling on what happens while
			// nobody is looking is not a ceiling on what someone chooses to do.
			Key: "filler.fetch.max_catalog_clips", Label: "Automatic-download catalog limit", EnvVar: "FILLER_FETCH_MAX_CATALOG_CLIPS", Group: GroupFiller,
			Kind: KindInt, Default: 2000, Advanced: true, Validate: positiveLimit,
			Doc:    "Stop fetching automatically once your catalog reaches this many clips.",
			Detail: "You can still add more by hand.",
		},
		{
			Key: "filler.storage.library_budget_gb", Label: "Filler storage allowance", EnvVar: "FILLER_STORAGE_LIBRARY_BUDGET_GB", Group: GroupFiller,
			Kind: KindInt, Unit: UnitGiB, Default: 0, Validate: nonNegativeWholeNumber,
			Doc:    "How much disk space Loomarr may use for filler, or 0 to choose a safe amount.",
			Detail: "In GiB. Leave at 0 to choose a safe allowance automatically. Loomarr always keeps a protected amount of free space for the rest of the system.",
		},
		{
			Key: "filler.research.enabled", Label: "Find missing details automatically", EnvVar: "FILLER_RESEARCH_ENABLED", Group: GroupFiller,
			Kind: KindBool, Presentation: PresentationSwitch, Default: true,
			Doc:    "Look up a clip's likely time period and place in public sources.",
			Detail: "This does not affect whether the clip can play.",
		},
		{
			Key: "filler.research.wikidata_enabled", Label: "Wikidata", EnvVar: "FILLER_RESEARCH_WIKIDATA_ENABLED", Group: GroupFiller,
			Kind: KindBool, Presentation: PresentationSwitch, Default: true, Advanced: true,
			Doc: "Search Wikidata for public facts that may help identify a clip's era or location.",
		},
		{
			Key: "filler.research.wikipedia_enabled", Label: "Wikipedia", EnvVar: "FILLER_RESEARCH_WIKIPEDIA_ENABLED", Group: GroupFiller,
			Kind: KindBool, Presentation: PresentationSwitch, Default: true, Advanced: true,
			Doc: "Search Wikipedia for public background that may help identify a clip's era or location.",
		},
		{
			Key: "filler.research.archive_enabled", Label: "Archive.org", EnvVar: "FILLER_RESEARCH_ARCHIVE_ENABLED", Group: GroupFiller,
			Kind: KindBool, Presentation: PresentationSwitch, Default: true, Advanced: true,
			Doc:    "Search the Internet Archive's public catalog for related historical items.",
			Detail: "Searches Archive.org's public catalog for related historical items and metadata.",
		},
		{
			Key: "filler.research.loc_enabled", Label: "Library of Congress", EnvVar: "FILLER_RESEARCH_LOC_ENABLED", Group: GroupFiller,
			Kind: KindBool, Presentation: PresentationSwitch, Default: true, Advanced: true,
			Doc: "Search the Library of Congress public catalog for related historical records.",
		},
		{
			Key: "filler.research.web_provider", Label: "Web search provider", EnvVar: "FILLER_RESEARCH_WEB_PROVIDER", Group: GroupFiller,
			Kind: KindEnum, Default: "none", Advanced: true,
			Enum:   []EnumOption{opt("none", "Not configured"), opt("brave", "Brave Search"), opt("searxng", "SearXNG (self-hosted)")},
			Doc:    "A general web search Loomarr may try when public sources can't identify a clip.",
			Detail: "Optional last-resort search after trusted public sources cannot identify a clip. Search is automatic and never decides safety, readiness, or scheduling.",
		},
		{
			Key: "filler.research.brave_api_key", Label: "Brave Search API key", EnvVar: "FILLER_RESEARCH_BRAVE_API_KEY", Group: GroupFiller,
			Kind: KindSecret, Default: "", Advanced: true,
			ShowWhen: map[string][]string{"filler.research.web_provider": {"brave"}},
			Doc:      "Your Brave Search API key.",
			Detail:   "Installation-wide Brave Search key. Loomarr sends the public clip title and search terms, never the media file. The key is encrypted and never shown again.",
		},
		{
			Key: "filler.research.searxng_url", Label: "SearXNG address", EnvVar: "FILLER_RESEARCH_SEARXNG_URL", Group: GroupFiller,
			Kind: KindURL, Default: "", Advanced: true, Validate: credentialFreeHTTPSEndpoint,
			ShowWhen: map[string][]string{"filler.research.web_provider": {"searxng"}},
			Doc:      "The address of your SearXNG search server.",
			Detail:   "HTTPS address of your SearXNG server. Loomarr requests its bounded JSON search API and does not fetch result pages.",
		},
		{
			Key: "filler.research.monthly_limit", Label: "Monthly web searches", EnvVar: "FILLER_RESEARCH_MONTHLY_LIMIT", Group: GroupFiller,
			Kind: KindInt, Default: 100, Advanced: true, Validate: positiveWholeNumber,
			Doc:    "The most web searches Loomarr may make in a month.",
			Detail: "Most general-web searches Loomarr may make in one UTC calendar month. Trusted public-source lookups do not count toward this limit.",
		},
		{
			Key: "filler.incoming.ready_window", Label: "Keep ready clips in Incoming", EnvVar: "FILLER_INCOMING_READY_WINDOW", Group: GroupFiller,
			Kind: KindDuration, Default: "24h", Advanced: true, Validate: fillerIncomingReadyWindow,
			Doc:    "How long clips stay under Ready in Incoming.",
			Detail: "They remain in your Library after they leave this view.",
		},
		{
			Key: "filler.breaks_per_hour", Label: "Breaks per program hour", EnvVar: "FILLER_BREAKS_PER_HOUR", Group: GroupFiller,
			Kind: KindInt, Default: 4, Validate: nonNegativeWholeNumber,
			Doc:    "How many commercial breaks an hour of programmes gets, unless a channel sets its own.",
			Detail: "Default commercial-break frequency for channels that follow it. Set 0 to disable breaks by default; each channel can choose its own frequency.",
		},
		{
			Key: "filler.break_duration", Label: "Length of each break", EnvVar: "FILLER_BREAK_DURATION", Group: GroupFiller,
			Kind: KindDuration, Default: "2m", Validate: breakDuration,
			Doc:    "How long each commercial break lasts, unless a channel sets its own.",
			Detail: "How long each commercial break lasts by default, with the commercials rotated so recently shown ones wait their turn. Channels can choose their own length. Use breaks per program hour to turn breaks off.",
		},
		{
			Key: "filler.pod_max", Label: "Clips per break", EnvVar: "FILLER_POD_MAX", Group: GroupFiller,
			Kind: KindInt, Default: 4, Validate: positiveWholeNumber,
			Doc:    "How many clips a break usually holds.",
			Detail: "Preferred clip count per break. Loomarr automatically exceeds it when shorter clips need more slots to fill the requested break length.",
		},
		{
			Key: "filler.cooldown_seconds", Label: "Repeat spacing · same commercial", EnvVar: "FILLER_COOLDOWN_SECONDS", Group: GroupFiller,
			Kind: KindInt, Unit: UnitSeconds, Default: 14400, Advanced: true, Validate: nonNegativeWholeNumber,
			Doc:    "How long before a commercial you've seen can air again on any channel.",
			Detail: "Default seconds before a commercial seen on any channel airs again (14400 is 4 hours). Only watched airings count. Loomarr first reaches for commercials from a nearby era, and repeats one sooner only when that would otherwise leave a break short. Each channel can choose its own spacing.",
		},
		// ⚠ Default 0 = OFF, and that is the whole safety property of this knob (V17c).
		// `00014_clips_quality` shipped with quality as display-only and warned that a
		// well-meaning "prefer HD" would quietly starve the era-accurate 4:3 commercials the
		// feature exists to play. That warning holds — which is why an install that sets
		// nothing selects no quality floor at all, pinned by a test.
		//
		// Advanced: an operator who does not know what 240p looks like in a break should never
		// meet this, and one who does will go looking.
		{
			Key: "filler.min_quality", Label: "Minimum picture height (px)", EnvVar: "FILLER_MIN_QUALITY", Group: GroupFiller,
			Kind: KindInt, Default: 0, Advanced: true,
			Doc:    "The lowest picture size a commercial may have and still air, or 0 for any.",
			Detail: "Minimum clip height in pixels for a commercial to be eligible (480 excludes 240p rips). 0 disables the floor, which is the default — era accuracy beats resolution.",
		},
		{
			Key: "filler.weight", Label: "Selection weight", EnvVar: "FILLER_WEIGHT", Group: GroupFiller,
			Kind: KindInt, Default: 1, Advanced: true,
			Doc: "How heavily this commercial set is drawn from, relative to others.",
		},
		{
			// ⚠ A REJECT, not a filter, and its default is ON — unlike filler.min_quality above,
			// which is opt-in eligibility over clips that already exist. A file shorter than this
			// never becomes a catalog row at all (§10 V40).
			//
			// It exists because `DurationMs <= 0` was the only guard, and a 2.9KB / 33ms truncated
			// download passed it and sat filed-and-airable in the dev catalog.
			Key: "filler.min_duration", Label: "Reject files shorter than", EnvVar: "FILLER_MIN_DURATION", Group: GroupFiller,
			Kind: KindDuration, Default: "10s", Advanced: true,
			Doc:    "Files shorter than this are turned away as broken downloads.",
			Detail: "Clips shorter than this are rejected on sight and never enter the catalog — a truncated download is not a short commercial. Set to 0s to accept anything with a readable duration.",
		},
		{
			// ⚠ **The only setting in Loomarr that deletes an operator's media**, which is why its
			// doc says so in the operator's own words rather than in ours. Everything else here
			// tombstones: "remove from catalog" keeps the file, disabling a source keeps its clips.
			//
			// It exists because partial confirm (§10 V54) leaves a residue BY DESIGN — every reel
			// files its confident cuts and keeps the doubtful ones back, so small proposals
			// accumulate and each one pins a 1–2 GB recording. Without an expiry the feature that
			// shrinks the operator's work grows their storage instead.
			//
			// ⚠ 0s is OFF, and that is the three-state encoding the rest of §10 uses: an operator
			// who has not chosen an expiry has not agreed to have recordings deleted. A reel that
			// produced NO clips is never eligible at any window — it is the only copy of that
			// content, and reaping it would destroy material Loomarr never managed to use.
			Key: "filler.split.review_window", Label: "Keep unreviewed cuts for", EnvVar: "FILLER_SPLIT_REVIEW_WINDOW", Group: GroupFiller,
			Kind: KindDuration, Default: "720h", Advanced: true,
			Doc:    "How long cuts you haven't reviewed wait before Loomarr gives up on them.",
			Detail: "When the time is up it drops the leftover cuts and DELETES the original recording to reclaim the space — but only for recordings that already produced clips, so nothing is lost that was never used. The clips themselves are never touched. Set to 0s to keep everything forever.",
		},
		{
			// ⚠ **ELIGIBILITY, not a reject — the pair to `filler.min_duration` above, and the
			// distinction is which side of the catalog boundary they sit on.** `min_duration`
			// refuses a file entry; these two decide whether a clip already IN the catalog may
			// fill a break. A clip outside them stays filed, searchable and pinnable; it simply
			// is not drawn automatically.
			//
			// ⚠ **`Policy.MinClipMs`/`MaxClipMs` existed for phases with no way to set them.**
			// They were assigned in tests and nowhere else — no key, no env var, no policy field —
			// so `durationEligible` always returned true and `PoolReport.Eligible`, which
			// `coverage.go` headlines as "the number that surprises operators", was arithmetically
			// identical to `Commercials` on every install. The pool strip showed one number twice
			// and called the pair a diagnosis. Wiring them is what makes that strip mean something.
			//
			// Both default to 0 = OFF, so no existing install changes behaviour on upgrade.
			Key: "filler.min_clip_duration", Label: "Shortest clip used automatically", EnvVar: "FILLER_MIN_CLIP_DURATION", Group: GroupFiller,
			Kind: KindDuration, Default: "0s", Advanced: true,
			Doc:    "Commercials shorter than this are not picked for breaks automatically.",
			Detail: "0s disables the floor. Unlike the minimum clip length above, this never rejects a clip — it stays in the catalog and can still be pinned to a channel.",
		},
		{
			Key: "filler.max_clip_duration", Label: "Longest clip used automatically", EnvVar: "FILLER_MAX_CLIP_DURATION", Group: GroupFiller,
			Kind: KindDuration, Default: "0s", Advanced: true,
			Doc:    "Commercials longer than this are not picked for breaks automatically.",
			Detail: "This is the guard against a three-minute infomercial filling a thirty-second gap. 0s disables the ceiling. Never rejects a clip; it stays in the catalog and can still be pinned.",
		},
		{
			// ⚠ Applied in the PLAYOUT chain, never written back to the file. The drop-folder
			// holds the operator's own files; in-place normalisation is destructive, unrepeatable,
			// and a re-scan cannot tell it already happened (§10 V40, §9.1).
			//
			// Measured spread across real fetched clips: -21.8 to -32.6 LUFS, ~11 dB of
			// clip-to-clip jump. -23 is the broadcast target.
			Key: "filler.target_lufs", Label: "Playback loudness target (LUFS)", EnvVar: "FILLER_TARGET_LUFS", Group: GroupFiller,
			Kind: KindString, Default: "-23", Advanced: true,
			Doc:    "How loud every filler clip plays, so breaks don't jump in volume.",
			Detail: "Loudness every filler clip is normalised to, in LUFS (-23 is the broadcast standard). Clips are measured at ingest; at playout a clip that misses the target gets a small constant gain (never a live filter, so no volume swell at clip starts). Empty disables it and clips play at whatever level they were recorded.",
		},
		{
			// ⚠ A clip with NO speech is always kept — a wordless visual spot has no language, and
			// those are often the best filler. Only confident non-target speech rejects (§10 V40).
			Key: "filler.language", Label: "Commercial language", EnvVar: "FILLER_LANGUAGE", Group: GroupFiller,
			Kind: KindEnum, Enum: fillerLanguageOptions(), Presentation: PresentationLanguage, Default: "en",
			Doc:    "The language commercials are in, unless a channel sets its own.",
			Detail: "Every channel's commercial language unless the channel sets its own. New clips are kept when their speech is in a language some channel uses and rejected when it is confidently in none of them; a clip with no speech at all is always kept. A clip whose language could not be determined airs only on channels that use this language.",
		},
		// --- Filler ingest (§10, §15) ---
		// ⚠ The vendored binaries ship in the SINGLE image (§16), so these paths are overrides
		// for an unusual layout, not an opt-in.
		// The two tool paths are what the `ingest` feature gate probes. They are
		// settings rather than hardcoded so an operator can point at a NEWER yt-dlp
		// than the image ships — yt-dlp releases fixes far faster than we cut images,
		// and a stale one silently stops extracting from YouTube.
		{
			Key: "ingest.ytdlp_path", Label: "yt-dlp executable", EnvVar: "INGEST_YTDLP_PATH", Group: GroupFiller,
			Kind: KindString, Presentation: PresentationPath, Default: "", Advanced: true,
			Doc:    "Where the yt-dlp download program lives.",
			Detail: "The Loomarr image sets this; empty means clip downloading is off.",
		},
		{
			Key: "ingest.ffmpeg_path", Label: "ffmpeg executable", EnvVar: "INGEST_FFMPEG_PATH", Group: GroupFiller,
			Kind: KindString, Presentation: PresentationPath, Default: "", Advanced: true,
			Doc: "Where the ffmpeg program lives (yt-dlp needs it to combine video and audio).",
		},
		{
			Key: "ingest.timeout", Label: "Download timeout", EnvVar: "INGEST_TIMEOUT", Group: GroupFiller,
			Kind: KindDuration, Default: "30m", Advanced: true,
			Doc: "How long one download may run before it's stopped, so a stuck fetch can't block others.",
		},
		// Compilation splitting (§10, V34). Empty/unrunnable ⇒ the transcript-rescue
		// step is unavailable and over-long segments surface as UNSPLITTABLE in the
		// review rather than being guessed at — coarse splitting needs only ffmpeg.
		{
			Key: "ingest.whisper_path", Label: "whisper executable", EnvVar: "INGEST_WHISPER_PATH", Group: GroupFiller,
			Kind: KindString, Presentation: PresentationPath, Default: "", Advanced: true,
			Doc:    "Where the whisper-cli speech program lives.",
			Detail: "The image sets this; empty means over-long compilation segments can't be transcribed for hidden ad breaks.",
		},
		{
			Key: "ingest.whisper_model", Label: "Transcription model file", EnvVar: "INGEST_WHISPER_MODEL", Group: GroupFiller,
			Kind: KindString, Presentation: PresentationPath, Default: "", Advanced: true,
			Doc:    "The speech model file used to transcribe long recordings.",
			Detail: "The whisper model file whisper-cli transcribes with. Size is a correctness property, not a quality preference — too small drops audio and the boundary detector then invents breaks.",
		},
		{
			// ⚠ A SECOND model, and the reason is the `.en` suffix on the one above. An
			// English-only whisper build does NOT identify languages — it assumes English and
			// transcribes accordingly, so asked about a Spanish advert it answers "en" and the
			// language gate silently never rejects anything (§10 V40).
			//
			// `tiny` (multilingual, ~74MB) is adequate here in a way it was not for splitting:
			// language ID is CLASSIFICATION over the first seconds, not transcription, so the
			// "does it drop audible speech" gate that ruled out tiny.en never applies.
			//
			// Empty ⇒ local language detection is unavailable and the gate stays inert; the image
			// sets it, a source build does not.
			Key: "filler.language_model", Label: "Language detection model file", EnvVar: "FILLER_LANGUAGE_MODEL", Group: GroupFiller,
			Kind: KindString, Presentation: PresentationPath, Default: "", Advanced: true,
			Doc:    "The model file used to work out what language a clip is in.",
			Detail: "Must be a MULTILINGUAL whisper model — an English-only one reports every clip as English, so the check would never reject anything. The image ships one; leave empty to turn local detection off.",
		},
		// --- Users & security (§15, Phase 9) ---
		{
			Key: "session.ttl", Label: "Sign-in lifetime", EnvVar: "SESSION_TTL", Group: GroupUsersSecurity,
			Kind: KindDuration, Default: "720h",
			Doc: "How long you stay signed in before needing to log in again.",
		},
		{
			Key: "cookie.secure", Label: "Secure cookies", EnvVar: "COOKIE_SECURE", Group: GroupUsersSecurity,
			Kind: KindEnum, Enum: []EnumOption{opt("auto", "Auto (match the request)"), opt("always", "Always"), opt("never", "Never (local dev only)")}, Default: "auto", Advanced: true,
			Doc: "When to mark the login cookie secure: auto (match the request), always, or never (for local dev only).",
		},
		{
			Key: "security.trust_proxy", Label: "Trust reverse proxy", EnvVar: "TRUST_PROXY", Group: GroupUsersSecurity,
			Kind: KindBool, Default: "false", Advanced: true,
			Doc:    "Turn on only when a reverse proxy sits in front of Loomarr.",
			Detail: "Trust the X-Forwarded-For and X-Forwarded-Proto headers. Turn this on only if a reverse proxy sits in front of Loomarr and sets them. Off by default so a direct client can't forge them to bypass the login rate limit or downgrade its cookie.",
		},

		// --- SSO: a third CREDENTIAL path, never a provisioning one (§11, D-F, V8) ---
		//
		// ⚠ There is deliberately NO `auth.sso.auto_create` and NO `auth.sso.admin_group`,
		// though the v2 mock draws both. Auto-create is lazy self-provision, which is exactly
		// what §11's allowlist exists to prevent; group-derived roles would move a Loomarr
		// decision to someone else's directory. Adding either key later is a §11 conversation,
		// not a settings change.
		{
			Key: "auth.sso.enabled", Label: "Enable single sign-on", EnvVar: "AUTH_SSO_ENABLED", Group: GroupSSO,
			Kind: KindBool, Default: "false",
			Doc:    "Let people sign in with your identity provider.",
			Detail: "They still need an account here — signing in with your provider does not create one.",
		},
		{
			Key: "auth.sso.issuer", Label: "SSO issuer URL", EnvVar: "AUTH_SSO_ISSUER", Group: GroupSSO,
			Kind: KindURL, Default: "",
			Doc:      "Your identity provider's address.",
			Detail:   "For example https://auth.example.home. Loomarr reads its published configuration from there.",
			ShowWhen: map[string][]string{"auth.sso.enabled": {"true"}},
		},
		{
			Key: "auth.sso.client_id", Label: "SSO client ID", EnvVar: "AUTH_SSO_CLIENT_ID", Group: GroupSSO,
			Kind: KindString, Default: "",
			Doc:      "The client ID your provider issued for Loomarr.",
			ShowWhen: map[string][]string{"auth.sso.enabled": {"true"}},
		},
		{
			Key: "auth.sso.client_secret", Label: "SSO client secret", EnvVar: "AUTH_SSO_CLIENT_SECRET", Group: GroupSSO,
			Kind: KindSecret, Default: "",
			Doc:      "The client secret your provider issued for Loomarr.",
			ShowWhen: map[string][]string{"auth.sso.enabled": {"true"}},
		},

		// --- Advanced: TTLs, retention, workers, event webhook (§15) ---
		{
			Key: "request.ttl", EnvVar: "REQUEST_TTL", Group: GroupAdvanced,
			Kind: KindDuration, Default: "48h",
			Doc: "How long Loomarr keeps trying to request a title before giving up.",
		},
		{
			Key: "downloading.ttl", EnvVar: "DOWNLOADING_TTL", Group: GroupAdvanced,
			Kind: KindDuration, Default: "12h",
			Doc: "How long a downloading title waits to finish before Loomarr gives up on it.",
		},
		// The background-job scheduler's per-job CRON schedules (§18.1). Sonarr/Overseerr-style
		// 6-field seconds-leading cron; edited via the Tasks page's Modify Job modal (presets
		// + an advanced raw-cron field). These OVERRIDE each job's built-in default cron.
		{
			Key: "job.system_health.schedule", EnvVar: "JOB_SYSTEM_HEALTH_SCHEDULE", Group: GroupAdvanced,
			Kind: KindCron, Presets: jobSchedulePresets, Default: "*/30 * * * * *",
			Doc:    "How often Loomarr checks its database and connections for Current Health.",
			Detail: "A cron schedule.",
		},
		{
			Key: "job.notification_delivery.schedule", EnvVar: "JOB_NOTIFICATION_DELIVERY_SCHEDULE", Group: GroupAdvanced,
			Kind: KindCron, Presets: jobSchedulePresets, Default: "*/15 * * * * *",
			Doc:    "How often Loomarr sends waiting invitation and recovery messages.",
			Detail: "Claims queued messages for delivery, on a cron schedule.",
		},
		{
			Key: "job.reconcile.schedule", EnvVar: "JOB_RECONCILE_SCHEDULE", Group: GroupAdvanced,
			Kind: KindCron, Presets: jobSchedulePresets, Default: "0 */5 * * * *",
			Doc:    "How often Loomarr checks on in-progress downloads.",
			Detail: "A cron schedule.",
		},
		{
			Key: "job.channel_maintenance.schedule", EnvVar: "JOB_CHANNEL_MAINTENANCE_SCHEDULE", Group: GroupAdvanced,
			Kind: KindCron, Presets: jobSchedulePresets, Default: "0 */10 * * * *",
			Doc:    "How often Loomarr refreshes series episodes and brings live channels up to date.",
			Detail: "Also reconciles live channels with Tunarr, on a cron schedule.",
		},
		{
			Key: "job.filler_sync.schedule", EnvVar: "JOB_FILLER_SYNC_SCHEDULE", Group: GroupAdvanced,
			Kind: KindCron, Presets: jobSchedulePresets, Default: "0 */15 * * * *",
			Doc:    "How often Loomarr syncs the filler catalog.",
			Detail: "A cron schedule.",
		},
		{
			// ⚠ Daily and off-peak on purpose. The window this job enforces is measured in WEEKS,
			// so a faster cadence buys nothing and only widens the chance of a pass landing while
			// an operator is mid-review on a reel one hour past its expiry.
			Key: "job.filler_split_sweep.schedule", EnvVar: "JOB_FILLER_SPLIT_SWEEP_SCHEDULE", Group: GroupAdvanced,
			Kind: KindCron, Presets: jobSchedulePresets, Default: "0 45 4 * * *",
			Doc:    "How often Loomarr checks for split suggestions you never reviewed.",
			Detail: "A cron schedule. What it does when it finds them is set by `filler.split.review_window`.",
		},
		{
			// Hourly at :20, clear of the :00 pipeline-heavy minute: a better copy that becomes
			// ready replaces the old one within the hour, and a pass only decodes clips it has
			// never fingerprinted (#2033).
			Key: "job.filler_identity.schedule", EnvVar: "JOB_FILLER_IDENTITY_SCHEDULE", Group: GroupAdvanced,
			Kind: KindCron, Presets: jobSchedulePresets, Default: "0 20 * * * *",
			Doc:    "How often Loomarr looks for copies of the same ad and keeps the best one.",
			Detail: "It also links shorter or alternate versions, on a cron schedule.",
		},
		{
			// Every five minutes: a new clip waits at the defects rung for this pass's verdict
			// (#2036), so the interval is how long admission waits. A pass decodes within a bounded
			// budget, held clips first, and an idle pass reads the records and decodes nothing.
			Key: "job.filler_defects.schedule", EnvVar: "JOB_FILLER_DEFECTS_SCHEDULE", Group: GroupAdvanced,
			Kind: KindCron, Presets: jobSchedulePresets, Default: "0 */5 * * * *",
			Doc:    "How often Loomarr checks ads' picture and sound against the standard for their era.",
			Detail: "A cron schedule. A new ad waits for this check before it airs, and an ad that fails is removed and a better copy sought.",
		},
		{
			// ⚠ **Every scheduled job needs its ScheduleKey declared here** — `Resolve` PANICS on
			// an undeclared key, so a job registered without one takes the whole app down at
			// startup rather than degrading. Caught by the boot test, which is exactly its job.
			//
			// Hourly rather than the 15-minute sync or the 6-hourly fetch, because this is the
			// expensive one: on the local backend a batch of 25 clips is minutes natively and
			// hours under QEMU (~341s per clip). Hourly drains a catalog steadily without a pass
			// overlapping the next.
			// ⚠ **This one key replaced FOUR** (§10 V51b), all now retired-ok:
			// `job.filler_language.schedule`, `job.filler_split.schedule` (retired-ok),
			// `job.filler_transcribe.schedule` and `job.filler_vision.schedule` (retired-ok).
			// Those four existed to keep expensive sweeps off each
			// other's toes by PHASE-OFFSETTING them (:15, :30, :45, :50) — a scheduling discipline
			// that only works while nobody adds a fifth, and which the comments they carried
			// spelled out at length.
			//
			// The pipeline makes the whole arrangement unnecessary rather than tidier: it runs
			// ONE clip at a time through all the rungs in order, so two expensive stages cannot
			// share the runner by construction. There is nothing left to offset.
			//
			// ⚠ Every two minutes, far tighter than the hourly sweeps, and affordable for the
			// reason the sweeps were not: a pass is bounded by the per-run budget rather than by
			// the catalog size, so an idle install costs one indexed query. It has to be tight,
			// because this is now the ONLY thing that advances a freshly downloaded clip.
			//
			// ⚠ Every job needs its schedule key declared or the settings service PANICS at
			// startup on `Resolve` of an undeclared key — so a new job and its key always land in
			// the same change.
			Key: "job.filler_pipeline.schedule", EnvVar: "JOB_FILLER_PIPELINE_SCHEDULE", Group: GroupAdvanced,
			Kind: KindCron, Presets: jobSchedulePresets, Default: "0 */2 * * * *",
			Doc:    "How often Loomarr prepares new filler clips.",
			Detail: "A cron schedule. Preparation means measuring, re-encoding, splitting, listening to and identifying them.",
		},
		// --- The image service jobs (§22, V52) ---
		//
		// ⚠ Every job needs its schedule key declared or the settings service PANICS at startup on
		// `Resolve` of an undeclared key, so all four land with the jobs themselves.
		{
			// Every minute, because this is what stands between an adopted row and a visible
			// image: `Adopt` deliberately does NOT fetch (a page adopting fifty posters would put
			// TMDB's latency on Loomarr's own page load), so until this runs the surface shows a
			// placeholder. A pass is bounded by the concurrency cap and an indexed work-list
			// query, so an idle install costs one cheap SELECT.
			Key: "job.images_fetch.schedule", EnvVar: "JOB_IMAGES_FETCH_SCHEDULE", Group: GroupAdvanced,
			Kind: KindCron, Presets: jobSchedulePresets, Default: "0 * * * * *",
			Doc:    "How often Loomarr downloads artwork it knows about but hasn't fetched yet.",
			Detail: "A cron schedule. Until this runs, those images show as placeholders.",
		},
		{
			// ⚠ Every five minutes: this is the step between a clip's artwork being RENDERED and
			// that artwork being visible through the image service, so a slow cadence reads as the
			// feature not working while an operator watches an import. Cheap to run often — the
			// work list selects only clips with artwork on disk and no image identity yet, so a
			// healthy install pays one indexed query.
			Key: "job.images_adopt_artwork.schedule", EnvVar: "JOB_IMAGES_ADOPT_ARTWORK_SCHEDULE", Group: GroupAdvanced,
			Kind: KindCron, Presets: jobSchedulePresets, Default: "0 */5 * * * *",
			Doc:    "How often Loomarr copies clip thumbnails and previews into the shared image library.",
			Detail: "Covers thumbnails and hover previews, on a cron schedule. Until a clip has been copied over, its older thumbnail is still what you see.",
		},
		{
			// At :20, clear of the filler media cluster (:15/:30/:45/:50) and of the two 04:xx
			// backup/retention jobs. AVIF encoding is CPU-intensive, so this is the image job that
			// genuinely contends for the box — §22 keeps it off request latency so a cold grid of
			// 50 posters cannot launch 50 encodes at once.
			Key: "job.images_avif.schedule", EnvVar: "JOB_IMAGES_AVIF_SCHEDULE", Group: GroupAdvanced,
			Kind: KindCron, Presets: jobSchedulePresets, Default: "0 20 * * * *",
			Doc:    "How often Loomarr makes the smallest copies of images in the background.",
			Detail: "Encodes the AVIF copies of images that don't have them yet, on a cron schedule. AVIF is the smallest format and the most expensive to produce, so it is made in the background; until it exists browsers take WebP.",
		},
		{
			Key: "job.images_maintenance.schedule", EnvVar: "JOB_IMAGES_MAINTENANCE_SCHEDULE", Group: GroupAdvanced,
			Kind: KindCron, Presets: jobSchedulePresets, Default: "0 0 5 * * *",
			Doc: "When Loomarr restores recoverable artwork, enforces retention, and cleans up image storage.",
		},
		{
			Key: "job.library_scan.schedule", EnvVar: "JOB_LIBRARY_SCAN_SCHEDULE", Group: GroupAdvanced,
			Kind: KindCron, Presets: jobSchedulePresets, Default: "0 */5 * * * *",
			Doc:    "How often Loomarr looks for new titles on the media server.",
			Detail: "Scans for newly-added titles to mark requested items available, on a cron schedule.",
		},
		{
			Key: "job.library_full_scan.schedule", EnvVar: "JOB_LIBRARY_FULL_SCAN_SCHEDULE", Group: GroupAdvanced,
			Kind: KindCron, Presets: jobSchedulePresets, Default: "0 0 3 * * *",
			Doc:    "How often Loomarr does a full media-server sweep to catch anything the quick scan missed.",
			Detail: "The quick scan is the incremental one above. A cron schedule.",
		},
		{
			Key: "job.library_scan.lookback", EnvVar: "JOB_LIBRARY_SCAN_LOOKBACK", Group: GroupAdvanced,
			Kind: KindDuration, Default: "1h",
			Doc: "How far back the incremental library scan looks for newly-added titles (should exceed the scan interval).",
		},
		{
			Key: "job.arr_queue_poll.schedule", EnvVar: "JOB_ARR_QUEUE_POLL_SCHEDULE", Group: GroupAdvanced,
			Kind: KindCron, Presets: jobSchedulePresets, Default: "0 * * * * *",
			Doc:    "How often Loomarr checks download progress in Sonarr and Radarr.",
			Detail: "A cron schedule. Only used when titles are requested from Sonarr and Radarr directly.",
		},
		{
			Key: "job.seerr_queue_poll.schedule", EnvVar: "JOB_SEERR_QUEUE_POLL_SCHEDULE", Group: GroupAdvanced,
			Kind: KindCron, Presets: jobSchedulePresets, Default: "0 * * * * *",
			Doc:    "How often Loomarr asks Seerr how downloads are going.",
			Detail: "Polls Seerr for coarse acquisition status, on a cron schedule. Only used when titles are requested through Seerr.",
		},
		{
			Key: "job.workers", EnvVar: "JOB_WORKERS", Group: GroupAdvanced,
			Kind: KindInt, Default: 1,
			Doc:    "How many channel suggestions can be worked on at once.",
			Detail: "One is the appliance-safe default; raise it deliberately for larger or hosted-model deployments.",
		},
		{
			Key: "job.timeout", EnvVar: "JOB_TIMEOUT", Group: GroupAdvanced,
			Kind: KindDuration, Default: "10m",
			Doc: "How long one channel suggestion may run before it's stopped.",
		},
		{
			Key: "jobs.retention", EnvVar: "JOBS_RETENTION", Group: GroupAdvanced,
			Kind: KindDuration, Default: "720h",
			Doc: "How long finished suggestion jobs are kept before they're cleaned up.",
		},
		{
			Key: "proposals.retention", EnvVar: "PROPOSALS_RETENTION", Group: GroupAdvanced,
			Kind: KindDuration, Default: "2160h",
			Doc: "How long suggested lineups are kept before they're cleaned up.",
		},
		{
			Key: "activity.retention", EnvVar: "ACTIVITY_RETENTION", Group: GroupAdvanced,
			Kind: KindDuration, Default: "720h",
			Doc: "How long the Dashboard's recent-activity entries are kept before they're cleaned up.",
		},
		{
			Key: "diagnostics.dir", EnvVar: "DIAGNOSTICS_DIR", Group: GroupAdvanced,
			Kind: KindString, Presentation: PresentationPath,
			DataSubdir: "diagnostics", Validate: storagePath(false), Advanced: true,
			Doc:    "Where Loomarr keeps the logs of its video and streaming programs.",
			Detail: "Where Loomarr keeps bounded ffmpeg and streaming-process output. The directory must be persistent if logs should survive a restart. A change applies to the next process; output already written stays where it was.",
		},
		{
			Key: "diagnostics.retention", EnvVar: "DIAGNOSTICS_RETENTION", Group: GroupAdvanced,
			Kind: KindDuration, Default: "168h", Validate: positiveDuration,
			Doc:    "How long diagnostic events and finished process logs are kept.",
			Detail: "How long Diagnostic events and completed Process runs are kept. Active Process runs are never removed by age.",
		},
		{
			Key: "diagnostics.max_storage_mb", EnvVar: "DIAGNOSTICS_MAX_STORAGE_MB", Group: GroupAdvanced,
			Kind: KindInt, Unit: UnitMiB, Default: 512, Validate: positiveWholeNumber,
			Doc:    "How much disk diagnostic events and process logs may use.",
			Detail: "Soft global storage budget in MiB for retained Diagnostic events and bounded Process output. Active Process runs remain protected.",
		},
		{
			Key: "job.housekeeping.schedule", EnvVar: "JOB_HOUSEKEEPING_SCHEDULE", Group: GroupAdvanced,
			Kind: KindCron, Presets: jobSchedulePresets, Default: "0 30 4 * * *",
			Doc: "When Loomarr removes expired sessions and operational records beyond their retention periods.",
		},
		{
			Key: "setup.completed", EnvVar: "SETUP_COMPLETED", Group: GroupAdvanced,
			Kind: KindBool, Default: false, Advanced: true,
			Doc:    "Whether first-run setup is done.",
			Detail: "Until it is, Loomarr opens the setup wizard.",
		},
	}
	for i := range settings {
		if !settings[i].MigrationOnly {
			settings[i].Owner = ownerFor(settings[i])
		}
	}
	return settings
}
