package api_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/fillerstore"
	"github.com/loomarr/loomarr/internal/images"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
)

// failingClipListStore is the real store with ListClips broken, so a test can prove a still lookup
// failure degrades the strip instead of failing it.
type failingClipListStore struct{ fillerstore.Store }

func (failingClipListStore) ListClips(context.Context, store.ClipFilter) ([]store.Clip, error) {
	return nil, errors.New("clip catalogue unavailable")
}

// timelineWire is the timeline response as the browser decodes it. Raw JSON values, not the Go
// DTOs: the wire is the contract, and a nil Go slice or pointer would hide an omitted field.
type timelineWire struct {
	Airings []struct {
		Kind string          `json:"kind"`
		Pod  json.RawMessage `json:"pod"`
	} `json:"airings"`
}

type podWire struct {
	Entries []map[string]json.RawMessage `json:"entries"`
}

// startTimelinePodsHarness serves the timeline over the common API lifecycle. With breakClips the
// handler reads a store whose ListClips fails, while the harness's own Store stays the real one for
// seeding.
func startTimelinePodsHarness(
	t *testing.T, g api.PlayoutGuide, pods api.PodPreviewer, imageService api.ImageService, breakClips bool,
) *apiHarness {
	t.Helper()
	return startAPIHarness(t, func(defaults apiHarnessDefaults) http.Handler {
		use := defaults.Store
		if breakClips {
			use = failingClipListStore{Store: defaults.Store}
		}
		return api.Router(defaults.Log, api.Options{
			Store:        use,
			Auth:         defaults.Auth,
			Log:          defaults.Log,
			PlayoutGuide: g,
			Pods:         pods,
			Images:       imageService,
		})
	})
}

func getTimelineWire(t *testing.T, srv *httptest.Server, channelID string) timelineWire {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/channels/"+channelID+"/timeline", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("timeline status %d, want 200", resp.StatusCode)
	}
	var body timelineWire
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return body
}

func timelineBreakGuide(now time.Time) *fakeXMLTVGuide {
	return &fakeXMLTVGuide{byChannel: map[string][]playout.Broadcast{
		"ch1": {
			{
				Kind: schedule.SlotProgram, Key: "movie:tmdb:90000089", Title: "Bellwether Junction",
				Start: now, Stop: now.Add(2 * time.Hour),
			},
			{
				Kind: schedule.SlotFiller, Start: now.Add(2 * time.Hour),
				Stop: now.Add(2*time.Hour + 2*time.Minute),
			},
		},
	}}
}

// seedPodClips stores two spots: "clip-still" has a still and a hover loop, "clip-bare" has neither.
func seedPodClips(t *testing.T, st fillerstore.Store) {
	t.Helper()
	for _, c := range []filler.Clip{
		{
			Hash: "clip-still", Path: "still.mp4", Name: "Period commercial", Kind: filler.Commercial,
			DurationMs: 30_000, ThumbImageHash: "spot-still", HoverImageHash: "spot-hover",
		},
		{
			Hash: "clip-bare", Path: "bare.mp4", Name: "Station ident", Kind: filler.StationID,
			DurationMs: 10_000,
		},
	} {
		if err := st.UpsertClip(context.Background(), store.Clip{Clip: c}); err != nil {
			t.Fatal(err)
		}
	}
}

func timelinePodsFixture() (*fakePods, *fakeImageService) {
	fp := &fakePods{pod: filler.Pod{
		MatchLevel: filler.MatchExact,
		TotalMs:    40_000,
		Entries: []filler.PodEntry{
			{Hash: "clip-still", Path: "still.mp4", Name: "Period commercial", Kind: filler.Commercial, DurationMs: 30_000},
			{Hash: "clip-bare", Path: "bare.mp4", Name: "Station ident", Kind: filler.StationID, DurationMs: 10_000},
		},
	}}
	img := newFakeImageService()
	img.records["spot-still"] = images.Image{
		Hash: "spot-still", Role: images.RoleThumb, Width: 320, Height: 240,
		Visibility: images.VisibilityMember,
	}
	img.records["spot-hover"] = images.Image{
		Hash: "spot-hover", Role: images.RoleThumb, Width: 320, Height: 240, Animated: true,
		Visibility: images.VisibilityMember,
	}
	return fp, img
}

// A break block carries its pod and each spot with a still carries it as an image-service record;
// the spot without one omits the key entirely (a null would make the client guard a non-case). The
// still is the clip's ThumbImageHash, never the hover loop the Guide prefers.
func TestChannelTimeline_BreakCarriesPodWithStills(t *testing.T) {
	now := time.Now()
	fp, img := timelinePodsFixture()
	h := startTimelinePodsHarness(t, timelineBreakGuide(now), fp, img, false)
	seedChannel(t, h.Store, "ch1", "Kettleton Classics", 1, "internal")
	seedPodClips(t, h.Store)

	wire := getTimelineWire(t, h.Server, "ch1")
	if len(wire.Airings) != 2 {
		t.Fatalf("got %d airings, want programme + break", len(wire.Airings))
	}
	if string(wire.Airings[0].Pod) != "" {
		t.Errorf("programme block carries a pod: %s", wire.Airings[0].Pod)
	}
	if wire.Airings[1].Kind != string(schedule.SlotFiller) || len(wire.Airings[1].Pod) == 0 {
		t.Fatalf("break block = %+v, want a pod on the wire", wire.Airings[1])
	}
	var pod podWire
	if err := json.Unmarshal(wire.Airings[1].Pod, &pod); err != nil {
		t.Fatal(err)
	}
	if len(pod.Entries) != 2 {
		t.Fatalf("pod has %d entries, want 2", len(pod.Entries))
	}

	var still api.ImageDTO
	raw, ok := pod.Entries[0]["thumbImage"]
	if !ok {
		t.Fatalf("entry with a still has no thumbImage on the wire: %v", pod.Entries[0])
	}
	if err := json.Unmarshal(raw, &still); err != nil {
		t.Fatal(err)
	}
	if still.Hash != "spot-still" || still.Animated {
		t.Errorf("thumbImage = %+v, want the still spot-still (not the hover loop)", still)
	}
	if string(pod.Entries[0]["path"]) != "" {
		t.Errorf("member-readable pod leaked a path: %s", pod.Entries[0]["path"])
	}
	if _, ok := pod.Entries[1]["thumbImage"]; ok {
		t.Errorf("entry without a still carries thumbImage: %s", pod.Entries[1]["thumbImage"])
	}
	if fp.batches != 1 {
		t.Errorf("PreviewAtMany calls = %d, want one batch", fp.batches)
	}
}

// The embedded fallback card has no clip hash; it never carries a still and never breaks the strip.
func TestChannelTimeline_FallbackCardHasNoStill(t *testing.T) {
	now := time.Now()
	fp, img := timelinePodsFixture()
	fp.pod = filler.Pod{
		MatchLevel: filler.MatchBumperCard, TotalMs: 5_000,
		Entries: []filler.PodEntry{{Name: "Please stand by", Kind: filler.Bumper, DurationMs: 5_000, IsFallbackCard: true}},
	}
	h := startTimelinePodsHarness(t, timelineBreakGuide(now), fp, img, false)
	seedChannel(t, h.Store, "ch1", "Kettleton Classics", 1, "internal")

	wire := getTimelineWire(t, h.Server, "ch1")
	var pod podWire
	if err := json.Unmarshal(wire.Airings[1].Pod, &pod); err != nil {
		t.Fatal(err)
	}
	if len(pod.Entries) != 1 {
		t.Fatalf("pod has %d entries, want the fallback card", len(pod.Entries))
	}
	if _, ok := pod.Entries[0]["thumbImage"]; ok {
		t.Errorf("fallback card carries thumbImage: %s", pod.Entries[0]["thumbImage"])
	}
}

// A pod-preview failure or a clip-catalogue failure degrades the strip — it never fails it.
func TestChannelTimeline_PodFailuresStillReturnTheStrip(t *testing.T) {
	cases := []struct {
		name       string
		podErr     error
		breakClips bool
		wantPod    bool
	}{
		{name: "pod preview fails", podErr: errors.New("assembler down"), wantPod: false},
		{name: "still lookup fails", breakClips: true, wantPod: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Now()
			fp, img := timelinePodsFixture()
			fp.err = tc.podErr
			h := startTimelinePodsHarness(t, timelineBreakGuide(now), fp, img, tc.breakClips)
			seedChannel(t, h.Store, "ch1", "Kettleton Classics", 1, "internal")
			seedPodClips(t, h.Store)

			wire := getTimelineWire(t, h.Server, "ch1")
			if len(wire.Airings) != 2 {
				t.Fatalf("got %d airings, want the full strip", len(wire.Airings))
			}
			if got := len(wire.Airings[1].Pod) > 0; got != tc.wantPod {
				t.Fatalf("break pod present = %v, want %v", got, tc.wantPod)
			}
			if !tc.wantPod {
				return
			}
			var pod podWire
			if err := json.Unmarshal(wire.Airings[1].Pod, &pod); err != nil {
				t.Fatal(err)
			}
			if len(pod.Entries) != 2 {
				t.Fatalf("pod has %d entries, want 2", len(pod.Entries))
			}
			for i, e := range pod.Entries {
				if _, ok := e["thumbImage"]; ok {
					t.Errorf("entry %d carries thumbImage after the lookup failed", i)
				}
			}
		})
	}
}

// No pod assembler wired: breaks are plain blocks, no error.
func TestChannelTimeline_NoPodsWired(t *testing.T) {
	now := time.Now()
	_, img := timelinePodsFixture()
	h := startTimelinePodsHarness(t, timelineBreakGuide(now), nil, img, false)
	seedChannel(t, h.Store, "ch1", "Kettleton Classics", 1, "internal")

	wire := getTimelineWire(t, h.Server, "ch1")
	if len(wire.Airings) != 2 || len(wire.Airings[1].Pod) != 0 {
		t.Fatalf("airings = %+v, want a break with no pod", wire.Airings)
	}
}
