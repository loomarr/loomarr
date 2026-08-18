package app

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/playout"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/testkit"
)

type staticChannelReader struct{ channel store.Channel }

func (s staticChannelReader) GetChannel(context.Context, string) (store.Channel, error) {
	return s.channel, nil
}

func TestBuildHandler_WiresMeasuredCapacityToAdmissionAndQuality(t *testing.T) {
	lastPlayoutResolver = nil
	t.Setenv("API_TOKEN", "capacity-test-token")

	st := testkit.MigratedSQLiteStore(t)
	for key, value := range map[string]string{
		"playout.backend":      "internal",
		"playout.encoder":      "libx264",
		"playout.max_channels": "9",
	} {
		if err := st.SetSetting(context.Background(), key, value); err != nil {
			t.Fatal(err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	h, err := BuildHandler(ctx, st, slog.New(slog.DiscardHandler), Overrides{})
	if err != nil {
		t.Fatal(err)
	}
	r := lastPlayoutResolver
	if r == nil {
		t.Fatal("BuildHandler wired no playout resolver")
	}
	// Detection is lazy; install the result a real encoder trial would publish without running
	// ffmpeg in a unit test. The configured 9 is deliberately above the measured 3.
	r.maxChannels.Store(3)

	req := httptest.NewRequest(http.MethodGet, "/v1/playout/sessions", nil)
	req.Header.Set("Authorization", "Bearer capacity-test-token")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /v1/playout/sessions = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var telemetry api.PlayoutTelemetry
	if err := json.NewDecoder(rec.Body).Decode(&telemetry); err != nil {
		t.Fatal(err)
	}
	if telemetry.Capacity != 3 {
		t.Errorf("admission capacity = %d, want measured capacity 3", telemetry.Capacity)
	}

	// Three committed transcodes on a measured-three box take Balanced's safe bottom rung.
	// If Profile reads the configured 9 instead, it incorrectly selects the top 5000 kbit/s rung.
	r.activeChannels = func() int { return 3 }
	if got := r.Profile(context.Background()).VideoBitrate; got != 1800 {
		t.Errorf("full-box profile bitrate = %d, want bottom-rung 1800", got)
	}
}

func TestPlayoutResolver_AudioTrackHonoursChannelOverride(t *testing.T) {
	r := &playoutResolver{
		audioLanguage: func() string { return "eng" },
		probeAudio: func(context.Context, string) ([]playout.AudioTrack, error) {
			return []playout.AudioTrack{{Language: "eng"}, {Language: "jpn"}}, nil
		},
		channels: staticChannelReader{channel: store.Channel{Policy: schedule.ChannelPolicy{
			OperatorPolicy: schedule.OperatorPolicy{Playout: &schedule.PlayoutPolicy{AudioLanguage: "jpn"}},
		}}},
	}

	if got := r.AudioTrackFor(context.Background(), "channel-1", "movie.mkv"); got != 1 {
		t.Fatalf("AudioTrackFor = %d, want channel override track 1", got)
	}
}

// ⚠ **THE QUALITY LADDER'S DEPENDENCIES ARE CALLED UNGUARDED.**
//
// `Profile` reaches `r.tier()`, `r.encoder()`, `r.capacity()` and `r.activeChannels()` with
// no nil checks, so any one of them missing is a panic on the LIVE playout path — when a
// viewer tunes in, which is the worst place to find out.
//
// That was not hypothetical: `activeChannels` used to be back-patched onto the resolver
// after construction, and deleting the assignment broke NO test. It is now set in the
// constructor literal, and this is the test that notices if it stops being.
func TestPlayoutResolver_ProfileNeedsEveryLadderInput(t *testing.T) {
	// A resolver wired the way BuildHandler wires it — every ladder input present.
	full := func() *playoutResolver {
		return &playoutResolver{
			tier:           func() string { return "720p" },
			encoder:        func() string { return "libx264" },
			capacity:       func() int { return 4 },
			activeChannels: func() int { return 1 },
		}
	}

	// The positive case first, so the negatives below are proven to be panics rather than a
	// resolver that never works.
	if got := full().Profile(context.Background()); got.Encoder == "" {
		t.Fatalf("Profile with every input wired returned %+v, want a usable profile", got)
	}

	// ⚠ Each input removed IN TURN must panic rather than silently degrade. A zero value
	// here would be worse than a crash: the ladder would quietly pick the wrong quality and
	// nobody would know which input was missing.
	for _, tc := range []struct {
		name string
		bust func(*playoutResolver)
	}{
		{"activeChannels", func(r *playoutResolver) { r.activeChannels = nil }},
		{"capacity", func(r *playoutResolver) { r.capacity = nil }},
		{"tier", func(r *playoutResolver) { r.tier = nil }},
		{"encoder", func(r *playoutResolver) { r.encoder = nil }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := full()
			tc.bust(r)
			defer func() {
				if recover() == nil {
					t.Errorf("Profile with %s unset did NOT panic — it is called unguarded, "+
						"so an unset input must fail loudly rather than resolve a wrong quality",
						tc.name)
				}
			}()
			_ = r.Profile(context.Background())
		})
	}
}

// ⚠ **THE ASSERTION THAT WAS MISSING.** The test above pins the invariant (an unset ladder
// input panics); this pins that BuildHandler actually SATISFIES it.
//
// Both are needed, and the gap between them is exactly where the original defect lived:
// `activeChannels` was back-patched after construction, and deleting the assignment left
// every test green while a viewer tuning in would panic.
//
// Reads the resolver BuildHandler really constructed rather than one assembled here, since
// a test-built resolver only proves the test knows how to fill a struct.
func TestBuildHandler_WiresEveryLadderInput(t *testing.T) {
	lastPlayoutResolver = nil

	st := testkit.MigratedSQLiteStore(t)
	// Playout is only wired on the internal backend; without this the resolver is nil and
	// the test would pass vacuously.
	if err := st.SetSetting(context.Background(), "playout.backend", "internal"); err != nil {
		t.Fatal(err)
	}

	if _, err := BuildHandler(context.Background(), st, slog.New(slog.DiscardHandler), Overrides{}); err != nil {
		t.Fatal(err)
	}
	r := lastPlayoutResolver
	if r == nil {
		t.Fatal("BuildHandler wired no playout resolver on the internal backend — " +
			"this test can no longer see what it is meant to guard")
	}

	for _, tc := range []struct {
		name string
		set  bool
	}{
		{"tier", r.tier != nil},
		{"encoder", r.encoder != nil},
		{"capacity", r.capacity != nil},
		{"activeChannels", r.activeChannels != nil},
	} {
		if !tc.set {
			t.Errorf("BuildHandler left %s unset — Profile calls it unguarded, so a viewer "+
				"tuning in would panic", tc.name)
		}
	}
}
