package schedule

import "testing"

func TestDeriveFiller_Audience(t *testing.T) {
	entry := func(r Rating) LineupEntry { return LineupEntry{OfficialRating: r} }
	for _, tc := range []struct {
		name    string
		ceiling Rating
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
