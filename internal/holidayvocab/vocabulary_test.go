package holidayvocab_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/holidayvocab"
)

func TestDefinitionsPinBuiltInIdentityAndAliasCoverage(t *testing.T) {
	want := []holidayvocab.Definition{
		{ID: "halloween", Label: "Halloween"},
		{ID: "thanksgiving", Label: "Thanksgiving"},
		{ID: "christmas", Label: "Christmas"},
		{ID: "newyear", Label: "New Year"},
		{ID: "valentines", Label: "Valentine's Day"},
	}
	if got := holidayvocab.Definitions(); !reflect.DeepEqual(got, want) {
		t.Fatalf("built-in holiday vocabulary = %+v, want %+v", got, want)
	}
	for _, definition := range want {
		named := holidayvocab.Judge(definition.ID, holidayvocab.Evidence{Title: definition.Label + " special"})
		if named.Verdict != holidayvocab.VerdictAbout {
			t.Fatalf("a title naming %q judged %+v, want about", definition.Label, named)
		}
	}
}

// The floor (§6) on invented titles that mirror a real Halloween idea's picks: a word in the
// title is not enough; the holiday's name is, and a mood word is when the genres back it up.
func TestJudgeHalloweenFloor(t *testing.T) {
	for _, tt := range []struct {
		name   string
		ev     holidayvocab.Evidence
		want   holidayvocab.Verdict
		reason holidayvocab.Reason
	}{
		{"teen comedy with ghost in the title",
			holidayvocab.Evidence{Title: "Ghost Mall", Genres: []string{"Comedy", "Drama"}}, holidayvocab.VerdictNot, ""},
		{"spy action film",
			holidayvocab.Evidence{Title: "Agent Zero: Ghost Directive", Genres: []string{"Action", "Thriller", "Adventure"}}, holidayvocab.VerdictNot, ""},
		{"supernatural romance has fantasy but no family genre",
			holidayvocab.Evidence{Title: "Phantom Heart Ghost", Genres: []string{"Fantasy", "Romance", "Thriller"}}, holidayvocab.VerdictNot, ""},
		{"true-crime docuseries",
			holidayvocab.Evidence{Title: "The Orchard Lane Murders: A Ghost Story", Genres: []string{"Documentary", "Crime"}}, holidayvocab.VerdictNot, ""},
		{"overview mood word alone",
			holidayvocab.Evidence{Title: "Night Shift", Text: []string{"A nurse thinks she sees a ghost."}}, holidayvocab.VerdictNot, ""},
		{"slasher franchise named for the night",
			holidayvocab.Evidence{Title: "Halloween Returns", Genres: []string{"Horror"}}, holidayvocab.VerdictAbout, holidayvocab.ReasonNameInTitle},
		{"holiday keyword",
			holidayvocab.Evidence{Title: "Pumpkin Patch", Keywords: []string{"halloween", "small town"}}, holidayvocab.VerdictAbout, holidayvocab.ReasonHolidayTag},
		{"annual special overview",
			holidayvocab.Evidence{Title: "Treetop of Terror IV", Text: []string{"In the annual Halloween special, three tales."}}, holidayvocab.VerdictAbout, holidayvocab.ReasonNameInText},
		{"folk horror",
			holidayvocab.Evidence{Title: "The Hollow Witch", Genres: []string{"Horror", "Mystery"}}, holidayvocab.VerdictAdjacent, holidayvocab.ReasonMoodWithGenre},
		{"anthology horror series",
			holidayvocab.Evidence{Title: "National Horror Tales", Genres: []string{"Drama", "Horror", "Thriller"}}, holidayvocab.VerdictAdjacent, holidayvocab.ReasonMoodWithGenre},
		{"family witch sitcom on a TV genre list",
			holidayvocab.Evidence{Title: "Wendy the Teenage Witch", Genres: []string{"Comedy", "Family", "Sci-Fi & Fantasy"}}, holidayvocab.VerdictAdjacent, holidayvocab.ReasonMoodWithGenre},
		{"horror genre without a mood word",
			holidayvocab.Evidence{Title: "The Quiet Thing", Genres: []string{"Horror"}}, holidayvocab.VerdictNot, ""},
		{"whole words only",
			holidayvocab.Evidence{Title: "The Ghostwriter", Genres: []string{"Horror"}}, holidayvocab.VerdictNot, ""},
	} {
		got := holidayvocab.Judge("halloween", tt.ev)
		if got.Verdict != tt.want || got.Reason != tt.reason {
			t.Errorf("%s: Judge(%+v) = %+v, want %s (%q)", tt.name, tt.ev, got, tt.want, tt.reason)
		}
		if got.Fits() != (tt.want != holidayvocab.VerdictNot) {
			t.Errorf("%s: Fits() = %v for verdict %s", tt.name, got.Fits(), got.Verdict)
		}
	}
	if got := holidayvocab.Judge("arbor-day", holidayvocab.Evidence{Title: "Arbor Day"}); got.Verdict != holidayvocab.VerdictNot {
		t.Errorf("unknown holiday judged %+v, want not", got)
	}
}

// Invented episodes mirroring a long-running animated sitcom's annual Halloween anthology: the
// overviews never say "Halloween" and there are no tags or episode genres. The original air date
// is what proves them (§6); without it the episode title still fits the mood; a date alone or a
// mood word in an ordinary overview proves nothing.
func TestJudgeHalloweenEpisodes(t *testing.T) {
	day := func(year int, month time.Month, d int) time.Time { return time.Date(year, month, d, 0, 0, 0, 0, time.UTC) }
	episode := func(title, overview string, aired time.Time) holidayvocab.Evidence {
		return holidayvocab.Evidence{Title: title, Text: []string{overview}, Episode: true, AirDate: aired}
	}
	for _, tt := range []struct {
		name   string
		ev     holidayvocab.Evidence
		want   holidayvocab.Verdict
		reason holidayvocab.Reason
	}{
		{"anthology premiered late October",
			episode("Treetop of Terror", "Three tales: a cursed doll, a talking cat and a flying saucer.", day(1990, time.October, 25)),
			holidayvocab.VerdictAbout, holidayvocab.ReasonMoodAiredNear},
		{"anthology premiered early November",
			episode("Treetop of Terror XII", "A leprechaun, a smart house and a wizard school.", day(2001, time.November, 2)),
			holidayvocab.VerdictAbout, holidayvocab.ReasonMoodAiredNear},
		{"mood word in the overview of an episode aired in the window",
			episode("Grandpa's Attic", "Lisa is sure a ghost lives upstairs.", day(1993, time.October, 30)),
			holidayvocab.VerdictAbout, holidayvocab.ReasonMoodAiredNear},
		{"anthology with no air date",
			episode("Treetop of Terror III", "Three more tales.", time.Time{}),
			holidayvocab.VerdictAdjacent, holidayvocab.ReasonMoodInEpisode},
		{"ordinary episode aired October 28",
			episode("The New Neighbor", "Dad buys a boat.", day(1994, time.October, 28)),
			holidayvocab.VerdictNot, ""},
		{"ghost in an ordinary overview, no date",
			episode("Grandpa's Attic", "Lisa is sure a ghost lives upstairs.", time.Time{}),
			holidayvocab.VerdictNot, ""},
		{"ghost in an ordinary overview, aired in spring",
			episode("Grandpa's Attic", "Lisa is sure a ghost lives upstairs.", day(1993, time.April, 8)),
			holidayvocab.VerdictNot, ""},
		{"mood word just outside the window",
			episode("Grandpa's Attic", "Lisa is sure a ghost lives upstairs.", day(1993, time.November, 8)),
			holidayvocab.VerdictNot, ""},
	} {
		got := holidayvocab.Judge("halloween", tt.ev)
		if got.Verdict != tt.want || got.Reason != tt.reason {
			t.Errorf("%s: Judge(%+v) = %+v, want %s (%q)", tt.name, tt.ev, got, tt.want, tt.reason)
		}
	}
	// A film title is not an episode title: the same words with no genre stay out.
	if got := holidayvocab.Judge("halloween", holidayvocab.Evidence{Title: "Treetop of Terror"}); got.Verdict != holidayvocab.VerdictNot {
		t.Errorf("film titled with a mood word and no genre judged %+v, want not", got)
	}
}

// Each holiday's broadcast window, including the ones that wrap into January.
func TestJudgeEpisodeAirWindows(t *testing.T) {
	day := func(year int, month time.Month, d int) time.Time { return time.Date(year, month, d, 0, 0, 0, 0, time.UTC) }
	for _, tt := range []struct {
		id, text string
		aired    time.Time
		want     holidayvocab.Verdict
	}{
		{"halloween", "a spooky night", day(2010, time.October, 15), holidayvocab.VerdictAbout},
		{"halloween", "a spooky night", day(2010, time.October, 14), holidayvocab.VerdictNot},
		{"halloween", "a spooky night", day(2010, time.November, 7), holidayvocab.VerdictAbout},
	} {
		got := holidayvocab.Judge(tt.id, holidayvocab.Evidence{Title: "Episode", Text: []string{tt.text}, Episode: true, AirDate: tt.aired})
		if got.Verdict != tt.want {
			t.Errorf("Judge(%s, %q aired %s) = %+v, want %s", tt.id, tt.text, tt.aired.Format(time.DateOnly), got, tt.want)
		}
	}
}

// Names that are also everyday words in a title stay out of their holiday.
func TestJudgeIgnoresNameLookAlikes(t *testing.T) {
	for _, tt := range []struct {
		id, title string
		want      holidayvocab.Verdict
	}{
		{"christmas", "Santa Clarita Nights", holidayvocab.VerdictNot},
		{"christmas", "Santa Barbara Blue", holidayvocab.VerdictNot},
		{"christmas", "Santa's Sleigh Ride", holidayvocab.VerdictAbout},
		{"newyear", "Bill Nye Explains Rockets", holidayvocab.VerdictNot},
		{"newyear", "The NYE Party", holidayvocab.VerdictAbout},
	} {
		if got := holidayvocab.Judge(tt.id, holidayvocab.Evidence{Title: tt.title}); got.Verdict != tt.want {
			t.Errorf("Judge(%s, %q) = %+v, want %s", tt.id, tt.title, got, tt.want)
		}
	}
}

func TestIntentAliasesCoverSchedulerKnownSantaAndNYE(t *testing.T) {
	for _, tt := range []struct {
		text string
		want string
	}{
		{text: "Santa specials", want: "christmas"},
		{text: "An NYE marathon", want: "newyear"},
	} {
		got := holidayvocab.MatchIntent(tt.text)
		if len(got) != 1 || got[0] != tt.want {
			t.Fatalf("MatchIntent(%q) = %v, want [%s]", tt.text, got, tt.want)
		}
	}
}

func TestChristmasEvidenceIgnoresOnlyPetNameOccurrences(t *testing.T) {
	for _, tt := range []struct {
		text string
		want bool
	}{
		{"Santa’s Little Helper gets lost", false},
		{"Santa's Little Helper and Santa's Little Helper", false},
		{"Santa visits Santa’s Little Helper", true},
		{"Santa's Little Helper meets Santa", true},
		{"Santa’s Little Helper celebrates Christmas", true},
		{"A Halloween episode with holiday decorations", false},
		{"The family meets Santa", true},
		{"Ｓａｎｔａ’s Little Helper runs away", false},
	} {
		got := holidayvocab.Judge("christmas", holidayvocab.Evidence{Text: []string{tt.text}})
		if got.Fits() != tt.want {
			t.Errorf("Judge(christmas, %q) = %+v, want fits=%v", tt.text, got, tt.want)
		}
	}
}
