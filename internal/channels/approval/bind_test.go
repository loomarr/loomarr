package approval_test

import (
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels/approval"
	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/library/provision"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
	"github.com/loomarr/loomarr/internal/platform/store/proposalstore"
	"github.com/loomarr/loomarr/internal/proposals"
)

// Bind is a pure function: every case below states its whole input and its whole expected
// output. There is no store, clock or logger to arrange.

var bindNow = time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)

func item(n int, name string) proposals.ProposalItem {
	return proposals.ProposalItem{MediaType: provision.Movie, TMDBID: 90000000 + n, Name: name}
}

func key(n int) provision.Key { return provision.Key("movie:tmdb:" + strconv.Itoa(90000000+n)) }

func existingChannel(lineup ...schedule.LineupEntry) channelstore.Channel {
	ch := channelstore.Channel{Lineup: lineup}
	ch.ID = "ch_existing"
	ch.IntentRef = "job-1"
	ch.RequestedBy = "original-requester"
	ch.Name = "Operator Name"
	ch.Number = 7
	ch.Strategy = schedule.Sequential
	ch.Status = schedule.StatusLive
	return ch
}

func proposalFrom(approvedBy string, body proposals.Proposal) proposalstore.Proposal {
	return proposalstore.Proposal{ID: "p1", JobID: "job-1", Status: "approved", CreatedBy: "requester", ApprovedBy: approvedBy, Body: body}
}

func titles(entries []schedule.LineupEntry) []string {
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Title)
	}
	return out
}

func TestBindLineup(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		in          approval.BindInput
		wantTitles  []string
		wantRetired []string
		wantDropped []string
	}{
		{
			name: "first approval lists the lineup then the acquisitions and collapses a title in both",
			in: approval.BindInput{
				Proposal: proposalFrom("admin", proposals.Proposal{
					Lineup:       []proposals.ProposalItem{item(1, "One"), item(2, "Two")},
					Acquisitions: []proposals.ProposalItem{item(2, "Two again"), item(3, "Three")},
				}),
				Number: 1, Now: bindNow,
			},
			wantTitles: []string{"One", "Two", "Three"},
		},
		{
			name: "a human approval replaces the lineup, removing what the person left out",
			in: approval.BindInput{
				Proposal: proposalFrom("admin", proposals.Proposal{Lineup: []proposals.ProposalItem{item(2, "Two")}}),
				Existing: existingChannel(schedule.LineupEntry{Key: key(1), Title: "One"}, schedule.LineupEntry{Key: key(2), Title: "Two"}),
				Now:      bindNow,
			},
			wantTitles: []string{"Two"},
		},
		{
			name: "a human approval ignores retirement and availability hints",
			in: approval.BindInput{
				Proposal: proposalFrom("admin", proposals.Proposal{
					Lineup:  []proposals.ProposalItem{item(2, "Two")},
					Retired: []provision.Key{key(2)},
				}),
				Existing:    existingChannel(schedule.LineupEntry{Key: key(1), Title: "One"}),
				Unavailable: map[provision.Key]bool{key(1): true},
				Now:         bindNow,
			},
			wantTitles: []string{"Two"},
		},
		{
			name: "auto-curate keeps an available title the model did not re-pick",
			in: approval.BindInput{
				Proposal: proposalFrom(proposalstore.AutoCuratedBy, proposals.Proposal{Lineup: []proposals.ProposalItem{item(2, "Two")}}),
				Existing: existingChannel(schedule.LineupEntry{Key: key(1), Title: "One"}),
				Now:      bindNow,
			},
			wantTitles: []string{"One", "Two"},
		},
		{
			name: "auto-curate drops a title the intent excludes, ignoring case and padding",
			in: approval.BindInput{
				Proposal: proposalFrom(proposalstore.AutoCuratedBy, proposals.Proposal{
					Intent: proposals.Intent{MustExclude: []string{"  oNe "}},
					Lineup: []proposals.ProposalItem{item(2, "Two")},
				}),
				Existing: existingChannel(schedule.LineupEntry{Key: key(1), Title: "One"}),
				Now:      bindNow,
			},
			wantTitles:  []string{"Two"},
			wantDropped: []string{"One"},
		},
		{
			name: "auto-curate drops a title that left the library pipeline",
			in: approval.BindInput{
				Proposal:    proposalFrom(proposalstore.AutoCuratedBy, proposals.Proposal{Lineup: []proposals.ProposalItem{item(2, "Two")}}),
				Existing:    existingChannel(schedule.LineupEntry{Key: key(1), Title: "One"}, schedule.LineupEntry{Key: key(3), Title: "Three"}),
				Unavailable: map[provision.Key]bool{key(1): true},
				Now:         bindNow,
			},
			wantTitles:  []string{"Three", "Two"},
			wantDropped: []string{"One"},
		},
		{
			name: "auto-curate applies the turnstile's retirement to an available title",
			in: approval.BindInput{
				Proposal: proposalFrom(proposalstore.AutoCuratedBy, proposals.Proposal{
					Lineup:  []proposals.ProposalItem{item(2, "Two")},
					Retired: []provision.Key{key(1)},
				}),
				Existing: existingChannel(schedule.LineupEntry{Key: key(1), Title: "One"}),
				Now:      bindNow,
			},
			wantTitles:  []string{"Two"},
			wantRetired: []string{"One"},
		},
		{
			name: "auto-curate with no channel yet binds like a first approval",
			in: approval.BindInput{
				Proposal: proposalFrom(proposalstore.AutoCuratedBy, proposals.Proposal{Lineup: []proposals.ProposalItem{item(2, "Two")}}),
				Number:   3, Now: bindNow,
			},
			wantTitles: []string{"Two"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := approval.Bind(tc.in)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(sorted(titles(got.Channel.Lineup)), sorted(tc.wantTitles)) {
				t.Fatalf("lineup = %v, want %v", titles(got.Channel.Lineup), tc.wantTitles)
			}
			if !reflect.DeepEqual(sorted(titles(got.Retired)), sorted(tc.wantRetired)) {
				t.Fatalf("retired = %v, want %v", titles(got.Retired), tc.wantRetired)
			}
			if !reflect.DeepEqual(sorted(titles(got.Dropped)), sorted(tc.wantDropped)) {
				t.Fatalf("dropped = %v, want %v", titles(got.Dropped), tc.wantDropped)
			}
		})
	}
}

func sorted(in []string) []string {
	out := append([]string{}, in...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j] < out[j-1]; j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func TestBindChannelIdentity(t *testing.T) {
	t.Parallel()
	body := proposals.Proposal{
		ChannelName: "Saturday Cartoons",
		Intent:      proposals.Intent{Description: "kids cartoons", ChannelName: "never stored", RefineText: "never stored"},
		Lineup:      []proposals.ProposalItem{item(1, "One")},
	}
	tests := []struct {
		name  string
		in    approval.BindInput
		check func(t *testing.T, ch channelstore.Channel)
	}{
		{
			name: "a new channel gets an id derived from its job, the given number, and the requester",
			in:   approval.BindInput{Proposal: proposalFrom("admin", body), Number: 4, Now: bindNow},
			check: func(t *testing.T, ch channelstore.Channel) {
				if !strings.HasPrefix(ch.ID, "ch_") || len(ch.ID) != len("ch_")+16 {
					t.Errorf("id = %q, want ch_ + 16 hex digits", ch.ID)
				}
				if ch.IntentRef != "job-1" || ch.RequestedBy != "requester" || ch.Number != 4 {
					t.Errorf("channel = %+v, want intent job-1, requester, number 4", ch)
				}
				if ch.Name != "Saturday Cartoons" || ch.Strategy != schedule.Sequential {
					t.Errorf("name %q strategy %v", ch.Name, ch.Strategy)
				}
			},
		},
		{
			name: "a new channel falls back to the intent description for its name",
			in: approval.BindInput{
				Proposal: proposalFrom("admin", proposals.Proposal{Intent: proposals.Intent{Description: "90s action"}, Lineup: body.Lineup}),
				Number:   1, Now: bindNow,
			},
			check: func(t *testing.T, ch channelstore.Channel) {
				if ch.Name != "90s action" {
					t.Errorf("name = %q", ch.Name)
				}
			},
		},
		{
			name: "an existing channel keeps its id, number, name and requester",
			in:   approval.BindInput{Proposal: proposalFrom("admin", body), Existing: existingChannel(), Number: 99, Now: bindNow},
			check: func(t *testing.T, ch channelstore.Channel) {
				if ch.ID != "ch_existing" || ch.Number != 7 || ch.Name != "Operator Name" || ch.RequestedBy != "original-requester" {
					t.Errorf("existing identity changed: %+v", ch)
				}
			},
		},
		{
			name: "the channel is building, due now, and stores the request without its run inputs",
			in:   approval.BindInput{Proposal: proposalFrom("admin", body), Number: 1, Now: bindNow},
			check: func(t *testing.T, ch channelstore.Channel) {
				if ch.Status != schedule.StatusBuilding || !ch.ReconcileDeadline.Equal(bindNow) {
					t.Errorf("status %v deadline %v, want building and due at %v", ch.Status, ch.ReconcileDeadline, bindNow)
				}
				if strings.Contains(ch.IntentJSON, "never stored") || !strings.Contains(ch.IntentJSON, "kids cartoons") {
					t.Errorf("intent json = %s", ch.IntentJSON)
				}
				if len(ch.Lineup) != 1 || !ch.Lineup[0].AddedAt.Equal(bindNow) {
					t.Errorf("lineup = %+v, want entries stamped at %v", ch.Lineup, bindNow)
				}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := approval.Bind(tc.in)
			if err != nil {
				t.Fatal(err)
			}
			tc.check(t, got.Channel)
		})
	}
}

func TestBindRefusals(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   approval.BindInput
	}{
		{"a proposal with no job has no channel to bind to", approval.BindInput{
			Proposal: proposalstore.Proposal{ID: "p1", Body: proposals.Proposal{Lineup: []proposals.ProposalItem{item(1, "One")}}}, Number: 1, Now: bindNow,
		}},
		{"a pick with no external id cannot be keyed", approval.BindInput{
			Proposal: proposalFrom("admin", proposals.Proposal{Lineup: []proposals.ProposalItem{{MediaType: provision.Movie, Name: "No id"}}}), Number: 1, Now: bindNow,
		}},
		{"a new channel needs a positive number", approval.BindInput{
			Proposal: proposalFrom("admin", proposals.Proposal{Lineup: []proposals.ProposalItem{item(1, "One")}}), Now: bindNow,
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got, err := approval.Bind(tc.in); err == nil {
				t.Fatalf("Bind succeeded: %+v", got.Channel)
			}
		})
	}
}

func TestBindIsDeterministicAndLeavesItsInputAlone(t *testing.T) {
	t.Parallel()
	existing := existingChannel(schedule.LineupEntry{Key: key(1), Title: "One"})
	in := approval.BindInput{
		Proposal:    proposalFrom(proposalstore.AutoCuratedBy, proposals.Proposal{Lineup: []proposals.ProposalItem{item(2, "Two")}, Retired: []provision.Key{key(1)}}),
		Existing:    existing,
		Unavailable: map[provision.Key]bool{key(1): true},
		Now:         bindNow,
	}
	first, err := approval.Bind(in)
	if err != nil {
		t.Fatal(err)
	}
	second, err := approval.Bind(in)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("same input, different binding:\n%+v\n%+v", first, second)
	}
	if len(existing.Lineup) != 1 || existing.Lineup[0].Title != "One" || len(in.Unavailable) != 1 {
		t.Fatalf("Bind changed its input: %+v", existing.Lineup)
	}
}
