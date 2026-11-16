package growth_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels/edit"
	"github.com/loomarr/loomarr/internal/channels/growth"
	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/library/provision"
	"github.com/loomarr/loomarr/internal/platform/llm"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
	"github.com/loomarr/loomarr/internal/testkit"
)

// judge is a model that answers from a fixed table keyed by the title named in the question, and
// counts what it was asked.
type judge struct {
	answers map[string]string // title name → reply
	asked   []string
}

func (j *judge) Name() string  { return "judge" }
func (j *judge) Model() string { return "judge-1" }

func (j *judge) Chat(_ context.Context, msgs []llm.Message, _ llm.ChatOptions) (llm.Response, error) {
	q := msgs[len(msgs)-1].Content
	for name, reply := range j.answers {
		if strings.Contains(q, fmt.Sprintf("Does %q", name)) {
			j.asked = append(j.asked, name)
			return llm.Response{Content: reply}, nil
		}
	}
	return llm.Response{}, fmt.Errorf("asked about a title the test did not expect: %s", q)
}

// allow is a spending cap that holds once its room is spent, and counts what was settled.
type allow struct {
	room    int
	settled int
}

func (a *allow) ReserveReview(context.Context, string, provision.Key, time.Time) (string, bool, error) {
	if a.room <= 0 {
		return "", false, nil
	}
	a.room--
	return fmt.Sprintf("spend-%d", a.room), true, nil
}

func (a *allow) SettleReview(context.Context, string, llm.Attribution, bool, time.Time) error {
	a.settled++
	return nil
}

func review(st store.Store, j *judge, spend growth.Spend, busy func() bool) *growth.Review {
	return growth.NewReview(st, j, &edit.Service{Store: st, Log: testkit.Logger()}, spend, busy,
		func() time.Time { return fillNow }, testkit.Logger())
}

const (
	fits    = `{"answer": "fits", "reason": "a family sitcom of the era"}`
	notFits = `{"answer": "does_not_fit", "reason": "a crime drama, not a sitcom"}`
)

// A title growth added that the model judges does not fit is taken off, with its reason kept, and
// the next night does not add it back; putting it back by hand makes it a seed the review never
// asks about.
func TestARejectedGrownTitleIsTakenOffAndStaysOff(t *testing.T) {
	st, fill := fillHarness(t, schedule.ChannelPolicy{})
	ctx := context.Background()
	if _, err := fill.Run(ctx); err != nil {
		t.Fatal(err)
	}
	j := &judge{answers: map[string]string{"Show 90000703": notFits}}
	spend := &allow{room: 10}
	if n, err := review(st, j, spend, nil).Run(ctx); err != nil || n != 1 {
		t.Fatalf("judged %d (err %v), want the one grown title", n, err)
	}
	if spend.settled != 1 {
		t.Errorf("settled %d questions, want the one asked", spend.settled)
	}
	ch := lineupOf(t, st)
	if len(ch.Lineup) != 2 || !ch.Policy.Declined("series:tvdb:90000703") {
		t.Fatalf("lineup %d titles, declined %v; want the rejected title off and declined", len(ch.Lineup), ch.Policy.Growth)
	}
	reviews, err := st.GrowthReviews(ctx, "ch-grow")
	if err != nil {
		t.Fatal(err)
	}
	if r := reviews["series:tvdb:90000703"]; r.Verdict != "does_not_fit" || r.Reason != "a crime drama, not a sitcom" || r.Model != "judge-1" {
		t.Errorf("stored verdict %+v", r)
	}
	if grown, err := fill.Run(ctx); err != nil || grown != 0 {
		t.Fatalf("the next night grew %d (err %v), want none", grown, err)
	}

	// The undo: a person puts it back by hand. It is a seed now, and the review leaves it alone.
	if _, err := (&edit.Service{Store: st, Log: testkit.Logger()}).Edit(ctx, edit.Backend{}, "ch-grow", 0,
		func(ch *channelstore.Channel) error {
			ch.Lineup = append(ch.Lineup, seed(90000703, 1990))
			return nil
		}); err != nil {
		t.Fatal(err)
	}
	j.asked = nil
	if n, err := review(st, j, &allow{room: 10}, nil).Run(ctx); err != nil || n != 0 || len(j.asked) != 0 {
		t.Fatalf("judged %d, asked %v (err %v); a title a person put back is never asked about", n, j.asked, err)
	}
	if len(lineupOf(t, st).Lineup) != 3 {
		t.Error("the title a person put back was taken off again")
	}
}

// Where a model is configured an unclear fit waits for it; once the model judges it fits, it joins
// at the next fill.
func TestAnUnclearFitJoinsOnceTheModelAcceptsIt(t *testing.T) {
	st, fill := fillHarnessWith(t, schedule.ChannelPolicy{}, true)
	libraryShow(t, st, 90000707, 0, "TV-PG", 20, "Comedy", "Family") // no year: an unclear fit
	ctx := context.Background()
	if _, err := fill.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if n := len(lineupOf(t, st).Lineup); n != 3 {
		t.Fatalf("first night: %d titles, want the seeds and the clear fit while the unclear one waits", n)
	}
	j := &judge{answers: map[string]string{"Show 90000703": fits, "Show 90000707": fits}}
	if n, err := review(st, j, &allow{room: 10}, nil).Run(ctx); err != nil || n != 2 {
		t.Fatalf("judged %d (err %v), want the grown title and the unclear one", n, err)
	}
	if j.asked[0] != "Show 90000703" {
		t.Errorf("asked %v, want the title already airing first", j.asked)
	}
	if _, err := fill.Run(ctx); err != nil {
		t.Fatal(err)
	}
	ch := lineupOf(t, st)
	if len(ch.Lineup) != 4 || ch.Lineup[3].Key != "series:tvdb:90000707" {
		t.Fatalf("lineup %+v, want the accepted unclear fit to join", ch.Lineup)
	}
}

// The review waits while a person is waiting on the model, and stops at the day's spending cap.
func TestTheReviewYieldsToAPersonAndStopsAtTheCap(t *testing.T) {
	st, fill := fillHarness(t, schedule.ChannelPolicy{})
	ctx := context.Background()
	if _, err := fill.Run(ctx); err != nil {
		t.Fatal(err)
	}
	j := &judge{answers: map[string]string{"Show 90000703": notFits}}
	if n, _ := review(st, j, &allow{room: 10}, func() bool { return true }).Run(ctx); n != 0 || len(j.asked) != 0 {
		t.Errorf("asked %v while a person was waiting", j.asked)
	}
	if n, _ := review(st, j, &allow{room: 0}, nil).Run(ctx); n != 0 || len(j.asked) != 0 {
		t.Errorf("asked %v past the spending cap", j.asked)
	}
	if len(lineupOf(t, st).Lineup) != 3 {
		t.Error("a title was taken off without a verdict")
	}
}
