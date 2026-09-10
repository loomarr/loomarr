package proposaloutlook_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/binder"
	"github.com/loomarr/loomarr/internal/channels"
	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/proposaloutlook"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/suggest"
	"github.com/loomarr/loomarr/internal/testkit/outlookfixture"
)

func TestOutlookUsesActualScheduleWithoutApprovingOrCreatingAvailability(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, "sqlite://"+filepath.Join(t.TempDir(), "outlook.db"), true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	minute := int64(time.Minute / time.Millisecond)
	media := &outlookfixture.OutlookLibrary{Metadata: map[string]library.ItemMetadata{"movie-one": {RuntimeMs: 90 * minute}, "movie-two": {RuntimeMs: 0}}}
	body := suggest.Proposal{
		Intent: suggest.Intent{Description: "An action channel"},
		Lineup: []suggest.ProposalItem{
			{MediaType: provision.Movie, TMDBID: 1, Name: "One", InLibrary: true, LibraryItemID: "movie-one"},
			{MediaType: provision.Movie, TMDBID: 2, Name: "Unknown runtime", InLibrary: true, LibraryItemID: "movie-two", RuntimeMinutes: 180},
		},
		Acquisitions: []suggest.ProposalItem{{MediaType: provision.Movie, TMDBID: 3, Name: "Missing", RuntimeMinutes: 200}},
	}
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	p := store.Proposal{ID: "proposal-one", JobID: "job-one", Status: "submitted", ProposalJSON: string(raw)}
	planner := binder.New(st, nil, nil, nil)
	engine := channels.New(st, nil, nil, nil, channels.Config{}, time.Now, nil)
	svc := proposaloutlook.New(proposaloutlook.Config{Titles: st, Library: media, Episodes: media.ResolveEpisodes, Planner: planner, Preview: engine})
	got, err := svc.Assess(ctx, p, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != "ready" || got.Programs != 1 || got.UniqueRuntimeMs != 90*minute || got.UnknownTitles != 1 || got.MissingAcquisitions != 1 {
		t.Fatalf("catalog guesses and missing acquisitions cannot add immediate runway: %+v", got)
	}
	listed, err := st.ListChannels(ctx)
	if err != nil || len(listed) != 0 {
		t.Fatalf("outlook must not persist a channel: %+v, %v", listed, err)
	}
	for _, key := range []provision.Key{"movie:tmdb:1", "movie:tmdb:2", "movie:tmdb:3"} {
		if _, err := st.GetTitle(ctx, key); err != store.ErrNotFound {
			t.Fatalf("outlook wrote title %s: %v", key, err)
		}
	}
	t.Run("series airing selector controls actual episode runway", func(t *testing.T) {
		media.Metadata = map[string]library.ItemMetadata{"show": {}}
		media.Episodes = map[string][]schedule.ResolvedProgram{"show": {
			{LibraryItemID: "season-one", DurationMs: 90 * minute, Season: 1, Episode: 1},
			{LibraryItemID: "season-two", DurationMs: 22 * minute, Season: 2, Episode: 1},
			{LibraryItemID: "season-three", DurationMs: 90 * minute, Season: 3, Episode: 1},
		}}
		series := suggest.Proposal{Intent: suggest.Intent{Description: "A series channel"}, Lineup: []suggest.ProposalItem{
			{MediaType: provision.Series, TMDBID: 44, TVDBID: 44, Name: "Show", InLibrary: true, LibraryItemID: "show", SeasonMin: 2, SeasonMax: 2},
		}}
		raw, err := json.Marshal(series)
		if err != nil {
			t.Fatal(err)
		}
		pending := p
		pending.ProposalJSON = string(raw)
		got, err := svc.Assess(ctx, pending, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got.Programs != 1 || got.Seasons != 1 || got.UniqueRuntimeMs != 22*minute {
			t.Fatalf("other seasons must not inflate the exact selected lineup: %+v", got)
		}
	})
	t.Run("waiting acquisition has no guessed runway", func(t *testing.T) {
		waiting := p
		waiting.ProposalJSON = `{"intent":{"description":"A movie channel"},"acquisitions":[{"mediaType":"movie","tmdbId":3,"name":"Missing","runtimeMinutes":200}]}`
		got, err := svc.Assess(ctx, waiting, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got.State != "waiting" || got.MissingAcquisitions != 1 || got.UniqueRuntimeMs != 0 || got.FirstRepeatMs != nil {
			t.Fatalf("missing content is waiting, not schedulable: %+v", got)
		}
	})
	pendingChannel, err := planner.PlanSubmittedChannel(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	p.Status = "approved"
	approvedChannel, err := planner.PlanApprovedChannel(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	if pendingChannel.ID != approvedChannel.ID || pendingChannel.Strategy != approvedChannel.Strategy {
		t.Fatalf("preview and approval must share seeded planning identity: %s / %s", pendingChannel.ID, approvedChannel.ID)
	}
	if _, err := planner.PlanSubmittedChannel(ctx, p); err == nil {
		t.Fatal("submitted planning must retain its own status gate")
	}
	p.Status = "submitted"
	if _, err := planner.PlanApprovedChannel(ctx, p); err == nil {
		t.Fatal("preview must not weaken approved planning's status gate")
	}
}
