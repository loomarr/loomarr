package playout

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/api/httpcore"
	"github.com/loomarr/loomarr/internal/channels/viewing"
	"github.com/loomarr/loomarr/internal/platform/settings"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/identitystore"
	"github.com/loomarr/loomarr/internal/playout"
)

// Store is the slice of the store the playout routes read: channels, and the paired device whose
// name a play URL's viewer carries.
type Store interface {
	GetChannel(ctx context.Context, id string) (store.Channel, error)
	ListChannels(ctx context.Context) ([]store.Channel, error)
	GetDeviceToken(ctx context.Context, tokenHash string) (identitystore.DeviceToken, error)
}

// Deps is what the playout routes need from the rest of the server. Every field is optional: a nil
// engine leaves the streaming routes answering 501, as the API's nil-semantics describe.
type Deps struct {
	Log   *slog.Logger
	Store Store
	// LiveSettings reads the public URL and, without a transition checkpoint, the applied backend.
	LiveSettings settings.Reader
	// BackendCheckpoint is the durable transition state; nil falls back to the applied setting.
	BackendCheckpoint func(context.Context) (httpcore.BackendCheckpoint, error)

	Playout         Playout
	PlayoutObserver PlayoutObserver
	PlayoutGuide    PlayoutGuide
	// PlayoutResolver describes channels for the play-url and track routes; nil reports none.
	PlayoutResolver PlayoutResolver
	// PlayoutCapability is the retained result of the encoder trial; nil reports none.
	PlayoutCapability func() playout.Capacity
	// PlayoutSecret signs play URLs; PlayoutSecretCurrent reads it per request when it can rotate.
	PlayoutSecret        func() string
	PlayoutSecretCurrent func(context.Context) (string, error)

	// Viewing counts who is watching; Now is the request clock (nil is time.Now).
	Viewing *viewing.Tracker
	Now     func() time.Time
	// GPU reports the GPU name and VRAM for the status header; ResidentLLMVRAM reports what a
	// resident model holds of it. Both are best-effort and may be nil.
	GPU             func(ctx context.Context) (name string, vramGiB float64, err error)
	ResidentLLMVRAM func(ctx context.Context) (gib float64, model string)
}

// Handlers serves the playout routes.
type Handlers struct {
	log                  *slog.Logger
	store                Store
	liveSettings         settings.Reader
	backend              httpcore.Backend
	playout              Playout
	playoutObserver      PlayoutObserver
	playoutGuide         PlayoutGuide
	playoutResolver      PlayoutResolver
	playoutCapability    func() playout.Capacity
	playoutSecret        func() string
	playoutSecretCurrent func(context.Context) (string, error)
	viewing              *viewing.Tracker
	now                  func() time.Time
	gpu                  func(ctx context.Context) (string, float64, error)
	residentLLMVRAM      func(ctx context.Context) (float64, string)

	// schemaOnly is set only by the OpenAPI exporter: it mounts every route whatever is configured,
	// so the generated spec lists them all.
	schemaOnly bool
}

// New returns the playout handlers over d. schemaOnly mounts every route regardless of which
// services d carries; only the OpenAPI exporter sets it.
func New(d Deps, schemaOnly bool) *Handlers {
	return &Handlers{
		log: d.Log, store: d.Store, liveSettings: d.LiveSettings,
		backend: httpcore.Backend{Source: d.BackendCheckpoint, Settings: d.LiveSettings},
		playout: d.Playout, playoutObserver: d.PlayoutObserver, playoutGuide: d.PlayoutGuide,
		playoutResolver: d.PlayoutResolver, playoutCapability: d.PlayoutCapability, playoutSecret: d.PlayoutSecret,
		playoutSecretCurrent: d.PlayoutSecretCurrent, viewing: d.Viewing, now: d.Now,
		gpu: d.GPU, residentLLMVRAM: d.ResidentLLMVRAM, schemaOnly: schemaOnly,
	}
}

// Register mounts every playout route on api.
func (s *Handlers) Register(api huma.API) {
	s.registerPlayout(api)
	s.registerChannelPlayURL(api)
	s.registerPlayoutStatus(api)
}

func (s *Handlers) clock() time.Time {
	if s.now != nil {
		return s.now()
	}
	return time.Now()
}

func (s *Handlers) writeProblem(w http.ResponseWriter, r *http.Request, status int, title, detail string) {
	httpcore.WriteProblem(s.log, w, r, status, title, detail)
}

// viewerOf is the person and device minting a play URL. A paired device is keyed by its revocation
// id and named as it was paired; a browser is keyed and named by its browser family and system.
// A break-glass API_TOKEN caller has no person, so it is never attributed. The viewer tag itself,
// which signs this into a play URL, lives in httpcore (viewertag.go).
func (s *Handlers) viewerOf(ctx context.Context) (viewing.Viewer, bool) {
	user, ok := httpcore.UserFrom(ctx)
	if !ok {
		return viewing.Viewer{}, false
	}
	if deviceID, ok := httpcore.DeviceIDFrom(ctx); ok {
		label := "Paired device"
		if s.store != nil {
			if dt, err := s.store.GetDeviceToken(ctx, deviceID); err == nil && strings.TrimSpace(dt.DeviceName) != "" {
				label = strings.TrimSpace(dt.DeviceName)
			}
		}
		key := deviceID
		if len(key) > 16 {
			key = key[:16]
		}
		return viewing.Viewer{UserID: user.ID, DeviceKey: "device:" + key, DeviceLabel: label}, true
	}
	label := "Browser"
	if r := httpcore.RequestFrom(ctx); r != nil {
		label = httpcore.ClientLabel(r.UserAgent())
	}
	return viewing.Viewer{UserID: user.ID, DeviceKey: "browser:" + label, DeviceLabel: label}, true
}

// requestViewer is the viewer a playout request's URL was minted for, from its signed tag.
// Best-effort by design: no key or a bad tag names nobody, and the request is served all the same.
func (s *Handlers) requestViewer(ctx context.Context, channelID string, query url.Values) (viewing.Viewer, bool) {
	raw := query.Get(httpcore.ViewerQueryParam)
	if raw == "" {
		return viewing.Viewer{}, false
	}
	key, err := s.currentPlayoutToken(ctx)
	if err != nil {
		return viewing.Viewer{}, false
	}
	return httpcore.VerifyViewerTag(key, channelID, raw)
}
