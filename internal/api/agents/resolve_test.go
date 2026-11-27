package agents

import (
	"strings"
	"testing"
	"time"
)

func TestResolveChannel(t *testing.T) {
	list := []channel{
		{ID: "c1", Number: 1, Name: "Cartoons"},
		{ID: "c2", Number: 2, Name: "Cartoons Classic"},
		{ID: "c3", Number: 3, Name: "Channel 4"},
		{ID: "c4", Number: 4, Name: "Late Films"},
		{ID: "c5", Number: 5, Name: "24"},
	}
	for _, tc := range []struct {
		query  string
		want   string // channel ID, or "" for an error
		errHas string
	}{
		{query: "c4", want: "c4"},
		{query: "2", want: "c2"},
		{query: "#2", want: "c2"},
		{query: " cartoons ", want: "c1"},                  // an exact name beats the longer one containing it
		{query: "channel 4", want: "c3"},                   // a channel named that wins over number 4
		{query: "ch 4", want: "c4"},                        // a spelled-out number
		{query: "ch.1", want: "c1"},                        // the abbreviation with no space
		{query: "films", want: "c4"},                       // a unique part of a name
		{query: "classic", want: "c2"},                     // case-insensitive
		{query: "toon", errHas: "matches 2 channels"},      // ambiguous: named, never guessed
		{query: "9", errHas: "no channel is number 9"},     // an unknown number
		{query: "weather", errHas: "no channel is called"}, // an unknown name
		{query: "  ", errHas: "say which channel"},
		{query: "24", want: "c5"}, // no channel 24, but one is named that
	} {
		got, err := resolveChannel(list, tc.query)
		if tc.want != "" {
			if err != nil || got.ID != tc.want {
				t.Errorf("resolveChannel(%q) = %q, %v; want %q", tc.query, got.ID, err, tc.want)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), tc.errHas) {
			t.Errorf("resolveChannel(%q) error = %v; want one containing %q", tc.query, err, tc.errHas)
		}
	}
}

// An unknown channel's error lists the channels, so the model can correct itself without
// another call; a long lineup is cut, not dumped.
func TestResolveChannelErrorListsSomeChannels(t *testing.T) {
	var list []channel
	for i := 1; i <= 30; i++ {
		list = append(list, channel{ID: "c", Number: i, Name: "Ch"})
	}
	_, err := resolveChannel(list, "news")
	if err == nil || !strings.Contains(err.Error(), "and 18 more") {
		t.Fatalf("error = %v; want the first 12 channels and 'and 18 more'", err)
	}
}

// Times read in the household's zone, and a weekday alone never names a time from another week.
func TestClockFormatsRelativeToNow(t *testing.T) {
	loc := time.FixedZone("Household", -5*3600)
	now := time.Date(2026, time.November, 21, 18, 0, 0, 0, loc)
	k := clock{loc: loc, now: now}
	for _, tc := range []struct {
		at   time.Time
		want string
	}{
		{now.Add(90 * time.Minute), "19:30"},
		{now.Add(-26 * time.Hour), "Fri 16:00"},
		{now.Add(-21 * 24 * time.Hour), "Sat 31 Oct 18:00"},
	} {
		if got := k.at(tc.at.UnixMilli()); got != tc.want {
			t.Errorf("at(%s) = %q, want %q", tc.at, got, tc.want)
		}
	}
	if got := k.stamp("0001-01-01T00:00:00Z"); got != "" {
		t.Errorf("a never-run job's zero time rendered as %q, want empty", got)
	}
}

func TestSegmentRefusesDotSegments(t *testing.T) {
	for _, v := range []string{"", " ", ".", ".."} {
		if _, err := segment("job", v, "list_jobs"); err == nil {
			t.Errorf("segment(%q) was accepted", v)
		}
	}
	if got, err := segment("job", "a/b", "list_jobs"); err != nil || got != "a%2Fb" {
		t.Errorf("segment(a/b) = %q, %v; want it escaped", got, err)
	}
}
