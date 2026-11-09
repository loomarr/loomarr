package outlook_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels"
	"github.com/loomarr/loomarr/internal/channels/approval"
	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/library"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/proposalstore"
	"github.com/loomarr/loomarr/internal/proposals"
	"github.com/loomarr/loomarr/internal/proposals/accept"
	"github.com/loomarr/loomarr/internal/proposals/outlook"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/testkit"
	"github.com/loomarr/loomarr/internal/testkit/outlookfixture"
)

// A find-more revision of a manual channel keeps the operator's picks out of the new proposal:
// the review carries them as additions and drops the new suggestions it has not chosen. The
// estimate must still cover the same three library picks it covered before the revision (#1943).
func TestOutlookAfterFindMoreRevisionKeepsTheSelectedLibraryPicks(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, testkit.MigratedSQLiteDSN(t), true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	minute := int64(time.Minute / time.Millisecond)
	media := &outlookfixture.OutlookLibrary{Metadata: map[string]library.ItemMetadata{
		"lib-a": {RuntimeMs: 100 * minute}, "lib-b": {RuntimeMs: 110 * minute}, "lib-c": {RuntimeMs: 120 * minute},
		"lib-new-1": {RuntimeMs: 95 * minute}, "lib-new-2": {RuntimeMs: 105 * minute},
	}}
	picks := []proposals.ProposalItem{
		{MediaType: provision.Movie, TMDBID: 90000101, Name: "Pick A", Year: 1990, InLibrary: true, LibraryItemID: "lib-a"},
		{MediaType: provision.Movie, TMDBID: 90000102, Name: "Pick B", Year: 1991, InLibrary: true, LibraryItemID: "lib-b"},
		{MediaType: provision.Movie, TMDBID: 90000103, Name: "Pick C", Year: 1992, InLibrary: true, LibraryItemID: "lib-c"},
	}
	manual := proposalstore.Proposal{ID: "proposal-manual", JobID: "job-manual", Status: "submitted", Body: proposals.Proposal{
		Intent: proposals.Intent{Description: "Movie night"}, ChannelName: "Movie night", Manual: true, Lineup: picks,
	}}
	if err := st.CreateJob(ctx, proposalstore.Job{ID: manual.JobID, Kind: "suggest", Status: "done"}); err != nil {
		t.Fatal(err)
	}
	resolver := &testkit.ApprovalAdditionResolver[proposals.ProposalItem]{}
	for _, pick := range picks {
		resolver.Results = append(resolver.Results, testkit.ApprovalAdditionResolution[proposals.ProposalItem]{Item: pick, Owned: true})
	}
	svc := outlook.New(outlook.Config{
		Titles: st, Library: media, Episodes: media.ResolveEpisodes,
		Planner:   approval.New(st, nil, nil, nil),
		Preview:   channels.New(st, nil, nil, nil, channels.Config{}, time.Now, nil),
		Additions: resolver,
	})

	before, err := svc.Assess(ctx, manual, nil)
	if err != nil {
		t.Fatal(err)
	}
	if before.State != "ready" || before.ScheduledTitles != 3 {
		t.Fatalf("manual proposal outlook = %+v, want ready with 3 scheduled titles", before)
	}

	// The revision replaces the proposal on the same job with only the new titles.
	current := make([]proposals.LineupContext, 0, len(picks))
	for _, pick := range picks {
		key, _ := pick.Key()
		current = append(current, proposals.LineupContext{Name: pick.Name, Year: pick.Year, Key: string(key)})
	}
	revised := proposalstore.Proposal{ID: "proposal-revised", JobID: manual.JobID, Status: "submitted", Body: proposals.Proposal{
		Intent: proposals.Intent{
			Description:   "Movie night",
			CurrentLineup: current,
			RefineText:    "Find 6–8 additional titles that match this brief. Do not repeat or replace the selected lineup.",
		},
		ChannelName: "Movie night",
		Lineup: []proposals.ProposalItem{
			{MediaType: provision.Movie, TMDBID: 90000201, Name: "New One", InLibrary: true, LibraryItemID: "lib-new-1", EditorialRole: proposals.EditorialDiscovery},
			{MediaType: provision.Movie, TMDBID: 90000202, Name: "New Two", InLibrary: true, LibraryItemID: "lib-new-2", EditorialRole: proposals.EditorialDiscovery},
		},
	}}
	edit := &accept.ApprovalEdit{
		DropKeys: []provision.Key{"movie:tmdb:90000201", "movie:tmdb:90000202"},
		Add:      picks,
	}
	after, err := svc.Assess(ctx, revised, edit)
	if err != nil {
		t.Fatal(err)
	}
	if after.State != "ready" || after.ScheduledTitles != 3 || after.UniqueRuntimeMs != before.UniqueRuntimeMs {
		t.Fatalf("outlook after the find-more revision = %+v, want the same 3 scheduled picks as before (%+v)", after, before)
	}
}

func TestOutlookUsesActualScheduleWithoutApprovingOrCreatingAvailability(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, testkit.MigratedSQLiteDSN(t), true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	minute := int64(time.Minute / time.Millisecond)
	media := &outlookfixture.OutlookLibrary{Metadata: map[string]library.ItemMetadata{"movie-one": {RuntimeMs: 90 * minute}, "movie-two": {RuntimeMs: 0}}}
	body := proposals.Proposal{
		Intent: proposals.Intent{Description: "An action channel"},
		Lineup: []proposals.ProposalItem{
			{MediaType: provision.Movie, TMDBID: 90000001, Name: "One", InLibrary: true, LibraryItemID: "movie-one"},
			{MediaType: provision.Movie, TMDBID: 90000002, Name: "Unknown runtime", InLibrary: true, LibraryItemID: "movie-two", RuntimeMinutes: 180},
		},
		Acquisitions: []proposals.ProposalItem{{MediaType: provision.Movie, TMDBID: 90000003, Name: "Missing", RuntimeMinutes: 200}},
	}
	p := proposalstore.Proposal{ID: "proposal-one", JobID: "job-one", Status: "submitted", Body: body}
	// A proposal always has the job that produced it; the planner refuses one whose job is gone.
	if err := st.CreateJob(ctx, proposalstore.Job{ID: p.JobID, Kind: "suggest", Status: "done"}); err != nil {
		t.Fatal(err)
	}
	planner := approval.New(st, nil, nil, nil)
	engine := channels.New(st, nil, nil, nil, channels.Config{}, time.Now, nil)
	svc := outlook.New(outlook.Config{Titles: st, Library: media, Episodes: media.ResolveEpisodes, Planner: planner, Preview: engine})
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
		addition := proposals.ProposalItem{
			MediaType: provision.Movie, TMDBID: 90000004, Name: "Owned addition",
			InLibrary: true, LibraryItemID: "movie-added",
		}
		resolver := &testkit.ApprovalAdditionResolver[proposals.ProposalItem]{Results: []testkit.ApprovalAdditionResolution[proposals.ProposalItem]{
			{Item: addition, Owned: true},
		}}
		ownedService := outlook.New(outlook.Config{
			Titles: st, Library: media, Episodes: media.ResolveEpisodes,
			Planner: planner, Preview: engine, Additions: resolver,
		})
		ownedProposal := p
		ownedProposal.Body = proposals.Proposal{Intent: proposals.Intent{Description: "An owned movie"}, Lineup: []proposals.ProposalItem{}}
		got, err := ownedService.Assess(ctx, ownedProposal, &accept.ApprovalEdit{Add: []proposals.ProposalItem{
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
		series := proposals.Proposal{Intent: proposals.Intent{Description: "A series channel"}, Lineup: []proposals.ProposalItem{
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
		waiting.Body = proposals.Proposal{Intent: proposals.Intent{Description: "A movie channel"}, Acquisitions: []proposals.ProposalItem{
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
