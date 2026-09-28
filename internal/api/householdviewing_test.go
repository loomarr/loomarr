package api_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/auth"
	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/testkit"
)

// pollPlayout serves a fresh live media playlist on every read, like a running packager.
type pollPlayout struct{ *fakePlayoutSessions }

type seekCloser struct{ *bytes.Reader }

func (seekCloser) Close() error { return nil }

func (pollPlayout) OpenAsset(context.Context, string, playout.EncodePlan, string) (playout.Asset, bool, error) {
	body := "#EXTM3U\n#EXT-X-TARGETDURATION:2\n#EXTINF:2.0,\nseg-1.ts\n"
	return playout.Asset{Content: seekCloser{bytes.NewReader([]byte(body))}, Modified: time.Unix(1, 0), Playlist: true}, true, nil
}

type testClock struct {
	mu sync.Mutex
	at time.Time
}

func (c *testClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.at
}

func (c *testClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.at = c.at.Add(d)
}

type viewingHarness struct {
	*apiHarness
	clock *testClock
}

func newViewingHarness(t *testing.T) *viewingHarness {
	t.Helper()
	ms := testkit.NewMediaServer(t)
	t.Cleanup(ms.Close)
	ms.Accounts = map[string]testkit.Account{
		"boss": {Password: "pw", ID: "u-boss", IsAdmin: true},
		"kid":  {Password: "pw", ID: "u-kid"},
		"sis":  {Password: "pw", ID: "u-sis"},
	}
	clock := &testClock{at: time.Now()}
	h := startAPIHarness(t, func(defaults apiHarnessDefaults) http.Handler {
		lib := library.New(library.Emby, ms.URL, ms.AdminToken, "dev")
		mgr := auth.NewManager(defaults.Store, time.Hour, time.Now)
		devices := auth.NewDeviceManager(defaults.Store, time.Now)
		seedImported(t, defaults.Store, "u-boss", "boss", store.RoleAdmin)
		seedImported(t, defaults.Store, "u-kid", "kid", store.RoleMember)
		seedImported(t, defaults.Store, "u-sis", "sis", store.RoleMember)
		cfg := map[string]string{"server.public_url": "http://loomarr.local:8080", "playout.backend": "internal"}
		sessions := pollPlayout{&fakePlayoutSessions{}}
		return api.Router(defaults.Log, api.Options{
			Store:           defaults.Store,
			Auth:            api.NewSessionAuthorizerCurrent(mgr, devices, func(context.Context) (string, error) { return "break-glass-token", nil }),
			Log:             defaults.Log,
			Login:           auth.NewLoginService(lib, defaults.Store, mgr, nil, time.Now),
			Sessions:        mgr,
			Devices:         devices,
			CookieSecure:    "false",
			LiveConfig:      func(key string) string { return cfg[key] },
			Playout:         sessions,
			PlayoutObserver: sessions,
			PlayoutSecret:   func() string { return playoutToken },
			Now:             clock.now,
		})
	})
	return &viewingHarness{apiHarness: h, clock: clock}
}

// caller is one device: a browser session or a paired TV's bearer.
type caller struct {
	session *http.Cookie
	bearer  string
	ua      string
}

func (h *viewingHarness) send(t *testing.T, c caller, method, path, body string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(method, h.Server.URL+path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.session != nil {
		req.AddCookie(c.session)
		req.Header.Set("X-Loomarr-Csrf", "1")
	}
	if c.bearer != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearer)
	}
	if c.ua != "" {
		req.Header.Set("User-Agent", c.ua)
	}
	res, err := h.Server.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = res.Body.Close() })
	return res
}

// mediaPlaylistURL mints a play URL as c and returns its media-playlist URL: the master's own
// query, which the master copies onto its variants.
func (h *viewingHarness) mediaPlaylistURL(t *testing.T, c caller, channelID string) string {
	t.Helper()
	res := h.send(t, c, http.MethodPost, "/v1/channels/"+channelID+"/play-url", "{}")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("play-url %s = %d", channelID, res.StatusCode)
	}
	var body struct {
		RelativeURL string `json:"relativeUrl"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return strings.Replace(body.RelativeURL, "/master.m3u8?", "/baseline.m3u8?", 1)
}

func (h *viewingHarness) poll(t *testing.T, playlist string) {
	t.Helper()
	res, err := h.Server.Client().Get(h.Server.URL + playlist)
	if err != nil {
		t.Fatal(err)
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("poll %s = %d", playlist, res.StatusCode)
	}
}

type viewingWire struct {
	Scope    string `json:"scope"`
	Watching int    `json:"watching"`
	Channels []struct {
		ChannelID string `json:"channelId"`
		Viewers   int    `json:"viewers"`
	} `json:"channels"`
	Viewers []struct {
		Name      string `json:"name"`
		Device    string `json:"device"`
		ChannelID string `json:"channelId"`
		You       bool   `json:"you"`
	} `json:"viewers"`
	ContinueWatching *struct {
		ChannelID string `json:"channelId"`
	} `json:"continueWatching"`
}

func (w viewingWire) String() string {
	s := fmt.Sprintf("scope=%s watching=%d channels=", w.Scope, w.Watching)
	for _, c := range w.Channels {
		s += fmt.Sprintf("%s:%d,", c.ChannelID, c.Viewers)
	}
	s += " viewers="
	for _, v := range w.Viewers {
		s += fmt.Sprintf("[%s|%s|%s|you=%t]", v.Name, v.Device, v.ChannelID, v.You)
	}
	if w.ContinueWatching != nil {
		s += " continue=" + w.ContinueWatching.ChannelID
	}
	return s
}

func (h *viewingHarness) viewing(t *testing.T, c caller) viewingWire {
	t.Helper()
	res := h.send(t, c, http.MethodGet, "/v1/household/viewing", "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("household viewing = %d", res.StatusCode)
	}
	var out viewingWire
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

const firefoxLinux = "Mozilla/5.0 (X11; Linux x86_64; rv:140.0) Gecko/20100101 Firefox/140.0"

// #1659 H2: an admin sees named viewing; a member sees counts plus only their own viewings; each
// person gets their own continue-watching; a paired TV is its pairer. Enforced by the server.
func TestHouseholdViewingIsNamedForAdminsAndCountedForMembers(t *testing.T) {
	t.Parallel()
	h := newViewingHarness(t)
	seedChannel(t, h.Store, "ch-films", "Films", 1, "internal")
	seedChannel(t, h.Store, "ch-news", "News", 2, "internal")
	boss := caller{session: login(t, h.Server, "boss", "pw")}
	kid := caller{session: login(t, h.Server, "kid", "pw")}
	sis := caller{session: login(t, h.Server, "sis", "pw"), ua: firefoxLinux}
	kidTV := caller{bearer: pairDevice(t, h.Server, kid.session)}

	tvNews := h.mediaPlaylistURL(t, kidTV, "ch-news")
	sisFilms := h.mediaPlaylistURL(t, sis, "ch-films")
	tvFilmsWarm := h.mediaPlaylistURL(t, kidTV, "ch-films")
	h.poll(t, tvNews)
	h.poll(t, sisFilms)
	h.clock.advance(2 * time.Second)
	h.poll(t, tvNews)
	h.poll(t, sisFilms)
	h.poll(t, tvFilmsWarm) // the warmer's single fetch of the next channel is not a viewer
	h.send(t, kidTV, http.MethodPut, "/v1/me/recent-channels/ch-news", "")

	if got, want := h.viewing(t, boss).String(),
		"scope=household watching=2 channels=ch-films:1,ch-news:1, viewers=[kid|Living room TV|ch-news|you=false][sis|Firefox on Linux|ch-films|you=false]"; got != want {
		t.Errorf("admin sees\n %s\nwant\n %s", got, want)
	}
	if got, want := h.viewing(t, kid).String(),
		"scope=self watching=2 channels=ch-films:1,ch-news:1, viewers=[kid|Living room TV|ch-news|you=true] continue=ch-news"; got != want {
		t.Errorf("member sees\n %s\nwant\n %s", got, want)
	}
	if got, want := h.viewing(t, kidTV).String(),
		"scope=self watching=2 channels=ch-films:1,ch-news:1, viewers=[kid|Living room TV|ch-news|you=true] continue=ch-news"; got != want {
		t.Errorf("member's paired TV sees\n %s\nwant\n %s", got, want)
	}
	if got, want := h.viewing(t, sis).String(),
		"scope=self watching=2 channels=ch-films:1,ch-news:1, viewers=[sis|Firefox on Linux|ch-films|you=true]"; got != want {
		t.Errorf("other member sees\n %s\nwant\n %s", got, want)
	}
	if got, want := h.viewing(t, caller{bearer: "break-glass-token"}).String(),
		"scope=household watching=2 channels=ch-films:1,ch-news:1, viewers=[kid|Living room TV|ch-news|you=false][sis|Firefox on Linux|ch-films|you=false]"; got != want {
		t.Errorf("API token sees\n %s\nwant\n %s", got, want)
	}

	// Players that stop polling have stopped watching.
	h.clock.advance(31 * time.Second)
	if got := h.viewing(t, boss); got.Watching != 0 || len(got.Viewers) != 0 {
		t.Errorf("after the players stop = %s, want nobody", got)
	}
}

// The viewer tag is signed: a member can't make the household believe someone else is watching.
func TestHouseholdViewingIgnoresAForgedViewerTag(t *testing.T) {
	t.Parallel()
	h := newViewingHarness(t)
	seedChannel(t, h.Store, "ch-films", "Films", 1, "internal")
	boss := caller{session: login(t, h.Server, "boss", "pw")}
	sis := caller{session: login(t, h.Server, "sis", "pw"), ua: firefoxLinux}

	playlist := h.mediaPlaylistURL(t, sis, "ch-films")
	u, err := url.Parse(playlist)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	_, mac, _ := strings.Cut(q.Get("viewer"), ".")
	q.Set("viewer", base64.RawURLEncoding.EncodeToString([]byte("u-kid\nbrowser:Firefox on Linux\nFirefox on Linux"))+"."+mac)
	u.RawQuery = q.Encode()
	h.poll(t, u.String())
	h.clock.advance(2 * time.Second)
	h.poll(t, u.String()) // the stream itself still plays: `sig` is intact

	if got := h.viewing(t, boss); got.Watching != 0 {
		t.Fatalf("forged tag attributed a viewing: %s", got)
	}
}
