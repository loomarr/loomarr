package programmer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/mantonx/loomarr/internal/httpx"
)

// Tunarr is the hand-written thin client for the Tunarr REST API (§6). It targets
// only the endpoints the scheduler needs (channels CRUD + programming); the
// version tested against is Tunarr 1.3.8 (Phase 0). TUNARR_API_KEY is optional
// for 1.3.8 (Phase-0 finding: empty securitySchemes) — if set, it's sent as a
// bearer token so a future key-requiring version works without code change.
type Tunarr struct {
	baseURL string
	apiKey  string // optional; "" = no auth header (valid for 1.3.8)
	// transcodeConfigID references a real Tunarr transcode config (Phase-0
	// finding 3: must be a valid uuid; the instance ships a "Default"). Supplied
	// at construction from GET /api/transcode_configs (or config).
	transcodeConfigID string
	http              *http.Client
	resolver          *contentResolver // media-server item id → Tunarr program uuid (§6)
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
	t := &Tunarr{
		baseURL:           strings.TrimRight(baseURL, "/"),
		apiKey:            apiKey,
		transcodeConfigID: transcodeConfigID,
		http:              httpx.New(httpx.TimeoutTunarr),
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

// EnsureChannel implements Programmer. On create it reads back the
// server-assigned id (Phase-0 finding 1) and returns it.
func (t *Tunarr) EnsureChannel(ctx context.Context, spec ChannelSpec) (string, error) {
	body := t.channelBody(spec)
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
// captured defaults (Phase-0 finding 2). transcodeConfigId comes from the client.
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
		TranscodeID: t.transcodeConfigID,
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
		req, err = http.NewRequestWithContext(ctx, method, t.baseURL+path, reader)
	} else {
		req, err = http.NewRequestWithContext(ctx, method, t.baseURL+path, nil)
	}
	if err != nil {
		return 0, err
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if t.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+t.apiKey)
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
