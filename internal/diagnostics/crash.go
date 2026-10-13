package diagnostics

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/time/rate"
)

// Client crash reports are the one client surface that accepts a message and a stack (#1816). A
// paired app sends them after it crashes; admins read them back. They stay inside Loomarr: no
// third-party service or SDK ever sees them.
const (
	// ClientCrashRetention and ClientCrashRetainedMax are the fixed retention policy, applied by
	// the nightly housekeeping pass: a report is removed after 30 days or once 1000 newer ones
	// exist, whichever comes first.
	ClientCrashRetention   = 30 * 24 * time.Hour
	ClientCrashRetainedMax = 1000

	// ClientCrashMaxBodyBytes is the request-body fence; a larger body is refused with 413.
	ClientCrashMaxBodyBytes = 128 << 10

	maxCrashMessageBytes    = 2 << 10
	maxCrashStackBytes      = 32 << 10
	maxCrashBreadcrumbs     = 50
	maxCrashBreadcrumbBytes = 256
	maxCrashNativeBytes     = 64 << 10
	maxCrashLabelBytes      = 128
	maxCrashPage            = 100
	crashReportsPerMinute   = 2
	crashReportBurst        = 10
	// A person's devices together: 30 an hour after a burst of 30.
	crashReportsPerPersonHour = 30
	crashReportPersonBurst    = 30
)

var (
	ErrInvalidClientCrash      = errors.New("invalid client crash report")
	ErrClientCrashRateLimited  = errors.New("client crash reports rate limited")
	ErrClientCrashNotFound     = errors.New("client crash report not found")
	ErrInvalidClientCrashQuery = errors.New("invalid client crash report query")
)

// CrashKind says what crashed: a fatal JavaScript error, a caught one, or the native process.
type CrashKind string

const (
	CrashJSFatal CrashKind = "js-fatal"
	CrashJSError CrashKind = "js-error"
	CrashNative  CrashKind = "native"
)

// CrashPlatform is the app build that sent the report.
type CrashPlatform string

const (
	CrashIOS       CrashPlatform = "ios"
	CrashAndroid   CrashPlatform = "android"
	CrashTVOS      CrashPlatform = "tvos"
	CrashAndroidTV CrashPlatform = "android-tv"
)

// BreadcrumbCategory is the closed set of things an app remembers before a crash.
type BreadcrumbCategory string

const (
	BreadcrumbNavigation BreadcrumbCategory = "navigation"
	BreadcrumbHTTP       BreadcrumbCategory = "http"
	BreadcrumbLifecycle  BreadcrumbCategory = "lifecycle"
	BreadcrumbLog        BreadcrumbCategory = "log"
)

// ClientCrashReportInput is the complete wire body. Who sent it (user, device) is deliberately
// absent: the server takes both from the authenticated request.
//
// The tags document the contract for OpenAPI and the generated client; the route skips Huma's
// schema validation so that every violation, enums included, is the module's 400 rather than a
// framework 422. ValidateClientCrash is the enforcement.
type ClientCrashReportInput struct {
	Kind          CrashKind               `json:"kind" enum:"js-fatal,js-error,native"`
	Platform      CrashPlatform           `json:"platform" enum:"ios,android,tvos,android-tv"`
	AppVersion    string                  `json:"appVersion" maxLength:"128"`
	AppBuild      string                  `json:"appBuild" maxLength:"128"`
	OSVersion     string                  `json:"osVersion" maxLength:"128"`
	DeviceModel   string                  `json:"deviceModel" maxLength:"128"`
	OccurredAt    string                  `json:"occurredAt" format:"date-time" doc:"RFC 3339, the device's clock"`
	Message       string                  `json:"message" minLength:"1" maxLength:"2048" doc:"At most 2 KiB of UTF-8"`
	Stack         string                  `json:"stack,omitempty" maxLength:"32768" doc:"At most 32 KiB of UTF-8"`
	Breadcrumbs   []ClientCrashBreadcrumb `json:"breadcrumbs" maxItems:"50"`
	NativePayload json.RawMessage         `json:"nativePayload,omitempty" doc:"kind=native only: the MetricKit MXDiagnosticPayload JSON object, at most 64 KiB"`
}

// ClientCrashBreadcrumb is one thing the app did before it crashed.
type ClientCrashBreadcrumb struct {
	At       string             `json:"at" format:"date-time" doc:"RFC 3339"`
	Category BreadcrumbCategory `json:"category" enum:"navigation,http,lifecycle,log"`
	Message  string             `json:"message" maxLength:"256" doc:"At most 256 bytes of UTF-8; URLs keep their path only"`
}

// UnmarshalJSON fails closed on unknown fields, so a client cannot smuggle headers or cookies
// beside the declared ones.
func (in *ClientCrashReportInput) UnmarshalJSON(data []byte) error {
	type wire ClientCrashReportInput
	return decodeClosedJSON(data, (*wire)(in))
}

func (b *ClientCrashBreadcrumb) UnmarshalJSON(data []byte) error {
	type wire ClientCrashBreadcrumb
	return decodeClosedJSON(data, (*wire)(b))
}

// ClientCrashReporter is who sent a report, as the server authenticated it. Every field may be
// empty: an API-token caller has no user, and a browser session has no paired device.
type ClientCrashReporter struct {
	UserID     string
	DeviceID   string
	DeviceName string
}

// ClientCrashRecord is the stored row. Times are Unix milliseconds. A page read leaves Stack,
// BreadcrumbsJSON and NativePayload empty; only a single-report read loads them.
type ClientCrashRecord struct {
	ID              string
	ReceivedAt      int64
	OccurredAt      int64
	UserID          string
	DeviceID        string
	DeviceName      string
	Kind            CrashKind
	Platform        CrashPlatform
	AppVersion      string
	AppBuild        string
	OSVersion       string
	DeviceModel     string
	Message         string
	Stack           string
	BreadcrumbsJSON string
	NativePayload   string
}

// ClientCrashStoreQuery is one newest-first page. A zero CursorReceivedAt means the first page.
type ClientCrashStoreQuery struct {
	Limit            int
	CursorReceivedAt int64
	CursorID         string
}

// ClientCrashStore is the persistence role the crash-report module needs.
type ClientCrashStore interface {
	InsertClientCrashReport(context.Context, ClientCrashRecord) error
	QueryClientCrashReports(context.Context, ClientCrashStoreQuery) ([]ClientCrashRecord, error)
	GetClientCrashReport(context.Context, string) (ClientCrashRecord, bool, error)
}

// ClientCrashSummary is one row of the admin list: everything but the stack, breadcrumbs and
// native payload, which only the single-report read returns.
type ClientCrashSummary struct {
	ID          string        `json:"id"`
	ReceivedAt  string        `json:"receivedAt" format:"date-time" doc:"RFC 3339, the server's clock"`
	OccurredAt  string        `json:"occurredAt" format:"date-time" doc:"RFC 3339, the device's clock"`
	UserID      string        `json:"userId,omitempty" doc:"The signed-in person whose app sent it; absent for an API-token caller"`
	DeviceID    string        `json:"deviceId,omitempty" doc:"The paired device's id from the device list; absent for a browser session"`
	DeviceName  string        `json:"deviceName,omitempty"`
	Kind        CrashKind     `json:"kind" enum:"js-fatal,js-error,native"`
	Platform    CrashPlatform `json:"platform" enum:"ios,android,tvos,android-tv"`
	AppVersion  string        `json:"appVersion"`
	AppBuild    string        `json:"appBuild"`
	OSVersion   string        `json:"osVersion"`
	DeviceModel string        `json:"deviceModel"`
	Message     string        `json:"message"`
}

// ClientCrashReport is one report in full.
type ClientCrashReport struct {
	ClientCrashSummary
	Stack         string                  `json:"stack,omitempty"`
	Breadcrumbs   []ClientCrashBreadcrumb `json:"breadcrumbs"`
	NativePayload json.RawMessage         `json:"nativePayload,omitempty"`
}

// ClientCrashPage is one newest-first page of summaries.
type ClientCrashPage struct {
	Items      []ClientCrashSummary `json:"items"`
	NextCursor string               `json:"nextCursor,omitempty" doc:"Pass as cursor for the next older page; absent on the last page"`
}

// ClientCrashes owns the crash-report contract: validation, scrubbing, per-device rate limiting,
// and the admin read projection.
type ClientCrashes struct {
	store ClientCrashStore
	now   func() time.Time

	mu       sync.Mutex
	limiters map[string]*rate.Limiter
}

func NewClientCrashes(store ClientCrashStore, now func() time.Time) *ClientCrashes {
	if now == nil {
		now = time.Now
	}
	return &ClientCrashes{store: store, now: now, limiters: make(map[string]*rate.Limiter)}
}

// Report validates, scrubs and stores one report and returns its id. A validation or rate error
// stores nothing.
func (c *ClientCrashes) Report(ctx context.Context, from ClientCrashReporter, in ClientCrashReportInput) (string, error) {
	now := c.now()
	record, err := validateClientCrash(in)
	if err != nil {
		return "", err
	}
	if !c.allow(from, now) {
		return "", ErrClientCrashRateLimited
	}
	record.ID = newCrashID(now)
	record.ReceivedAt = now.UnixMilli()
	record.UserID = truncate(from.UserID, maxCrashLabelBytes)
	record.DeviceID = truncate(from.DeviceID, maxCrashLabelBytes)
	record.DeviceName = truncate(from.DeviceName, maxCrashLabelBytes)
	if err := c.store.InsertClientCrashReport(ctx, record); err != nil {
		return "", fmt.Errorf("store client crash report: %w", err)
	}
	return record.ID, nil
}

// rateKey charges a paired device by its own id, so one crashing phone cannot starve another
// device of the same person. A browser session falls back to the person, an API token to itself.
func rateKey(from ClientCrashReporter) string {
	switch {
	case from.DeviceID != "":
		return "device:" + from.DeviceID
	case from.UserID != "":
		return "user:" + from.UserID
	default:
		return "api_token"
	}
}

// allow charges the sender's own bucket and, for a signed-in person, the person's aggregate
// bucket. Without the aggregate, one member's devices together could fill the 1000-report cap
// and evict everyone else's reports. A report is charged to both or to neither.
func (c *ClientCrashes) allow(from ClientCrashReporter, now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	limiters := []*rate.Limiter{c.limiter(rateKey(from), rate.Every(time.Minute/crashReportsPerMinute), crashReportBurst)}
	if from.UserID != "" {
		limiters = append(limiters, c.limiter("person:"+from.UserID,
			rate.Every(time.Hour/crashReportsPerPersonHour), crashReportPersonBurst))
	}
	reservations := make([]*rate.Reservation, 0, len(limiters))
	for _, limiter := range limiters {
		reservation := limiter.ReserveN(now, 1)
		reservations = append(reservations, reservation)
		if !reservation.OK() || reservation.DelayFrom(now) > 0 {
			for _, taken := range reservations {
				taken.CancelAt(now)
			}
			return false
		}
	}
	return true
}

func (c *ClientCrashes) limiter(key string, every rate.Limit, burst int) *rate.Limiter {
	limiter := c.limiters[key]
	if limiter == nil {
		limiter = rate.NewLimiter(every, burst)
		c.limiters[key] = limiter
	}
	return limiter
}

// List returns one newest-first page of summaries.
func (c *ClientCrashes) List(ctx context.Context, limit int, cursor string) (ClientCrashPage, error) {
	if limit < 1 || limit > maxCrashPage {
		return ClientCrashPage{}, fmt.Errorf("%w: limit must be 1-%d", ErrInvalidClientCrashQuery, maxCrashPage)
	}
	query := ClientCrashStoreQuery{Limit: limit + 1}
	if cursor != "" {
		decoded, err := decodeCrashCursor(cursor)
		if err != nil {
			return ClientCrashPage{}, fmt.Errorf("%w: cursor is invalid", ErrInvalidClientCrashQuery)
		}
		query.CursorReceivedAt, query.CursorID = decoded.ReceivedAt, decoded.ID
	}
	records, err := c.store.QueryClientCrashReports(ctx, query)
	if err != nil {
		return ClientCrashPage{}, err
	}
	page := ClientCrashPage{Items: make([]ClientCrashSummary, 0, min(len(records), limit))}
	if len(records) > limit {
		records = records[:limit]
		last := records[len(records)-1]
		page.NextCursor = encodeCrashCursor(crashCursor{ReceivedAt: last.ReceivedAt, ID: last.ID})
	}
	for _, record := range records {
		page.Items = append(page.Items, crashSummary(record))
	}
	return page, nil
}

// Get returns one report in full.
func (c *ClientCrashes) Get(ctx context.Context, id string) (ClientCrashReport, error) {
	record, ok, err := c.store.GetClientCrashReport(ctx, id)
	if err != nil {
		return ClientCrashReport{}, err
	}
	if !ok {
		return ClientCrashReport{}, ErrClientCrashNotFound
	}
	report := ClientCrashReport{ClientCrashSummary: crashSummary(record), Stack: record.Stack,
		Breadcrumbs: []ClientCrashBreadcrumb{}}
	if record.BreadcrumbsJSON != "" {
		if err := json.Unmarshal([]byte(record.BreadcrumbsJSON), &report.Breadcrumbs); err != nil {
			return ClientCrashReport{}, fmt.Errorf("decode crash breadcrumbs %s: %w", id, err)
		}
	}
	if record.NativePayload != "" {
		report.NativePayload = json.RawMessage(record.NativePayload)
	}
	return report, nil
}

func crashSummary(record ClientCrashRecord) ClientCrashSummary {
	return ClientCrashSummary{
		ID: record.ID, ReceivedAt: rfc3339(record.ReceivedAt), OccurredAt: rfc3339(record.OccurredAt),
		UserID: record.UserID, DeviceID: record.DeviceID, DeviceName: record.DeviceName,
		Kind: record.Kind, Platform: record.Platform, AppVersion: record.AppVersion,
		AppBuild: record.AppBuild, OSVersion: record.OSVersion, DeviceModel: record.DeviceModel,
		Message: record.Message,
	}
}

func rfc3339(ms int64) string {
	return time.UnixMilli(ms).UTC().Format(time.RFC3339Nano)
}

// validateClientCrash enforces the contract on the raw input, then scrubs. Limits are measured
// in bytes before scrubbing; a redaction marker can lengthen a field, so the scrubbed value is
// truncated back to the same limit.
func validateClientCrash(in ClientCrashReportInput) (ClientCrashRecord, error) {
	switch in.Kind {
	case CrashJSFatal, CrashJSError, CrashNative:
	default:
		return ClientCrashRecord{}, invalidClientCrash("kind must be js-fatal, js-error or native")
	}
	switch in.Platform {
	case CrashIOS, CrashAndroid, CrashTVOS, CrashAndroidTV:
	default:
		return ClientCrashRecord{}, invalidClientCrash("platform must be ios, android, tvos or android-tv")
	}
	for _, label := range [...]struct{ name, value string }{{"appVersion", in.AppVersion},
		{"appBuild", in.AppBuild}, {"osVersion", in.OSVersion}, {"deviceModel", in.DeviceModel}} {
		if err := crashText(label.name, label.value, maxCrashLabelBytes); err != nil {
			return ClientCrashRecord{}, err
		}
	}
	occurred, err := crashTime("occurredAt", in.OccurredAt)
	if err != nil {
		return ClientCrashRecord{}, err
	}
	if strings.TrimSpace(in.Message) == "" {
		return ClientCrashRecord{}, invalidClientCrash("message is required")
	}
	if err := crashText("message", in.Message, maxCrashMessageBytes); err != nil {
		return ClientCrashRecord{}, err
	}
	if err := crashText("stack", in.Stack, maxCrashStackBytes); err != nil {
		return ClientCrashRecord{}, err
	}
	if len(in.Breadcrumbs) > maxCrashBreadcrumbs {
		return ClientCrashRecord{}, invalidClientCrash(fmt.Sprintf("at most %d breadcrumbs", maxCrashBreadcrumbs))
	}
	breadcrumbs := make([]ClientCrashBreadcrumb, 0, len(in.Breadcrumbs))
	for i, crumb := range in.Breadcrumbs {
		at, err := crashTime(fmt.Sprintf("breadcrumbs[%d].at", i), crumb.At)
		if err != nil {
			return ClientCrashRecord{}, err
		}
		switch crumb.Category {
		case BreadcrumbNavigation, BreadcrumbHTTP, BreadcrumbLifecycle, BreadcrumbLog:
		default:
			return ClientCrashRecord{}, invalidClientCrash(fmt.Sprintf(
				"breadcrumbs[%d].category must be navigation, http, lifecycle or log", i))
		}
		if err := crashText(fmt.Sprintf("breadcrumbs[%d].message", i), crumb.Message, maxCrashBreadcrumbBytes); err != nil {
			return ClientCrashRecord{}, err
		}
		breadcrumbs = append(breadcrumbs, ClientCrashBreadcrumb{
			At: at.UTC().Format(time.RFC3339Nano), Category: crumb.Category,
			Message: scrubCrashText(crumb.Message, maxCrashBreadcrumbBytes),
		})
	}
	native, err := nativeCrashPayload(in.Kind, in.NativePayload)
	if err != nil {
		return ClientCrashRecord{}, err
	}
	breadcrumbsJSON, err := json.Marshal(breadcrumbs)
	if err != nil {
		return ClientCrashRecord{}, fmt.Errorf("encode crash breadcrumbs: %w", err)
	}
	return ClientCrashRecord{
		OccurredAt: occurred.UnixMilli(), Kind: in.Kind, Platform: in.Platform,
		AppVersion:      scrubCrashText(in.AppVersion, maxCrashLabelBytes),
		AppBuild:        scrubCrashText(in.AppBuild, maxCrashLabelBytes),
		OSVersion:       scrubCrashText(in.OSVersion, maxCrashLabelBytes),
		DeviceModel:     scrubCrashText(in.DeviceModel, maxCrashLabelBytes),
		Message:         scrubCrashText(in.Message, maxCrashMessageBytes),
		Stack:           scrubCrashText(in.Stack, maxCrashStackBytes),
		BreadcrumbsJSON: string(breadcrumbsJSON), NativePayload: native,
	}, nil
}

func crashText(name, value string, limit int) error {
	if !utf8.ValidString(value) {
		return invalidClientCrash(name + " must be UTF-8")
	}
	if len(value) > limit {
		return invalidClientCrash(fmt.Sprintf("%s exceeds %d bytes", name, limit))
	}
	return nil
}

func crashTime(name, value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}, invalidClientCrash(name + " must be an RFC 3339 time")
	}
	return parsed, nil
}

// nativeCrashPayload accepts the MetricKit payload only on a native crash, only as a JSON object,
// and scrubs every string inside it. Walking the decoded value rather than regex-replacing the
// raw text keeps the stored payload valid JSON.
func nativeCrashPayload(kind CrashKind, raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	if kind != CrashNative {
		return "", invalidClientCrash("nativePayload is only accepted on a native crash")
	}
	if len(raw) > maxCrashNativeBytes {
		return "", invalidClientCrash(fmt.Sprintf("nativePayload exceeds %d bytes", maxCrashNativeBytes))
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", invalidClientCrash("nativePayload must be a JSON object")
	}
	// No HTML escaping: it would turn every `<`, `>` and `&` into six bytes and let the stored
	// payload outgrow the limit its input was checked against.
	var scrubbed strings.Builder
	encoder := json.NewEncoder(&scrubbed)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(scrubCrashJSON("", payload)); err != nil {
		return "", fmt.Errorf("encode native crash payload: %w", err)
	}
	return strings.TrimSuffix(scrubbed.String(), "\n"), nil
}

func scrubCrashJSON(key string, value any) any {
	if crashSensitiveKey(key) {
		return redacted
	}
	switch typed := value.(type) {
	case string:
		return scrubCrashText(typed, maxCrashNativeBytes)
	case map[string]any:
		for nested, item := range typed {
			typed[nested] = scrubCrashJSON(nested, item)
		}
		return typed
	case []any:
		for i, item := range typed {
			typed[i] = scrubCrashJSON("", item)
		}
		return typed
	default:
		return typed
	}
}

// crashSensitiveKey widens the recorder's exact-name list to any key naming a token, cookie,
// session or credential header ("X-Device-Token", "csrfToken", "loomarr_session").
func crashSensitiveKey(key string) bool {
	lower := strings.ToLower(key)
	return sensitiveKey(key) || strings.Contains(lower, "token") || strings.Contains(lower, "cookie") ||
		strings.Contains(lower, "session") || strings.Contains(lower, "authorization")
}

// urlQueryPattern removes a query or fragment from any run of non-space text: an absolute URL, a
// path, or a bare relative URI such as an HLS segment ("seg00012.ts?sig=…"). Signed play URLs
// carry their token there, and segment URIs inherit it. A query must look like `key=` so prose
// ("did this fail? no") survives; the removal runs to the next space or quote, never stopping
// at a bracket that might sit inside the signature.
var urlQueryPattern = regexp.MustCompile(`([^\s'"<>?#]+)(?:\?[^\s'"<>=&#?]+=|#)[^\s'"<>]*`)

// encodedQueryPattern is the same cut for a percent-encoded URL ("…%3Ftoken%3D…").
var encodedQueryPattern = regexp.MustCompile(`([^\s'"<>%]+)%3[Ff][^\s'"<>]*`)

// crashBearerPattern is bearerValuePattern that also accepts a non-breaking space.
var crashBearerPattern = regexp.MustCompile(`(?i)(bearer[\s\x{00a0}]+)[^\s\x{00a0},;'"]+`)

// credentialHeaderPattern drops the rest of a credential header line, however it is quoted. The
// key=value patterns below would stop at the first `;` and leave the rest of a multi-cookie
// header behind.
var credentialHeaderPattern = regexp.MustCompile(
	`(?im)((?:^|[\s"'{,\[(])(?:proxy-)?(?:authorization|cookie|set-cookie)["']?\s*:)[^\r\n]*`)

// crashSecretKeys names a value that is a credential wherever it appears: as a header, a query
// pair, a JSON or JS object key (quoted or not), or an assignment.
const crashSecretKeys = `(?i)((?:proxy[-_]?)?authorization|(?:set[-_]?)?cookie|[a-z0-9_-]*token[a-z0-9_-]*|` +
	`[a-z0-9_-]*session[a-z0-9_-]*|bearer|api[-_]?key|password|passwd|secret|signature|sig)(["']?\s*[=:]\s*)`

var (
	crashSecretDoubleQuoted = regexp.MustCompile(crashSecretKeys + `"[^"]*"`)
	crashSecretSingleQuoted = regexp.MustCompile(crashSecretKeys + `'[^']*'`)
	// An unquoted value takes its authorization scheme with it, so "Basic dXNl…" loses both words.
	crashSecretBare = regexp.MustCompile(crashSecretKeys + `(?:(?i:basic|bearer|digest|token)[\s\x{00a0}]+)?[^\s,;&'"]+`)
)

// scrubCrashText is the server-side half of the crash privacy contract: no credential, header
// value, cookie or URL query survives into a stored report, whatever the client sent. NUL is
// dropped too: Postgres TEXT refuses it, and a refused insert would lose the report.
func scrubCrashText(value string, limit int) string {
	value = strings.ReplaceAll(value, "\x00", "")
	value = credentialHeaderPattern.ReplaceAllString(value, `${1} [redacted]`)
	value = urlCredentialPattern.ReplaceAllString(value, `${1}[redacted]@`)
	value = urlQueryPattern.ReplaceAllString(value, `${1}`)
	value = encodedQueryPattern.ReplaceAllString(value, `${1}`)
	value = crashBearerPattern.ReplaceAllString(value, `${1}[redacted]`)
	value = crashSecretDoubleQuoted.ReplaceAllString(value, `${1}${2}"[redacted]"`)
	value = crashSecretSingleQuoted.ReplaceAllString(value, `${1}${2}'[redacted]'`)
	value = crashSecretBare.ReplaceAllString(value, `${1}${2}[redacted]`)
	return truncate(value, limit)
}

func invalidClientCrash(detail string) error {
	return fmt.Errorf("%w: %s", ErrInvalidClientCrash, detail)
}

func newCrashID(now time.Time) string {
	return "crash_" + strings.TrimPrefix(newID(now), "diag_")
}

type crashCursor struct {
	ReceivedAt int64  `json:"receivedAt"`
	ID         string `json:"id"`
}

func encodeCrashCursor(cursor crashCursor) string {
	encoded, _ := json.Marshal(cursor)
	return base64.RawURLEncoding.EncodeToString(encoded)
}

func decodeCrashCursor(value string) (crashCursor, error) {
	if len(value) > maxCursorBytes {
		return crashCursor{}, ErrInvalidClientCrashQuery
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return crashCursor{}, err
	}
	var cursor crashCursor
	if err := json.Unmarshal(decoded, &cursor); err != nil {
		return crashCursor{}, err
	}
	if cursor.ReceivedAt <= 0 || cursor.ID == "" || len(cursor.ID) > maxCrashLabelBytes {
		return crashCursor{}, ErrInvalidClientCrashQuery
	}
	return cursor, nil
}
