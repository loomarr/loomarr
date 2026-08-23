package programmer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/loomarr/loomarr/internal/httpx"
)

// now is the clock for stamping a new channel's loop anchor (StartTime). A package var so
// a test can freeze it; production is time.Now.
var now = time.Now

// Tunarr is the hand-written thin client for the Tunarr REST API (§6). It targets
// only the endpoints the scheduler needs (channels CRUD + programming); the
// version tested against is Tunarr 1.3.8 (Phase 0). Tunarr ships with no auth
// (Phase-0 finding: empty securitySchemes) and Loomarr talks to it machine-to-
// machine, so there is no API key — just the base URL.
type Tunarr struct {
	// config resolves all hot-applied Tunarr settings together. Each exported
	// operation snapshots it once so a multi-request operation cannot start on one
	// Tunarr instance and finish on another after a concurrent settings change.
	config func() Config
	// resolvedTranscodeID caches the auto-resolved Default for one normalized
	// Tunarr URL so switching instances cannot reuse an endpoint-owned id.
	transcodeMu          sync.Mutex
	resolvedTranscodeID  string
	resolvedTranscodeURL string
	http                 *http.Client
	bulkHTTP             *http.Client     // longer timeout for the content-index bulk read only
	resolver             *contentResolver // media-server item id → Tunarr program uuid (§6)
}

// Config is the complete set of live settings used by the Tunarr adapter. A
// NewDynamic provider is read once at the start of every exported operation.
type Config struct {
	BaseURL               string
	TranscodeConfigID     string
	FillerWeight          int
	FillerCooldownSeconds int
}

type operationConfigKey struct{}

// operation snapshots live config once, or reuses the parent operation's
// snapshot when one exported method delegates to another.
func (t *Tunarr) operation(ctx context.Context) (context.Context, Config) {
	if cfg, ok := ctx.Value(operationConfigKey{}).(Config); ok {
		return ctx, cfg
	}
	cfg := t.config()
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	return context.WithValue(ctx, operationConfigKey{}, cfg), cfg
}

func (t *Tunarr) configFor(ctx context.Context) Config {
	_, cfg := t.operation(ctx)
	return cfg
}

// New builds a Tunarr client. A non-empty transcodeConfigID must be a real config
// id from the target instance; empty auto-resolves its Default (§6 / Phase-0
// finding 3).
func New(baseURL, transcodeConfigID string) *Tunarr {
	base := strings.TrimRight(baseURL, "/")
	return NewDynamic(func() Config {
		return Config{BaseURL: base, TranscodeConfigID: transcodeConfigID}
	})
}

// NewDynamic builds a Tunarr client whose settings resolve once per public
// operation (config-design §3 hot-apply). The composition root passes a closure
// over the settings snapshot.
func NewDynamic(config func() Config) *Tunarr {
	t := &Tunarr{
		config:   config,
		http:     httpx.NewNamed("tunarr", httpx.TimeoutTunarr),
		bulkHTTP: httpx.NewNamed("tunarr-bulk", httpx.TimeoutTunarrBulk),
	}
	t.resolver = &contentResolver{refresh: t.buildContentIndex}
	return t
}

// --- wire types (pinned to the Phase-0 fixtures, not remembered field names) ---

// tunarrChannel is the create/get channel shape. Only the fields we set or read
// are modeled; the create envelope's other required fields are filled with the
// captured minimal-valid defaults (Phase-0 finding 2).
type tunarrChannel struct {
	ID           string      `json:"id"`
	Name         string      `json:"name"`
	Number       int         `json:"number"`
	GroupTitle   string      `json:"groupTitle"`
	Icon         tunarrIcon  `json:"icon"`
	ProgramCount int         `json:"programCount"`
	StreamMode   string      `json:"streamMode"`
	Offline      tunarrOffl  `json:"offline"`
	OnDemand     tunarrOnDem `json:"onDemand"`
	StartTime    int64       `json:"startTime"`
	GuideMinDur  int         `json:"guideMinimumDuration"`
	Stealth      bool        `json:"stealth"`
	Duration     int64       `json:"duration"`
	TranscodeID  string      `json:"transcodeConfigId"`
	FillerColls  []any       `json:"fillerCollections"`
	// Tunarr applies this channel-level programme repeat cooldown in milliseconds. The
	// per-collection cooldown is a different list-selection rule and stays zero for our one list.
	FillerRepeatCooldown int64 `json:"fillerRepeatCooldown"`
	Subtitles            bool  `json:"subtitlesEnabled"`
	DisableFillO         bool  `json:"disableFillerOverlay"`
}

type tunarrIcon struct {
	Path     string `json:"path"`
	Width    int    `json:"width"`
	Duration int    `json:"duration"`
	Position string `json:"position"`
}

type tunarrOffl struct {
	Mode string `json:"mode"`
}

type tunarrOnDem struct {
	Enabled bool `json:"enabled"`
}

// createEnvelope is the oneOf discriminated create body (Phase-0 finding 2):
// {"type":"new","channel":{…}}.
type createEnvelope struct {
	Type    string        `json:"type"`
	Channel tunarrChannel `json:"channel"`
}

// transcodeConfig is a /api/transcode_configs entry — only the fields the resolve
// + validate needs. Tunarr ships one named "Default".
type transcodeConfig struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// TranscodeConfigID resolves the transcode-config uuid every channel create must
// carry (Phase-0 finding 3). A configured id is trusted as-is; otherwise the
// instance's own configs are fetched and the "Default" (or, failing an exact name
// match, the first) is chosen and cached. This is what makes TUNARR_TRANSCODE_
// CONFIG_ID an Advanced knob rather than a required one (§15): an empty setting
// used to send "" and Tunarr rejected every channel with a bare 400 (invalid
// uuid), while the setup check — probing only reachability — stayed green.
//
// Returns an error when the instance reports no configs at all, so the setup check
// can surface an actionable failure instead of letting channel creation die later.
func (t *Tunarr) TranscodeConfigID(ctx context.Context) (string, error) {
	ctx, cfg := t.operation(ctx)
	if id := strings.TrimSpace(cfg.TranscodeConfigID); id != "" {
		return id, nil
	}
	t.transcodeMu.Lock()
	defer t.transcodeMu.Unlock()
	if t.resolvedTranscodeURL == cfg.BaseURL && t.resolvedTranscodeID != "" {
		return t.resolvedTranscodeID, nil
	}
	var configs []transcodeConfig
	if err := t.doJSON(ctx, http.MethodGet, "/api/transcode_configs", nil, &configs); err != nil {
		return "", fmt.Errorf("resolve transcode config: %w", err)
	}
	if len(configs) == 0 {
		return "", fmt.Errorf("Tunarr reports no transcode configs — create one (its UI ships a Default)")
	}
	chosen := configs[0].ID
	for _, c := range configs {
		if strings.EqualFold(c.Name, "Default") {
			chosen = c.ID
			break
		}
	}
	t.resolvedTranscodeID = chosen
	t.resolvedTranscodeURL = cfg.BaseURL
	return chosen, nil
}

// EnsureChannel implements Programmer. On create it reads back the
// server-assigned id (Phase-0 finding 1) and returns it.
func (t *Tunarr) EnsureChannel(ctx context.Context, spec ChannelSpec) (string, error) {
	ctx, _ = t.operation(ctx)
	body := t.channelBody(spec)
	// Resolve the transcode config once, here where we have a ctx (channelBody has
	// none). A create with an empty id is the FINDING 5 dead-end: Tunarr 400s and the
	// channel never appears, while the setup check reads green.
	transcodeID, err := t.TranscodeConfigID(ctx)
	if err != nil {
		return "", fmt.Errorf("channel %d: %w", spec.Number, err)
	}
	body.TranscodeID = transcodeID
	if spec.TunarrID == "" {
		// Create: anchor the looping lineup at NOW (epoch ms). Leaving it 0 anchored the
		// loop at epoch 0 (1970), which surfaced in the guide as a ~1960 start.
		body.StartTime = now().UnixMilli()
		// POST /api/channels with the {type:"new",channel:{…}} envelope.
		env := createEnvelope{Type: "new", Channel: body}
		var created tunarrChannel
		if err := t.doJSON(ctx, http.MethodPost, "/api/channels", env, &created); err != nil {
			return "", fmt.Errorf("create channel %d: %w", spec.Number, err)
		}
		if created.ID == "" {
			return "", fmt.Errorf("create channel %d: server returned no id", spec.Number)
		}
		return created.ID, nil
	}
	// Update: PUT /api/channels/{id} with the full channel (id preserved). Preserve the
	// EXISTING loop anchor the caller threaded (spec.StartTime) so the lineup doesn't jump
	// back to its start every reconcile. If the caller didn't supply one (0 — e.g. a spec
	// built without a read-back), fall back to stamping now rather than re-writing epoch 0.
	body.ID = spec.TunarrID
	if spec.StartTime > 0 {
		body.StartTime = spec.StartTime // preserve a valid anchor (stable loop across reconciles)
	} else {
		// 0 means either no read-back OR a channel created before this fix (the broken
		// epoch-0 anchor). Either way, stamp NOW to HEAL it — a channel's anchor should never
		// be epoch 0, so re-anchoring to now on the next reconcile fixes pre-fix channels
		// without a manual recreate. (Costs one loop-seam jump, once, on the healing sweep.)
		body.StartTime = now().UnixMilli()
	}
	if err := t.doJSON(ctx, http.MethodPut, "/api/channels/"+spec.TunarrID, body, nil); err != nil {
		return "", fmt.Errorf("update channel %s: %w", spec.TunarrID, err)
	}
	return spec.TunarrID, nil
}

// channelBody fills the minimal-valid create/update channel from a spec plus the
// captured defaults (Phase-0 finding 2). TranscodeID is set by EnsureChannel (which
// has the ctx to resolve it), not here.
func (t *Tunarr) channelBody(spec ChannelSpec) tunarrChannel {
	return tunarrChannel{
		Name:        spec.Name,
		Number:      spec.Number,
		GroupTitle:  spec.Group,
		Icon:        tunarrIcon{Path: spec.Logo, Position: "bottom-right"},
		StreamMode:  "hls", // Phase-0 finding 5 enum; hls is the safe default
		Offline:     tunarrOffl{Mode: "pic"},
		OnDemand:    tunarrOnDem{Enabled: false},
		StartTime:   0,
		GuideMinDur: 300,
		FillerColls: []any{},
	}
}

// GetChannel implements Programmer. A 404 → (zero, false, nil).
func (t *Tunarr) GetChannel(ctx context.Context, tunarrID string) (ActualChannel, bool, error) {
	ctx, _ = t.operation(ctx)
	var ch tunarrChannel
	status, snippet, err := t.doStatus(ctx, t.http, http.MethodGet, "/api/channels/"+tunarrID, nil, &ch)
	if err != nil {
		return ActualChannel{}, false, err
	}
	if status == http.StatusNotFound {
		return ActualChannel{}, false, nil
	}
	if status < 200 || status >= 300 {
		return ActualChannel{}, false, statusErr("get channel "+tunarrID, status, snippet)
	}
	return ActualChannel{
		TunarrID:     ch.ID,
		Number:       ch.Number,
		Name:         ch.Name,
		Group:        ch.GroupTitle,
		Logo:         ch.Icon.Path,
		ProgramCount: ch.ProgramCount,
		StartTime:    ch.StartTime, // preserve the loop anchor across updates
	}, true, nil
}

// ListChannels implements Programmer: every channel Tunarr currently has, Loomarr-owned or not.
//
// ⚠ **It exists so Loomarr can pick a channel number that is free on BOTH sides** (§9 V54).
// `nextFreeChannelNumber` consulted only Loomarr's own store, which knows nothing about channels
// created by an earlier install, a reset database, or the operator by hand. Picking a number Tunarr
// already uses makes the create fail — and Tunarr reports that collision as `500` with an EMPTY
// BODY, so the symptom was an opaque, permanent failure rather than "that number is taken".
func (t *Tunarr) ListChannels(ctx context.Context) ([]ActualChannel, error) {
	ctx, _ = t.operation(ctx)
	var raw []tunarrChannel
	status, snippet, err := t.doStatus(ctx, t.http, http.MethodGet, "/api/channels", nil, &raw)
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return nil, statusErr("list channels", status, snippet)
	}
	out := make([]ActualChannel, 0, len(raw))
	for _, ch := range raw {
		out = append(out, ActualChannel{
			TunarrID: ch.ID, Number: ch.Number, Name: ch.Name, Group: ch.GroupTitle,
			Logo: ch.Icon.Path, ProgramCount: ch.ProgramCount, StartTime: ch.StartTime,
		})
	}
	return out, nil
}

// DeleteChannel implements Programmer; a 404 is treated as already-gone (idempotent).
func (t *Tunarr) DeleteChannel(ctx context.Context, tunarrID string) error {
	ctx, _ = t.operation(ctx)
	status, snippet, err := t.doStatus(ctx, t.http, http.MethodDelete, "/api/channels/"+tunarrID, nil, nil)
	if err != nil {
		return err
	}
	if status == http.StatusNotFound || (status >= 200 && status < 300) {
		return nil
	}
	return statusErr("delete channel "+tunarrID, status, snippet)
}

// --- HTTP helpers ---

func (t *Tunarr) doJSON(ctx context.Context, method, path string, in, out any) error {
	return t.doJSONClient(ctx, t.http, method, path, in, out)
}

// doJSONBulk is doJSON on the longer-timeout bulk client — for the one large read
// (the content-index build) that a channel-CRUD timeout would cancel (§6).
func (t *Tunarr) doJSONBulk(ctx context.Context, method, path string, in, out any) error {
	return t.doJSONClient(ctx, t.bulkHTTP, method, path, in, out)
}

func (t *Tunarr) doJSONClient(ctx context.Context, client *http.Client, method, path string, in, out any) error {
	status, snippet, err := t.doStatus(ctx, client, method, path, in, out)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return statusErr(method+" "+path, status, snippet)
	}
	return nil
}

// statusErr formats a non-2xx Tunarr response, appending the body snippet when
// present so the error carries Tunarr's own reason (a 400's "{"error":"..."}")
// rather than a bare status code — the difference between "can't reach Tunarr,
// status 400" and knowing exactly which field the request got wrong.
func statusErr(what string, status int, snippet string) error {
	if snippet != "" {
		return fmt.Errorf("%s: status %d: %s", what, status, snippet)
	}
	return fmt.Errorf("%s: status %d", what, status)
}

// doStatus executes a request and returns the HTTP status; it decodes into out
// only on a 2xx (so callers can special-case 404 without a decode error). in is
// JSON-marshaled when non-nil. On a non-2xx it returns a bounded snippet of the
// response body so callers can surface Tunarr's own error reason (e.g. a 400's
// "{"error":"..."}") instead of a bare status code.
func (t *Tunarr) doStatus(ctx context.Context, client *http.Client, method, path string, in, out any) (int, string, error) {
	var reader *strings.Reader
	if in != nil {
		blob, err := json.Marshal(in)
		if err != nil {
			return 0, "", fmt.Errorf("marshal %s body: %w", path, err)
		}
		reader = strings.NewReader(string(blob))
	}
	var req *http.Request
	var err error
	if reader != nil {
		req, err = http.NewRequestWithContext(ctx, method, t.configFor(ctx).BaseURL+path, reader)
	} else {
		req, err = http.NewRequestWithContext(ctx, method, t.configFor(ctx).BaseURL+path, nil)
	}
	if err != nil {
		return 0, "", err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, "", fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Read a bounded slice of the error body; Tunarr replies with a small JSON
		// object, and we never want a runaway body in an error string.
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return resp.StatusCode, strings.TrimSpace(string(snippet)), nil
	}
	if out != nil {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return resp.StatusCode, "", fmt.Errorf("decode %s: %w", path, err)
		}
	}
	return resp.StatusCode, "", nil
}

var _ Programmer = (*Tunarr)(nil)
