package schedule

import (
	"testing"

	"github.com/loomarr/loomarr/internal/platform/rating"
)

func TestDeriveFiller_Audience(t *testing.T) {
	entry := func(r rating.Rating) LineupEntry { return LineupEntry{OfficialRating: r} }
	for _, tc := range []struct {
		name    string
		ceiling rating.Rating
		lineup  []LineupEntry
		want    string
	}{
		{"kids lineup, no ceiling, derives kids", "", []LineupEntry{entry("TV-Y"), entry("TV-Y7")}, "kids"},
		{"one adult title keeps a mixed lineup out of kids", "", []LineupEntry{entry("TV-Y"), entry("TV-MA")}, "general"},
		{"the ceiling wins over the lineup", "TV-Y7", []LineupEntry{entry("TV-14")}, "kids"},
		{"unrated entries neither help nor hurt", "", []LineupEntry{entry(""), entry("TV-Y")}, "kids"},
		{"no ratings anywhere derives nothing", "", []LineupEntry{entry("")}, ""},
		{"empty lineup derives nothing", "", nil, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := ChannelPolicy{ProposalPolicy: ProposalPolicy{Audience: AudiencePolicy{Ceiling: tc.ceiling}}}
			if got := DeriveFiller(p, tc.lineup).Audience; got != tc.want {
				t.Errorf("audience = %q, want %q", got, tc.want)
			}
		})
	}
}

// A channel's network is the one most of its series premiered on (#4432): a series counts its
// first network once, a movie counts nothing, and a tie or no networks at all derives none.
func TestDeriveFiller_NetworkIsTheLineupsMajority(t *testing.T) {
	series := func(networks ...string) LineupEntry { return LineupEntry{Networks: networks} }
	for _, tc := range []struct {
		name   string
		lineup []LineupEntry
		want   string
	}{
		{"one series", []LineupEntry{series("Kestrel")}, "KESTREL"},
		{"the most series win", []LineupEntry{series("Lantern"), series("Harbor"), series("Lantern"), series("Kestrel")}, "LANTERN"},
		{"only the premiere network counts", []LineupEntry{series("Harbor", "Lantern"), series("Lantern", "Harbor"), series("Harbor")}, "HARBOR"},
		{"names agree however they are written", []LineupEntry{series("The Juniper"), series("Juniper"), series("Kestrel")}, "JUNIPER"},
		{"a tie is no network", []LineupEntry{series("Harbor"), series("Lantern")}, ""},
		{"movies have none", []LineupEntry{{}, {}, series("Lantern")}, "LANTERN"},
		{"no networks at all", []LineupEntry{{}, {}}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := DeriveFiller(ChannelPolicy{}, tc.lineup).Network; got != tc.want {
				t.Errorf("network = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDeriveFiller_EraFromLineupYears(t *testing.T) {
	lineup := []LineupEntry{{Year: 1993}, {Year: 0}, {Year: 1987}, {Year: 1999}}
	got := DeriveFiller(ChannelPolicy{}, lineup).Era
	if got == nil || got.From != 1987 || got.To != 1999 {
		t.Fatalf("era = %+v, want 1987-1999 (unknown years ignored)", got)
	}
	if DeriveFiller(ChannelPolicy{}, []LineupEntry{{}}).Era != nil {
		t.Error("a lineup with no known years must derive no era")
	}
}
