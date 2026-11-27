package approval_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/channels/approval"
	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/platform/store/proposalstore"
	"github.com/loomarr/loomarr/internal/proposals"
)

func movies(from, n int) []proposals.ProposalItem {
	out := make([]proposals.ProposalItem, n)
	for i := range out {
		out[i] = item(from+i, fmt.Sprintf("Film %d", from+i))
	}
	return out
}

func storedLineup(from, n int) []schedule.LineupEntry {
	out := make([]schedule.LineupEntry, n)
	for i := range out {
		out[i] = schedule.LineupEntry{Key: key(from + i), Title: fmt.Sprintf("Film %d", from+i)}
	}
	return out
}

// The lineup the editor can save holds at most schedule.MaxLineupEntries. Every approval that
// writes one keeps to it, whoever approved: the unattended re-curation adds what fits and reports
// the rest; a person's approval that would replace the lineup with more than fits is refused
// rather than saved as a lineup the editor cannot save.
func TestBindKeepsTheLineupWithinWhatTheEditorCanSave(t *testing.T) {
	t.Parallel()
	ceiling := schedule.MaxLineupEntries
	for name, tc := range map[string]struct {
		by        string
		existing  []schedule.LineupEntry
		lineup    []proposals.ProposalItem
		acquire   []proposals.ProposalItem
		wantLen   int
		wantFull  int // picks reported as turned away
		wantError string
	}{
		"re-curation with room":                          {proposalstore.AutoCuratedBy, storedLineup(0, 10), movies(10, 5), nil, 15, 0, ""},
		"re-curation fills the ceiling exactly":          {proposalstore.AutoCuratedBy, storedLineup(0, ceiling-3), movies(ceiling-3, 3), nil, ceiling, 0, ""},
		"re-curation past the ceiling adds what fits":    {proposalstore.AutoCuratedBy, storedLineup(0, ceiling-3), movies(ceiling-3, 8), nil, ceiling, 5, ""},
		"re-curation on a full lineup adds nothing":      {proposalstore.AutoCuratedBy, storedLineup(0, ceiling), movies(ceiling, 4), nil, ceiling, 4, ""},
		"re-curation counts acquisitions as picks too":   {proposalstore.AutoCuratedBy, storedLineup(0, ceiling-1), movies(ceiling-1, 1), movies(ceiling, 2), ceiling, 2, ""},
		"re-curation of a lineup already over keeps it":  {proposalstore.AutoCuratedBy, storedLineup(0, ceiling+20), movies(ceiling+20, 3), nil, ceiling + 20, 3, ""},
		"a person's approval of exactly the ceiling":     {"admin", storedLineup(0, 5), movies(0, ceiling), nil, ceiling, 0, ""},
		"a person's approval past the ceiling":           {"admin", storedLineup(0, 5), movies(0, ceiling+1), nil, 0, 0, "1000"},
		"a person's approval past it through both lists": {"admin", nil, movies(0, ceiling-1), movies(ceiling, 5), 0, 0, "1000"},
		"a first approval past the ceiling":              {"admin", nil, movies(0, ceiling+3), nil, 0, 0, "1000"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			existing := existingChannel(tc.existing...)
			in := approval.BindInput{
				Proposal: proposalFrom(tc.by, proposals.Proposal{Lineup: tc.lineup, Acquisitions: tc.acquire}),
				Existing: existing, Number: 7, Now: bindNow,
			}
			got, err := approval.Bind(in)
			if tc.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantError) {
					t.Fatalf("Bind = %v, want an error naming the ceiling %s", err, tc.wantError)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Channel.Lineup) != tc.wantLen {
				t.Fatalf("lineup = %d titles, want %d", len(got.Channel.Lineup), tc.wantLen)
			}
			if len(got.Full) != tc.wantFull {
				t.Fatalf("Full = %d titles, want %d", len(got.Full), tc.wantFull)
			}
			// What is reported turned away is not in the lineup.
			held := map[string]bool{}
			for _, e := range got.Channel.Lineup {
				held[string(e.Key)] = true
			}
			for _, e := range got.Full {
				if held[string(e.Key)] {
					t.Fatalf("%s is reported turned away and is in the lineup", e.Key)
				}
			}
		})
	}
}
