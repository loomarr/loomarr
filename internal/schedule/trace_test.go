package schedule_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
)

func traceHas(facts []schedule.ScheduleFact, stage schedule.ScheduleStage, reason schedule.ScheduleReason, key provision.Key) bool {
	for _, fact := range facts {
		if fact.Stage == stage && fact.Reason == reason && fact.Key == key {
			return true
		}
	}
	return false
}

func TestComputeDesiredAt_TraceComesFromTheDecisionSeams(t *testing.T) {
	entries := []schedule.LineupEntry{
		ratedEntry("movie:tmdb:1", "Available", "TV-Y7", 1994),
		ratedEntry("movie:tmdb:2", "Coming soon", "TV-Y7", 1995),
		ratedEntry("movie:tmdb:3", "Too adult", "TV-MA", 1996),
	}
	policy := schedule.ChannelPolicy{ProposalPolicy: schedule.ProposalPolicy{
		Audience: schedule.AudiencePolicy{Ceiling: "TV-Y7"},
	}}

	got := schedule.ComputeDesiredAt(policyChannel(), entries,
		mapAvail{"movie:tmdb:1": "lib-1"}, schedule.PodFill, policy, time.Time{})

	if got.Trace.Version != schedule.ScheduleTraceVersion {
		t.Fatalf("trace version = %d, want %d", got.Trace.Version, schedule.ScheduleTraceVersion)
	}
	if got.Trace.Ordering != schedule.OrderSequential || got.Trace.Seed != 0 {
		t.Fatalf("trace ordering metadata = %+v", got.Trace)
	}
	checks := []struct {
		stage  schedule.ScheduleStage
		reason schedule.ScheduleReason
		key    provision.Key
	}{
		{schedule.StageHardFilter, schedule.ReasonEligible, "movie:tmdb:1"},
		{schedule.StageAvailability, schedule.ReasonAvailable, "movie:tmdb:1"},
		{schedule.StagePlacement, schedule.ReasonSequential, "movie:tmdb:1"},
		{schedule.StageAvailability, schedule.ReasonUnavailablePodFill, "movie:tmdb:2"},
		{schedule.StageHardFilter, schedule.ReasonOverCeiling, "movie:tmdb:3"},
	}
	for _, check := range checks {
		if !traceHas(got.Trace.Facts, check.stage, check.reason, check.key) {
			t.Errorf("trace missing %s/%s for %s: %+v", check.stage, check.reason, check.key, got.Trace.Facts)
		}
	}
	if got.Trace.FactTotal != len(got.Trace.Facts) || got.Trace.RecordedTotal != len(got.Trace.Facts) || got.Trace.Truncated {
		t.Fatalf("trace bounds = facts:%d total:%d recorded:%d truncated:%v", len(got.Trace.Facts), got.Trace.FactTotal, got.Trace.RecordedTotal, got.Trace.Truncated)
	}
}

func TestComputeDesiredAt_TraceExplainsEpisodeSelection(t *testing.T) {
	key := provision.Key("series:tmdb:456")
	episodes := make([]schedule.ResolvedProgram, 0, 8)
	for episode, rating := range []float64{6.1, 9.4, 6.3, 8.9, 6.0, 9.1, 6.2, 8.8} {
		episodes = append(episodes, schedule.ResolvedProgram{
			LibraryItemID: fmt.Sprintf("ep-%d", episode+1), Title: fmt.Sprintf("Episode %d", episode+1),
			DurationMs: 22 * 60 * 1000, Season: 1, Episode: episode + 1, CommunityRating: rating,
		})
	}
	got := schedule.ComputeDesiredAt(policyChannel(), []schedule.LineupEntry{{
		Key: key, Title: "The Simpsons",
		EpisodeSelection: schedule.EpisodeSelection{Mode: schedule.EpisodeHighlights},
	}}, newSeriesAvail(map[string][]schedule.ResolvedProgram{string(key): episodes}), schedule.PodFill,
		schedule.ChannelPolicy{}, time.Time{})

	if !traceHas(got.Trace.Facts, schedule.StageEpisodeSelection, schedule.ReasonHighlightsSelected, key) {
		t.Fatalf("trace does not explain selected highlights: %+v", got.Trace.Facts)
	}
	if !traceHas(got.Trace.Facts, schedule.StageEpisodeSelection, schedule.ReasonHighlightsOmitted, key) {
		t.Fatalf("trace does not explain omitted highlights: %+v", got.Trace.Facts)
	}
}

func TestComputeDesiredAt_TraceNamesEditorialFullRunFallback(t *testing.T) {
	key := provision.Key("series:tmdb:456")
	got := schedule.ComputeDesiredAt(policyChannel(), []schedule.LineupEntry{{
		Key: key, Title: "The Simpsons",
		EpisodeSelection: schedule.EpisodeSelection{Mode: schedule.EpisodeHighlights},
	}}, newSeriesAvail(map[string][]schedule.ResolvedProgram{string(key): {
		{LibraryItemID: "ep-1", Title: "Episode 1", Season: 1, Episode: 1},
	}}), schedule.PodFill, schedule.ChannelPolicy{}, time.Time{})

	if !traceHas(got.Trace.Facts, schedule.StageEpisodeSelection, schedule.ReasonFullRunFallback, key) {
		t.Fatalf("trace claims no explicit fallback for insufficient highlight evidence: %+v", got.Trace.Facts)
	}
}

func TestComputeDesiredAt_TraceExplainsWindowPlacement(t *testing.T) {
	const entriesN = 6
	entries := make([]schedule.LineupEntry, 0, entriesN)
	avail := make(mapAvail, entriesN)
	for i := range entriesN {
		key := provision.Key(fmt.Sprintf("movie:tmdb:%d", i+1))
		entries = append(entries, schedule.LineupEntry{Key: key, Title: fmt.Sprintf("Movie %d", i+1), DurationMs: int64(time.Hour / time.Millisecond)})
		avail[key] = fmt.Sprintf("lib-%d", i+1)
	}
	policy := schedule.ChannelPolicy{Window: schedule.Duration(time.Hour)}
	got := schedule.ComputeDesiredAt(policyChannel(), entries, avail, schedule.PodFill, policy,
		time.Unix(int64(4*time.Hour/time.Second), 0))

	if got.Trace.WindowMs != int64(time.Hour/time.Millisecond) || got.Trace.WindowIndex != 4 {
		t.Fatalf("window metadata = %dms/index %d", got.Trace.WindowMs, got.Trace.WindowIndex)
	}
	if !traceHas(got.Trace.Facts, schedule.StagePlacement, schedule.ReasonWindowedOut, "movie:tmdb:1") {
		t.Fatalf("trace does not explain the rotated-out deck head")
	}
	if got.Trace.Truncated {
		t.Fatal("small trace unexpectedly truncated")
	}
}

func TestComputeDesiredAt_TraceIsBounded(t *testing.T) {
	const entriesN = 600
	entries := make([]schedule.LineupEntry, 0, entriesN)
	avail := make(mapAvail, entriesN)
	for i := range entriesN {
		key := provision.Key(fmt.Sprintf("movie:tmdb:%d", i+1))
		entries = append(entries, schedule.LineupEntry{Key: key, Title: fmt.Sprintf("Movie %d", i+1)})
		avail[key] = fmt.Sprintf("lib-%d", i+1)
	}
	got := schedule.ComputeDesiredAt(policyChannel(), entries, avail, schedule.PodFill,
		schedule.ChannelPolicy{}, time.Time{})

	if len(got.Trace.Facts) != schedule.ScheduleTraceMaxFacts || got.Trace.RecordedTotal != schedule.ScheduleTraceMaxFacts || !got.Trace.Truncated {
		t.Fatalf("bounded trace = len:%d recorded:%d truncated:%v", len(got.Trace.Facts), got.Trace.RecordedTotal, got.Trace.Truncated)
	}
	if got.Trace.FactTotal <= got.Trace.RecordedTotal {
		t.Fatalf("fact total %d must retain omitted-count evidence above recorded %d", got.Trace.FactTotal, got.Trace.RecordedTotal)
	}
}
