package scan

import (
	"context"
	"crypto/subtle"
	"errors"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/loomarr/loomarr/internal/library/arrhook"
	mp "github.com/loomarr/loomarr/internal/library/metadataprovider"
)

// HookState is where one arr's webhook stands.
type HookState string

const (
	// HookRegistered: the arr saved Loomarr's connection, which it tests before saving, so it
	// could reach Loomarr then.
	HookRegistered HookState = "registered"
	// HookNotConfigured: the arr has no connection in Loomarr.
	HookNotConfigured HookState = "not_configured"
	// HookNoAddress: Loomarr's own address (server.public_url) is unset, so there is nothing to
	// register. The hourly walk still finds the arr's files.
	HookNoAddress HookState = "no_address"
	// HookFailed: registering failed; Error says why.
	HookFailed HookState = "failed"
)

// HookStatus is one arr's webhook, for Settings → Library.
type HookStatus struct {
	App          string
	State        HookState
	Error        string
	RegisteredAt time.Time
	// LastEvent is the type of the last event received since Loomarr started, "" for none.
	LastEvent   string
	LastEventAt time.Time
}

// hookRefresh is how often the registrations are checked: a connection removed by hand in an arr,
// or an arr connected later, is put back within the hour.
const hookRefresh = time.Hour

// hookRetry is the first wait after a registration fails; each further failure doubles it, up to
// hookRefresh. Loomarr registers as it starts, and a reverse proxy that routes only to a healthy
// container (Traefik) answers the arr's test post 404 for that first half minute.
const hookRetry = time.Minute

var (
	// ErrHookUnknownApp is an event for an arr Loomarr does not register with.
	ErrHookUnknownApp = errors.New("scan: not a webhook Loomarr registers")
	// ErrHookNoAddress is a test with Loomarr's own address unset.
	ErrHookNoAddress = errors.New("scan: Loomarr's address is not set")
	// ErrHookNotConfigured is a test of an arr with no connection.
	ErrHookNotConfigured = errors.New("scan: the arr is not connected")
)

// ArrHooks keeps Sonarr's and Radarr's webhooks pointed at Loomarr (decision 10) and turns their
// events into folder passes (decision 9).
type ArrHooks struct {
	Scanner *Scanner
	// Clients are the arrs, by authority name ("sonarr", "radarr").
	Clients map[string]arrhook.Client
	// URL is the address an arr posts app's events to, "" when Loomarr's own address is unset.
	URL func(app string) string
	// Secret is what each arr sends as its password.
	Secret string
	Now    func() time.Time
	// Degraded hears when an arr's registration keeps failing (decision 32): its new files then
	// wait for the hourly walk. It hears the second failure in a row, so a failure the retry
	// clears (a restart) is not reported. Nil hears nothing.
	Degraded func(ctx context.Context, app string, err error)

	mu     sync.Mutex
	status map[string]HookStatus
	// written holds the apps registered since start. The first registration writes the connection
	// even when it looks unchanged, since the arr masks the password Loomarr would compare.
	written map[string]bool
	// failures counts each app's registrations that failed in a row.
	failures map[string]int
}

// Run registers at start and every hour until ctx ends. After a failure it tries again sooner:
// hookRetry, doubling up to the hour.
func (h *ArrHooks) Run(ctx context.Context) {
	retry := hookRetry
	for {
		failed := false
		for _, app := range h.apps() {
			if h.Register(ctx, app) != nil {
				failed = true
			}
		}
		wait := hookRefresh
		if failed {
			wait, retry = retry, min(retry*2, hookRefresh)
		} else {
			retry = hookRetry
		}
		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
		}
	}
}

// Register makes sure app has Loomarr's connection, and records how that went.
func (h *ArrHooks) Register(ctx context.Context, app string) error {
	client, ok := h.Clients[app]
	if !ok {
		return ErrHookUnknownApp
	}
	url := h.URL(app)
	if url == "" {
		h.setState(app, HookNoAddress, nil)
		return nil
	}
	h.mu.Lock()
	rewrite := !h.written[app]
	h.mu.Unlock()
	_, err := client.Register(ctx, url, h.Secret, rewrite)
	switch {
	case errors.Is(err, arrhook.ErrNotConfigured):
		h.setState(app, HookNotConfigured, nil)
		return nil
	case err != nil:
		if ctx.Err() != nil {
			return err
		}
		h.Scanner.Importer.log().Warn("library scan: an arr's webhook is not registered; the hourly walk still finds its files",
			"app", app, "err", err)
		if h.setState(app, HookFailed, err) == 2 && h.Degraded != nil {
			h.Degraded(ctx, app, err)
		}
		return err
	}
	h.mu.Lock()
	if h.written == nil {
		h.written = map[string]bool{}
	}
	h.written[app] = true
	h.mu.Unlock()
	h.setState(app, HookRegistered, nil)
	return nil
}

// Test registers app again and asks it to send its test event, which Receive records.
func (h *ArrHooks) Test(ctx context.Context, app string) error {
	if err := h.Register(ctx, app); err != nil {
		return err
	}
	switch h.Status(app).State {
	case HookNoAddress:
		return ErrHookNoAddress
	case HookNotConfigured:
		return ErrHookNotConfigured
	}
	return h.Clients[app].Test(ctx)
}

// Authorized reports whether a delivery carries the secret.
func (h *ArrHooks) Authorized(username, password string) bool {
	if h.Secret == "" {
		return false
	}
	ok := subtle.ConstantTimeCompare([]byte(password), []byte(h.Secret)) == 1
	return ok && username == arrhook.Username
}

// Receive handles one delivery from app: it is recorded, and the folders it names are passed
// over once changes go quiet.
func (h *ArrHooks) Receive(ctx context.Context, app string, body []byte) error {
	if _, ok := h.Clients[app]; !ok {
		return ErrHookUnknownApp
	}
	ev, err := arrhook.Parse(body)
	if err != nil {
		return err
	}
	h.mu.Lock()
	s := h.statusLocked(app)
	s.LastEvent, s.LastEventAt = ev.Type, h.now()
	h.status[app] = s
	h.mu.Unlock()
	if len(ev.Folders) > 0 {
		h.Scanner.Importer.ArrChanged(arrItemOf(app, ev))
	}
	_, err = h.Scanner.ArrChanged(ctx, app, ev.Folders)
	return err
}

// arrKeyNamespace is the namespace of the id an arr keys an item of a kind by, in its webhooks and
// its API: Sonarr a series by its TVDB id, Radarr a movie by its TMDB id.
func arrKeyNamespace(kind mp.Kind) mp.Namespace {
	if kind == mp.KindSeries {
		return mp.NamespaceTVDB
	}
	return mp.NamespaceTMDB
}

// arrItemOf is the item an event is about, nil when it names none.
func arrItemOf(app string, ev arrhook.Event) *ArrItem {
	switch {
	case ev.SeriesTVDB > 0:
		return &ArrItem{Authority: app, Ref: mp.Ref{Kind: mp.KindSeries,
			ID: mp.ExternalID{Namespace: arrKeyNamespace(mp.KindSeries), Value: strconv.Itoa(ev.SeriesTVDB)}}}
	case ev.MovieTMDB > 0:
		return &ArrItem{Authority: app, Ref: mp.Ref{Kind: mp.KindMovie,
			ID: mp.ExternalID{Namespace: arrKeyNamespace(mp.KindMovie), Value: strconv.Itoa(ev.MovieTMDB)}}}
	}
	return nil
}

// Statuses is every arr's webhook, sorted by app.
func (h *ArrHooks) Statuses() []HookStatus {
	out := make([]HookStatus, 0, len(h.Clients))
	for _, app := range h.apps() {
		out = append(out, h.Status(app))
	}
	return out
}

// Status is app's webhook.
func (h *ArrHooks) Status(app string) HookStatus {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.statusLocked(app)
}

func (h *ArrHooks) statusLocked(app string) HookStatus {
	if h.status == nil {
		h.status = map[string]HookStatus{}
	}
	s, ok := h.status[app]
	if !ok {
		s = HookStatus{App: app, State: HookNotConfigured}
	}
	return s
}

// setState records app's state and returns how many registrations in a row have now failed.
func (h *ArrHooks) setState(app string, state HookState, err error) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	s := h.statusLocked(app)
	s.State, s.Error = state, ""
	if err != nil {
		s.Error = err.Error()
	}
	if state == HookRegistered {
		s.RegisteredAt = h.now()
	}
	h.status[app] = s
	if h.failures == nil {
		h.failures = map[string]int{}
	}
	if state == HookFailed {
		h.failures[app]++
	} else {
		delete(h.failures, app)
	}
	return h.failures[app]
}

func (h *ArrHooks) apps() []string {
	apps := make([]string, 0, len(h.Clients))
	for app := range h.Clients {
		apps = append(apps, app)
	}
	slices.Sort(apps)
	return apps
}

func (h *ArrHooks) now() time.Time {
	if h.Now != nil {
		return h.Now()
	}
	return time.Now()
}
