package proposaloutlook_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/binder"
	"github.com/loomarr/loomarr/internal/channels"
	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/proposal"
	"github.com/loomarr/loomarr/internal/proposaloutlook"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/suggest"
	"github.com/loomarr/loomarr/internal/testkit"
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
	body := proposal.Proposal{
		Intent: proposal.Intent{Description: "An action channel"},
		Lineup: []proposal.ProposalItem{
			{MediaType: provision.Movie, TMDBID: 90000001, Name: "One", InLibrary: true, LibraryItemID: "movie-one"},
			{MediaType: provision.Movie, TMDBID: 90000002, Name: "Unknown runtime", InLibrary: true, LibraryItemID: "movie-two", RuntimeMinutes: 180},
		},
		Acquisitions: []proposal.ProposalItem{{MediaType: provision.Movie, TMDBID: 90000003, Name: "Missing", RuntimeMinutes: 200}},
	}
	p := store.Proposal{ID: "proposal-one", JobID: "job-one", Status: "submitted", Body: body}
	// A proposal always has the job that produced it; the planner refuses one whose job is gone.
	if err := st.CreateJob(ctx, store.Job{ID: p.JobID, Kind: "suggest", Status: "done"}); err != nil {
		t.Fatal(err)
	}
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
	for _, key := range []provision.Key{"movie:tmdb:90000001", "movie:tmdb:90000002", "movie:tmdb:90000003"} {
		if _, err := st.GetTitle(ctx, key); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("outlook wrote title %s: %v", key, err)
		}
	}
	t.Run("owned review addition uses the same resolved placement as approval", func(t *testing.T) {
		media.Metadata = map[string]library.ItemMetadata{"movie-added": {RuntimeMs: 80 * minute}}
		addition := proposal.ProposalItem{
			MediaType: provision.Movie, TMDBID: 90000004, Name: "Owned addition",
			InLibrary: true, LibraryItemID: "movie-added",
		}
		resolver := &testkit.ApprovalAdditionResolver[proposal.ProposalItem]{Results: []testkit.ApprovalAdditionResolution[proposal.ProposalItem]{
			{Item: addition, Owned: true},
		}}
		ownedService := proposaloutlook.New(proposaloutlook.Config{
			Titles: st, Library: media, Episodes: media.ResolveEpisodes,
			Planner: planner, Preview: engine, Additions: resolver,
		})
		ownedProposal := p
		ownedProposal.Body = proposal.Proposal{Intent: proposal.Intent{Description: "An owned movie"}, Lineup: []proposal.ProposalItem{}}
		got, err := ownedService.Assess(ctx, ownedProposal, &suggest.ApprovalEdit{Add: []proposal.ProposalItem{
			{MediaType: provision.Movie, TMDBID: 90000004, Name: "Owned addition", InLibrary: false},
		}})
		if err != nil {
			t.Fatal(err)
		}
		if got.State != "ready" || got.Programs != 1 || got.MissingAcquisitions != 0 || got.UniqueRuntimeMs != 80*minute {
			t.Fatalf("owned addition outlook = %+v, want immediately schedulable without acquisition", got)
		}
	})
	t.Run("series airing selector controls actual episode runway", func(t *testing.T) {
		media.Metadata = map[string]library.ItemMetadata{"show": {}}
		media.Episodes = map[string][]schedule.ResolvedProgram{"show": {
			{LibraryItemID: "season-one", DurationMs: 90 * minute, Season: 1, Episode: 1},
			{LibraryItemID: "season-two", DurationMs: 22 * minute, Season: 2, Episode: 1},
			{LibraryItemID: "season-three", DurationMs: 90 * minute, Season: 3, Episode: 1},
		}}
		series := proposal.Proposal{Intent: proposal.Intent{Description: "A series channel"}, Lineup: []proposal.ProposalItem{
			{MediaType: provision.Series, TMDBID: 90000044, TVDBID: 90000044, Name: "Show", InLibrary: true, LibraryItemID: "show", SeasonMin: 2, SeasonMax: 2},
		}}
		pending := p
		pending.Body = series
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
		waiting.Body = proposal.Proposal{Intent: proposal.Intent{Description: "A movie channel"}, Acquisitions: []proposal.ProposalItem{
			{MediaType: provision.Movie, TMDBID: 90000003, Name: "Missing", RuntimeMinutes: 200},
		}}
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
