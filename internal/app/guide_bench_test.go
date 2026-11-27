package app

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/filler/clips"
	"github.com/loomarr/loomarr/internal/library/provision"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
	"github.com/loomarr/loomarr/internal/playout/tunarr/tunarrtest"
	"github.com/loomarr/loomarr/internal/testkit"
)

// guideFixtureShape is the install the Guide benchmark reproduces (#2147 step 9.4): the real
// install runs 8 live sequential channels on internal playout, five with about 128 desired slots
// and three with about 31, with filler breaks on.
type guideFixtureShape struct {
	large, small             int // channels of each size
	largeTitles, smallTitles int // lineup entries per channel of each size
	clips                    int
}

var guideInstallShape = guideFixtureShape{large: 5, small: 3, largeTitles: 80, smallTitles: 20, clips: 400}

// guideFixture boots the real composition root over a migrated SQLite store, seeds the channels
// through the reconcile route, and returns a server and the admin bearer token.
func guideFixture(b *testing.B, shape guideFixtureShape) *httptest.Server {
	b.Helper()
	const token = "guide-bench-token"
	b.Setenv("API_TOKEN", token)
	b.Setenv("PLAYOUT_BACKEND", "")
	b.Setenv("TUNARR_URL", "")
	b.Setenv("CHANNEL_RECONCILE_EVERY", "9999h")
	mediaServer := testkit.NewMediaServer(b)
	b.Cleanup(mediaServer.Close)
	b.Setenv("LIBRARY_FLAVOR", "emby")
	b.Setenv("LIBRARY_URL", mediaServer.URL)
	b.Setenv("LIBRARY_TOKEN", mediaServer.AdminToken)

	ctx, cancel := context.WithCancel(context.Background())
	b.Cleanup(cancel)
	st := testkit.MigratedSQLiteStore(b)
	application, err := Build(ctx, st, slog.New(slog.DiscardHandler), Overrides{Programmer: tunarrtest.NewTunarr(), DataDir: b.TempDir()})
	if err != nil {
		b.Fatalf("Build: %v", err)
	}
	b.Cleanup(func() { _ = application.Shutdown(context.Background()) })
	srv := httptest.NewServer(application.Handler())
	b.Cleanup(srv.Close)

	now := time.Now().UTC()
	for i := range shape.clips {
		hash := fmt.Sprintf("%064x", i+1)
		role := clips.Commercial
		if i%10 == 0 {
			role = clips.StationID
		}
		c := clips.Clip{
			Hash: hash, Path: "ads/" + hash + ".mp4", Name: "Spot " + strconv.Itoa(i),
			Role: role, Era: 1970 + (i%4)*10, Audience: clips.General,
			DurationMs: int64(15_000 + (i%4)*15_000), TunarrProgramID: "tp-" + hash,
		}
		if err := st.UpsertClip(ctx, clipstore.Clip{Clip: c, UpdatedAt: now}); err != nil {
			b.Fatal(err)
		}
	}

	titleID := 90_100_000
	// A large channel airs half-hour episodes, so a 24h rolling window holds about 128 slots once
	// breaks sit between them; a small one airs features and holds about 31.
	seedChannel := func(n, titles, shortest, spread int) {
		id := fmt.Sprintf("bench-%02d", n)
		var lineup []schedule.LineupEntry
		for j := range titles {
			titleID++
			title := provision.Title{MediaType: provision.Movie, TMDBID: titleID, Name: fmt.Sprintf("Feature %d-%d", n, j), Year: 1980 + j%20}
			key, err := title.Key()
			if err != nil {
				b.Fatal(err)
			}
			lib := "lib-" + strconv.Itoa(titleID)
			testkit.SaveLibraryItems(b, st, lib)
			if err := st.UpsertTitle(ctx, provision.Record{Key: key, Title: title, State: provision.Available, LibraryID: lib}); err != nil {
				b.Fatal(err)
			}
			minutes := shortest + (j*7)%spread
			lineup = append(lineup, schedule.LineupEntry{Key: key, Title: title.Name, Year: title.Year, DurationMs: int64(minutes) * int64(time.Minute/time.Millisecond)})
		}
		ch := channelstore.Channel{Lineup: lineup}
		ch.ID, ch.Name, ch.Number = id, "Bench "+strconv.Itoa(n), n
		ch.Strategy, ch.Status = schedule.Sequential, schedule.StatusBuilding
		if _, err := st.SaveChannel(ctx, ch); err != nil {
			b.Fatal(err)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL+"/v1/channels/"+id+"/reconcile", nil)
		if err != nil {
			b.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			b.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			b.Fatalf("reconcile %s = %d", id, resp.StatusCode)
		}
		got, err := st.GetChannel(ctx, id)
		if err != nil {
			b.Fatal(err)
		}
		if got.Status != schedule.StatusLive {
			b.Fatalf("%s status = %s, want live", id, got.Status)
		}
		b.Logf("%s: %d desired slots", id, len(got.Desired))
	}
	n := 1
	for range shape.large {
		seedChannel(n, shape.largeTitles, 20, 6)
		n++
	}
	for range shape.small {
		seedChannel(n, shape.smallTitles, 85, 30)
		n++
	}
	return srv
}

// serverTimingPhase reads one phase's duration out of a Server-Timing header.
func serverTimingPhase(header, phase string) float64 {
	for part := range strings.SplitSeq(header, ",") {
		name, rest, _ := strings.Cut(strings.TrimSpace(part), ";")
		if name != phase {
			continue
		}
		for attr := range strings.SplitSeq(rest, ";") {
			if v, ok := strings.CutPrefix(strings.TrimSpace(attr), "dur="); ok {
				f, _ := strconv.ParseFloat(v, 64)
				return f
			}
		}
	}
	return 0
}

func benchmarkGuide(b *testing.B, shape guideFixtureShape) {
	srv := guideFixture(b, shape)
	get := func() string {
		req, err := http.NewRequest(http.MethodGet, srv.URL+"/v1/guide", nil)
		if err != nil {
			b.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer guide-bench-token")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			b.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			b.Fatalf("GET /v1/guide = %d", resp.StatusCode)
		}
		return resp.Header.Get("Server-Timing")
	}
	get() // warm the caches a polling grid keeps warm
	var timelines, totals []float64
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		h := get()
		timelines = append(timelines, serverTimingPhase(h, "timelines"))
		totals = append(totals, serverTimingPhase(h, "total"))
	}
	b.StopTimer()
	slices.Sort(timelines)
	slices.Sort(totals)
	p := func(xs []float64, q float64) float64 { return xs[min(len(xs)-1, int(float64(len(xs))*q))] }
	b.ReportMetric(p(timelines, 0.5), "timelines-p50-ms")
	b.ReportMetric(p(timelines, 0.95), "timelines-p95-ms")
	b.ReportMetric(p(totals, 0.5), "total-p50-ms")
	b.ReportMetric(p(totals, 0.95), "total-p95-ms")
}

// BenchmarkGuide measures GET /v1/guide's default window through the real composition root at the
// install's shape and at twice it.
func BenchmarkGuide(b *testing.B) {
	b.Run("install", func(b *testing.B) { benchmarkGuide(b, guideInstallShape) })
	double := guideInstallShape
	double.large, double.small, double.clips = 10, 6, 800
	b.Run("double", func(b *testing.B) { benchmarkGuide(b, double) })
}
