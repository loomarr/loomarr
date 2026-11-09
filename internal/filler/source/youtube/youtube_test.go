package youtube_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/acquisition"
	"github.com/loomarr/loomarr/internal/filler/fetch"
	"github.com/loomarr/loomarr/internal/filler/source"
	"github.com/loomarr/loomarr/internal/filler/source/youtube"
	"github.com/loomarr/loomarr/internal/testkit"
)

func video(id string) source.Item {
	return source.Item{
		ID: id, URL: "https://www.youtube.com/watch?v=" + id,
		DurationMS: 30_000, DurationKnown: true,
	}
}

func titled(id, title string, duration time.Duration) source.Item {
	item := video(id)
	item.Title, item.DurationMS = title, int(duration.Milliseconds())
	return item
}

func ids(items []source.Item) []string {
	out := make([]string, len(items))
	for i, item := range items {
		out[i] = item.ID
	}
	return out
}

// listOf is a provider over a fixed listing that records how much it was asked to read.
func listOf(items []source.Item, asked *int) *youtube.Provider {
	return youtube.New(func(_ context.Context, _ string, limit int) ([]source.Item, int, error) {
		if asked != nil {
			*asked = limit
		}
		return items, len(items), nil
	})
}

func list(t *testing.T, p *youtube.Provider, cp source.Checkpoint, req source.ListRequest) source.Listing {
	t.Helper()
	listing, err := p.List(t.Context(), source.Source{ID: "s", URI: "target", Checkpoint: cp}, req)
	if err != nil {
		t.Fatal(err)
	}
	return listing
}

// Registration is stricter than ingest: a registered source is fetched unattended, so a typo'd or
// look-alike host must be refused rather than stored as a row that can never fetch.
func TestProvider_AddressAcceptsOnlyYouTubeHosts(t *testing.T) {
	p := youtube.New(nil)
	for _, tc := range []struct{ in, want string }{
		{"https://www.youtube.com/playlist?list=PL123", "https://www.youtube.com/playlist?list=PL123"},
		{"youtube.com/@vault/videos", "https://youtube.com/@vault/videos"},
		{"https://youtu.be/abc", "https://youtu.be/abc"},
		{"  https://m.youtube.com/channel/UC1  ", "https://m.youtube.com/channel/UC1"},

		{"", ""},
		{"https://example.com/playlist?list=PL123", ""},
		{"https://youtube.com.evil.test/playlist?list=PL123", ""},
		{"https://www.youtube.com/", ""},
	} {
		if got := p.Address(tc.in); got != tc.want {
			t.Errorf("Address(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// Without yt-dlp the provider still answers, naming the sentinel the API turns into advice.
func TestProvider_FinderFailsWithTheProviderSentinelWhenUnavailable(t *testing.T) {
	p := youtube.New(nil)
	if _, err := p.Suggest(t.Context(), "retro", 3); !errors.Is(err, acquisition.ErrSourceProvider) {
		t.Errorf("Suggest without a finder = %v, want acquisition.ErrSourceProvider", err)
	}
	if _, err := p.Resolve(t.Context(), "https://www.youtube.com/@vault"); !errors.Is(err, acquisition.ErrSourceProvider) {
		t.Errorf("Resolve without a finder = %v, want acquisition.ErrSourceProvider", err)
	}
}

func TestProvider_IsFetchedAndListsByAReadBoundedByTheLookback(t *testing.T) {
	p := youtube.New(nil)
	if p.ID() != "youtube" || !p.Describe().Fetched {
		t.Fatalf("provider = %q %+v, want the fetched youtube provider", p.ID(), p.Describe())
	}
	var _ source.Lister = p

	var asked int
	list(t, listOf(nil, &asked), source.Checkpoint{}, source.ListRequest{Take: 2, List: 2})
	if asked != youtube.InitialLookback {
		t.Fatalf("asked for %d items, want the %d-item lookback whatever the budget", asked, youtube.InitialLookback)
	}
}

func TestList_FailsWhenTheListingCannotBeRead(t *testing.T) {
	boom := errors.New("yt-dlp failed")
	failing := youtube.New(func(context.Context, string, int) ([]source.Item, int, error) { return nil, 0, boom })
	if _, err := failing.List(t.Context(), source.Source{URI: "x"}, source.ListRequest{Take: 1}); !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the listing's own", err)
	}
	if _, err := youtube.New(nil).List(t.Context(), source.Source{URI: "x"}, source.ListRequest{Take: 1}); err == nil {
		t.Fatal("a provider without an enumerator listed")
	}
}

// The sweep: the first check reads from the newest item and stops at Take, leaving a cursor past
// the last item it examined and the newest item as the pending watermark; the next resumes from
// the cursor and, finishing, commits the watermark; a later sweep stops at that watermark.
func TestList_ResumesPastTheCursorAndStopsAtTheWatermark(t *testing.T) {
	items := []source.Item{video("v1"), video("v2"), video("v3"), video("v4")}
	req := source.ListRequest{Take: 2, List: 2}

	first := list(t, listOf(items, nil), source.Checkpoint{}, req)
	if !reflect.DeepEqual(ids(first.Items), []string{"v1", "v2"}) ||
		first.Checkpoint != (source.Checkpoint{Cursor: "v2", PendingWatermark: "v1"}) || !first.ReplaceOutcomes {
		t.Fatalf("first = %v %+v replace=%v", ids(first.Items), first.Checkpoint, first.ReplaceOutcomes)
	}

	second := list(t, listOf(items, nil), first.Checkpoint, req)
	if !reflect.DeepEqual(ids(second.Items), []string{"v3", "v4"}) ||
		second.Checkpoint != (source.Checkpoint{Watermark: "v1"}) || second.ReplaceOutcomes {
		t.Fatalf("second = %v %+v replace=%v, want the rest, the watermark committed, and the first sweep's outcomes extended",
			ids(second.Items), second.Checkpoint, second.ReplaceOutcomes)
	}

	refreshed := append([]source.Item{video("new")}, items...)
	third := list(t, listOf(refreshed, nil), second.Checkpoint, req)
	if !reflect.DeepEqual(ids(third.Items), []string{"new"}) ||
		third.Checkpoint != (source.Checkpoint{Watermark: "new"}) || !third.ReplaceOutcomes {
		t.Fatalf("third = %v %+v replace=%v, want only what is newer than the watermark", ids(third.Items), third.Checkpoint, third.ReplaceOutcomes)
	}
}

// A cursor whose item has left the listing cannot be resumed from: the sweep restarts, and the
// outcomes it counts replace the stale sweep's.
func TestList_RestartsTheSweepWhenTheCursorItemIsGone(t *testing.T) {
	items := []source.Item{video("v1"), video("v2")}
	got := list(t, listOf(items, nil),
		source.Checkpoint{Cursor: "gone", PendingWatermark: "stale", Watermark: "old"},
		source.ListRequest{Take: 5})
	if !reflect.DeepEqual(ids(got.Items), []string{"v1", "v2"}) || !got.ReplaceOutcomes ||
		got.Checkpoint != (source.Checkpoint{Watermark: "v1"}) {
		t.Fatalf("got = %v %+v replace=%v, want a fresh sweep from the newest item", ids(got.Items), got.Checkpoint, got.ReplaceOutcomes)
	}
}

// A page of items the caller already has must not use up the check, and is counted as known.
func TestList_CountsKnownItemsAgainstNothing(t *testing.T) {
	items := []source.Item{video("v1"), video("v2"), video("v3")}
	got := list(t, listOf(items, nil), source.Checkpoint{}, source.ListRequest{
		Take: 1, Known: func(item source.Item) bool { return item.ID != "v3" },
	})
	if !reflect.DeepEqual(ids(got.Items), []string{"v3"}) || got.Outcomes[source.OutcomeAlreadyKnown] != 2 {
		t.Fatalf("got = %v %v, want the one unknown item after two known ones", ids(got.Items), got.Outcomes)
	}
}

// The flat listing's own metadata rules an item out before anything is downloaded, each for a
// reason the operator can read.
func TestList_ScreensItemsByTheirMetadata(t *testing.T) {
	items := []source.Item{
		video("eligible"),
		titled("short", "", 9*time.Second),
		titled("long", "", 121*time.Second),
		{ID: "unknown", URL: "u"},
		{ID: "live", URL: "u", DurationKnown: true, DurationMS: 30_000, LiveStatus: "is_live"},
		{ID: "past-live", URL: "u", DurationKnown: true, DurationMS: 30_000, LiveStatus: "post_live"},
		{ID: "upcoming", URL: "u", DurationKnown: true, DurationMS: 30_000, LiveStatus: "is_upcoming"},
		{ID: "private", URL: "u", DurationKnown: true, DurationMS: 30_000, Availability: "private"},
		{ID: "gated", URL: "u", DurationKnown: true, DurationMS: 30_000, Availability: "needs_auth"},
		{ID: "no-url", DurationKnown: true, DurationMS: 30_000},
		{ID: "", URL: "u", DurationKnown: true, DurationMS: 30_000},
	}
	got := list(t, listOf(items, nil), source.Checkpoint{}, source.ListRequest{
		Take: 20, MinDuration: 10 * time.Second, MaxDuration: 2 * time.Minute,
	})
	want := source.Outcomes{
		source.OutcomeTooShort: 1, source.OutcomeTooLong: 1,
		source.OutcomeMetadataIncomplete: 3, source.OutcomeLive: 2,
		source.OutcomeUpcoming: 1, source.OutcomePrivate: 1, source.OutcomeUnavailable: 1,
	}
	if !reflect.DeepEqual(ids(got.Items), []string{"eligible"}) || !reflect.DeepEqual(got.Outcomes, want) {
		t.Fatalf("got = %v %v, want only the eligible item and %v", ids(got.Items), got.Outcomes, want)
	}
}

// The caller's rejection comes first: its outcome is the one counted, and the item is not offered.
func TestList_AsksTheCallersRejectionFirst(t *testing.T) {
	reel := titled("reel", "", 30*time.Minute)
	got := list(t, listOf([]source.Item{reel, video("spot")}, nil), source.Checkpoint{}, source.ListRequest{
		Take: 5, Reject: func(item source.Item) (source.Outcome, bool) {
			return source.OutcomeTooLong, item.ID == "reel"
		},
	})
	if !reflect.DeepEqual(ids(got.Items), []string{"spot"}) || got.Outcomes[source.OutcomeTooLong] != 1 {
		t.Fatalf("got = %v %v", ids(got.Items), got.Outcomes)
	}
}

// One campaign published as several duration cuts is offered as its fullest cut, and the choice is
// made over the whole listing, so a cursor splitting it across checks cannot offer two.
func TestList_OffersOneCutPerCampaign(t *testing.T) {
	short := titled("qb-15", "HH X GFL QB Rory :15 sec. spot", 15*time.Second)
	long := titled("qb-30", "HH X GFL QB Rory :30 sec. spot", 30*time.Second)
	other := titled("hot-takes", "Hot Takes - 30 Sec", 30*time.Second)
	items := []source.Item{short, other, long}

	got := list(t, listOf(items, nil), source.Checkpoint{}, source.ListRequest{Take: 5})
	if !reflect.DeepEqual(ids(got.Items), []string{"hot-takes", "qb-30"}) ||
		got.Outcomes[source.OutcomeAlreadyKnown] != 1 {
		t.Fatalf("got = %v %v, want the fuller QB cut and the other campaign, the :15 cut counted known", ids(got.Items), got.Outcomes)
	}

	// Split across two checks: the :15 cut sits in the first window, the :30 in the second.
	first := list(t, listOf(items, nil), source.Checkpoint{}, source.ListRequest{Take: 1})
	second := list(t, listOf(items, nil), first.Checkpoint, source.ListRequest{Take: 5})
	if all := append(ids(first.Items), ids(second.Items)...); !reflect.DeepEqual(all, []string{"hot-takes", "qb-30"}) {
		t.Fatalf("checks offered %v, want the QB campaign once, as its fuller cut", all)
	}
}

func TestList_TiesBetweenCutsBreakOnHeightThenID(t *testing.T) {
	a := titled("a", "Acme :30 sec spot", 30*time.Second)
	b := titled("b", "Acme :30 sec spot", 30*time.Second)
	b.Height = 1080
	c := titled("c", "Acme :30 sec spot", 30*time.Second)
	c.Height = 1080
	got := list(t, listOf([]source.Item{a, c, b}, nil), source.Checkpoint{}, source.ListRequest{Take: 5})
	if !reflect.DeepEqual(ids(got.Items), []string{"b"}) {
		t.Fatalf("got %v, want the tallest cut, the lowest id among equals", ids(got.Items))
	}
}

// The listing of a source that reads through yt-dlp carries every field the provider screens on.
func TestYtDlp_ReadsTheFlatListing(t *testing.T) {
	ytdlp := testkit.Executable(t, "yt-dlp", `#!/bin/sh
case "$*" in
  *--no-config*--flat-playlist*--skip-download*--dump-single-json*--playlist-end\ 4*https://www.youtube.com/@retroads/videos) ;;
  *) exit 9 ;;
esac
printf '%s\n' '{"entries":[{"id":"one","webpage_url":"https://www.youtube.com/watch?v=one","duration":31.5,"availability":"public","live_status":"not_live"}]}'
`)
	items, _, err := youtube.YtDlp(fetch.NewYouTubeEnumerator(ytdlp))(t.Context(), "https://www.youtube.com/@retroads/videos", 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "one" || items[0].URL != "https://www.youtube.com/watch?v=one" ||
		!items[0].DurationKnown || items[0].DurationMS != 31_500 ||
		items[0].Availability != "public" || items[0].LiveStatus != "not_live" {
		t.Fatalf("items = %+v, want bounded YouTube result", items)
	}
}

// A finder given an address checks it; given words it searches. The provider draws the line.
func TestProvider_RecognizesAddressesNotWords(t *testing.T) {
	p := youtube.New(nil)
	for in, want := range map[string]bool{
		"https://www.youtube.com/@vault/videos": true,
		"youtu.be/abc":                          true,
		"@vault":                                true,
		"UCabcdefghijklmnop":                    true,
		"PLabcdefghij":                          true,
		"retro ads":                             false,
		"vault":                                 false,
		"https://example.com/watch":             false,
	} {
		if got := p.Recognizes(in); got != want {
			t.Errorf("Recognizes(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestProvider_LinksToItsOwnPages(t *testing.T) {
	p := youtube.New(nil)
	if got := p.SourceURL(" https://www.youtube.com/@vault "); got != "https://www.youtube.com/@vault" {
		t.Errorf("SourceURL = %q", got)
	}
	if got := p.EmbedURL("https://www.youtube.com/watch?v=abc_123"); got != "https://www.youtube-nocookie.com/embed/abc_123?autoplay=1&playsinline=1" {
		t.Errorf("EmbedURL = %q", got)
	}
	for _, refused := range []string{"https://youtube.com.evil.test/watch?v=abc", "https://www.youtube.com/watch?v=a/../b", "https://www.youtube.com/watch", "::"} {
		if got := p.EmbedURL(refused); got != "" {
			t.Errorf("EmbedURL(%q) = %q, want none", refused, got)
		}
	}
}
