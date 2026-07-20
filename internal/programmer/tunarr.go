package programmer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/mantonx/loomarr/internal/httpx"
)

// Tunarr is the hand-written thin client for the Tunarr REST API (§6). It targets
// only the endpoints the scheduler needs (channels CRUD + programming); the
// version tested against is Tunarr 1.3.8 (Phase 0). TUNARR_API_KEY is optional
// for 1.3.8 (Phase-0 finding: empty securitySchemes) — if set, it's sent as a
// bearer token so a future key-requiring version works without code change.
type Tunarr struct {
	// conn resolves base URL + optional api key per request (config-design §3
	// hot-apply): a Tunarr URL saved in Settings takes effect on the next push.
	conn func() (baseURL, apiKey string)
	// transcodeConfigID references a real Tunarr transcode config (Phase-0
	// finding 3: channel create requires a valid uuid; the instance ships a
	// "Default"). EMPTY is the common case (§15: TUNARR_TRANSCODE_CONFIG_ID is an
	// Advanced knob most operators never touch) and means "resolve the instance
	// Default" — see transcodeID. A household admin should not have to hunt a uuid
	// out of Tunarr to get a channel on air.
	transcodeConfigID string
	// resolvedTranscodeID caches the auto-resolved Default so the resolve query
	// runs once, not per channel create.
	transcodeMu         sync.Mutex
	resolvedTranscodeID string
	http                *http.Client
	resolver            *contentResolver // media-server item id → Tunarr program uuid (§6)
	// filler-list attach policy (§10/§15): weight = relative draw when a channel has
	// multiple lists; cooldownSeconds = min seconds before a clip repeats. Zero
	// values fall back to sane defaults in attachFillerList.
	fillerWeight   int
	fillerCooldown int
}

// WithFillerPolicy sets the filler-list attach knobs (FILLER_WEIGHT /
// FILLER_COOLDOWN_SECONDS, §15). Returns the client for chaining.
func (t *Tunarr) WithFillerPolicy(weight, cooldownSeconds int) *Tunarr {
	t.fillerWeight = weight
	t.fillerCooldown = cooldownSeconds
	return t
}

// New builds a Tunarr client. transcodeConfigID must be a real config id from
// the target instance (§6 / Phase-0 finding 3).
func New(baseURL, apiKey, transcodeConfigID string) *Tunarr {
	base := strings.TrimRight(baseURL, "/")
	return NewDynamic(func() (string, string) { return base, apiKey }, transcodeConfigID)
}

// NewDynamic builds a Tunarr client whose base URL + api key resolve per request
// via conn (config-design §3 hot-apply). The composition root passes a closure
// over the settings snapshot.
func NewDynamic(conn func() (baseURL, apiKey string), transcodeConfigID string) *Tunarr {
	t := &Tunarr{
		conn:              conn,
		transcodeConfigID: transcodeConfigID,
		http:              httpx.NewNamed("tunarr", httpx.TimeoutTunarr),
	}
	t.resolver = &contentResolver{refresh: t.buildContentIndex}
	return t
}

func (t *Tunarr) baseURL() string { u, _ := t.conn(); return strings.TrimRight(u, "/") }
func (t *Tunarr) apiKey() string  { _, k := t.conn(); return k }

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
	Subtitles    bool        `json:"subtitlesEnabled"`
	DisableFillO bool        `json:"disableFillerOverlay"`
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
	if id := strings.TrimSpace(t.transcodeConfigID); id != "" {
		return id, nil
	}
	t.transcodeMu.Lock()
	defer t.transcodeMu.Unlock()
	if t.resolvedTranscodeID != "" {
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
	return chosen, nil
}

// EnsureChannel implements Programmer. On create it reads back the
// server-assigned id (Phase-0 finding 1) and returns it.
func (t *Tunarr) EnsureChannel(ctx context.Context, spec ChannelSpec) (string, error) {
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
		// Create: POST /api/channels with the {type:"new",channel:{…}} envelope.
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
	// Update: PUT /api/channels/{id} with the full channel (id preserved).
	body.ID = spec.TunarrID
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
	var ch tunarrChannel
	status, err := t.doStatus(ctx, http.MethodGet, "/api/channels/"+tunarrID, nil, &ch)
	if err != nil {
		return ActualChannel{}, false, err
	}
	if status == http.StatusNotFound {
		return ActualChannel{}, false, nil
	}
	if status < 200 || status >= 300 {
		return ActualChannel{}, false, fmt.Errorf("get channel %s: status %d", tunarrID, status)
	}
	return ActualChannel{
		TunarrID:     ch.ID,
		Number:       ch.Number,
		Name:         ch.Name,
		Group:        ch.GroupTitle,
		Logo:         ch.Icon.Path,
		ProgramCount: ch.ProgramCount,
	}, true, nil
}

// DeleteChannel implements Programmer; a 404 is treated as already-gone (idempotent).
func (t *Tunarr) DeleteChannel(ctx context.Context, tunarrID string) error {
	status, err := t.doStatus(ctx, http.MethodDelete, "/api/channels/"+tunarrID, nil, nil)
	if err != nil {
		return err
	}
	if status == http.StatusNotFound || (status >= 200 && status < 300) {
		return nil
	}
	return fmt.Errorf("delete channel %s: status %d", tunarrID, status)
}

// --- HTTP helpers ---

func (t *Tunarr) doJSON(ctx context.Context, method, path string, in, out any) error {
	status, err := t.doStatus(ctx, method, path, in, out)
	if err != nil {
		return err
	}
	if status < 200 || status >= 300 {
		return fmt.Errorf("%s %s: status %d", method, path, status)
	}
	return nil
}

// doStatus executes a request and returns the HTTP status; it decodes into out
// only on a 2xx (so callers can special-case 404 without a decode error). in is
// JSON-marshaled when non-nil.
func (t *Tunarr) doStatus(ctx context.Context, method, path string, in, out any) (int, error) {
	var reader *strings.Reader
	if in != nil {
		blob, err := json.Marshal(in)
		if err != nil {
			return 0, fmt.Errorf("marshal %s body: %w", path, err)
		}
		reader = strings.NewReader(string(blob))
	}
	var req *http.Request
	var err error
	if reader != nil {
		req, err = http.NewRequestWithContext(ctx, method, t.baseURL()+path, reader)
	} else {
		req, err = http.NewRequestWithContext(ctx, method, t.baseURL()+path, nil)
	}
	if err != nil {
		return 0, err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if t.apiKey() != "" {
		req.Header.Set("Authorization", "Bearer "+t.apiKey())
	}
	resp, err := t.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("%s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if out != nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return resp.StatusCode, fmt.Errorf("decode %s: %w", path, err)
		}
	}
	return resp.StatusCode, nil
}

var _ Programmer = (*Tunarr)(nil)
