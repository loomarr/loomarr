package store

import (
	"context"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
)

// A verdict is kept per channel and title: a second review replaces the first, another channel's
// verdict on the same title is its own, an unknown verdict is refused, and the verdicts go with
// their channel (#4333).
func testGrowthReviews(t *testing.T, newStore NewStoreFunc) {
	ctx := context.Background()
	st := newStore(t)
	for i, id := range []string{"ch-a", "ch-b"} {
		if _, err := st.SaveChannel(ctx, Channel{ID: id, Name: id, Number: 90 + i, Strategy: schedule.Shuffle,
			Status: schedule.StatusLive}); err != nil {
			t.Fatal(err)
		}
	}
	at := time.Unix(1_800_000_000, 0).UTC()
	save := func(r channelstore.GrowthReview) {
		t.Helper()
		if err := st.SaveGrowthReview(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	save(channelstore.GrowthReview{ChannelID: "ch-a", Key: "series:tvdb:90000701", Line: "old line",
		Verdict: channelstore.GrowthFits, Reason: "a family sitcom", Model: "m1", ReviewedAt: at})
	save(channelstore.GrowthReview{ChannelID: "ch-a", Key: "series:tvdb:90000701", Line: "new line",
		Verdict: channelstore.GrowthDoesNotFit, Reason: "a crime drama", Model: "m2", ReviewedAt: at.Add(time.Hour)})
	save(channelstore.GrowthReview{ChannelID: "ch-b", Key: "series:tvdb:90000701", Line: "b line",
		Verdict: channelstore.GrowthFits, ReviewedAt: at})
	if err := st.SaveGrowthReview(ctx, channelstore.GrowthReview{ChannelID: "ch-a", Key: "movie:tmdb:90000702",
		Verdict: "maybe"}); err == nil {
		t.Error("a verdict outside the fixed choice was saved")
	}

	a, err := st.GrowthReviews(ctx, "ch-a")
	if err != nil {
		t.Fatal(err)
	}
	got := a["series:tvdb:90000701"]
	if len(a) != 1 || got.Verdict != channelstore.GrowthDoesNotFit || got.Line != "new line" || got.Reason != "a crime drama" ||
		got.Model != "m2" || !got.ReviewedAt.Equal(at.Add(time.Hour)) {
		t.Fatalf("channel a verdicts %+v, want only the second review", a)
	}
	if b, _ := st.GrowthReviews(ctx, "ch-b"); b["series:tvdb:90000701"].Verdict != channelstore.GrowthFits {
		t.Errorf("channel b verdicts %+v, want its own", b)
	}

	chA, err := st.GetChannel(ctx, "ch-a")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteChannel(ctx, "ch-a", chA.Revision); err != nil {
		t.Fatal(err)
	}
	if gone, err := st.GrowthReviews(ctx, "ch-a"); err != nil || len(gone) != 0 {
		t.Errorf("a deleted channel kept %d verdicts (err %v)", len(gone), err)
	}
}
