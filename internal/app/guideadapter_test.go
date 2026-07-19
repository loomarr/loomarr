package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/programmer"
)

// Now/next selection is a containment test, not "take the first entry" — a guide that
// arrives out of order, or whose first entry has already finished, must still report what
// is actually airing. Serving a hand-built window here (rather than the pinned capture,
// which is fixed in wall-clock time) lets the boundaries be exercised precisely; the
// pinned capture guards the PARSING, in programmer/guide_contract_test.go.
func TestNowNext_PicksAiringAndUpcoming(t *testing.T) {
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	ms := func(offset time.Duration) string { return strconv.FormatInt(now.Add(offset).UnixMilli(), 10) }

	body := `{"ch-1":{"id":"ch-1","name":"Test","number":42,"programs":[
	  {"type":"content","start":` + ms((-2 * time.Hour)) + `,"stop":` + ms((-1 * time.Hour)) + `,"program":{"title":"Already Over"}},
	  {"type":"content","start":` + ms((30 * time.Minute)) + `,"stop":` + ms((90 * time.Minute)) + `,"program":{"title":"Later"}},
	  {"type":"content","start":` + ms((-10 * time.Minute)) + `,"stop":` + ms((20 * time.Minute)) + `,"program":{"title":"On Now"}},
	  {"type":"flex","start":` + ms((20 * time.Minute)) + `,"stop":` + ms((30 * time.Minute)) + `,"title":"Commercials"}
	]}}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	a := guideAdapter{tunarr: programmer.New(srv.URL, "", "cfg"), window: 3 * time.Hour}
	got, err := a.NowNext(context.Background(), now)
	if err != nil {
		t.Fatalf("NowNext: %v", err)
	}

	nn, ok := got["ch-1"]
	if !ok {
		t.Fatal("channel missing from now/next")
	}
	// The airing program is the one CONTAINING now — not the first in the array, which
	// finished an hour ago.
	if nn.Now == nil || nn.Now.Title != "On Now" {
		t.Errorf("Now = %+v, want the containing entry (On Now)", nn.Now)
	}
	// Next is the EARLIEST future entry: the commercial gap at +20m, not the program at +30m.
	if nn.Next == nil || nn.Next.Title != "Commercials" {
		t.Errorf("Next = %+v, want the earliest upcoming entry (Commercials)", nn.Next)
	}
	// A flex entry is a gap, so the UI shows dead air rather than claiming a program.
	if nn.Next != nil && !nn.Next.Gap {
		t.Error("a flex entry must be flagged Gap")
	}
}

// A channel with nothing scheduled in the window is absent, not an empty-titled card.
func TestNowNext_OmitsChannelsWithNothingScheduled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"ch-1":{"id":"ch-1","name":"Empty","number":1,"programs":[]}}`))
	}))
	defer srv.Close()

	a := guideAdapter{tunarr: programmer.New(srv.URL, "", "cfg"), window: time.Hour}
	got, err := a.NowNext(context.Background(), time.Now())
	if err != nil {
		t.Fatalf("NowNext: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %v, want no entry for a channel with nothing scheduled", got)
	}
}
