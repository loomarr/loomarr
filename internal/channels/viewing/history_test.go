package viewing

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/channelstore"
	"github.com/loomarr/loomarr/internal/platform/store/identitystore"
	"github.com/loomarr/loomarr/internal/testkit"
)

func historyStore(t *testing.T) store.Store {
	t.Helper()
	ctx := context.Background()
	st := testkit.MigratedSQLiteStore(t)
	for _, id := range []string{"u-ada", "u-bo"} {
		if err := st.UpsertUser(ctx, identitystore.User{ID: id, Name: id, Role: identitystore.RoleMember,
			CreatedAt: t0, UpdatedAt: t0}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := st.SaveChannel(ctx, channelstore.Channel{Channel: schedule.Channel{ID: "ch-cartoons",
		Name: "Cartoons", Number: 1, Strategy: schedule.Sequential, Status: "live"}}); err != nil {
		t.Fatal(err)
	}
	return st
}

// history renders the rows oldest first as "user device channel-name programme start-end", times
// as offsets from t0.
func history(t *testing.T, st store.Store) string {
	t.Helper()
	rows, err := st.ListViewings(context.Background(), store.ViewingQuery{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for i := len(rows) - 1; i >= 0; i-- {
		v := rows[i]
		out = append(out, fmt.Sprintf("%s %q %s %q %s-%s", v.UserID, v.Device, v.ChannelName, v.Programme,
			v.StartedAt.Sub(t0), v.EndedAt.Sub(t0)))
	}
	return strings.Join(out, "\n")
}

type guide struct{ titles map[string]string }

func (g *guide) programmes(context.Context, time.Time) (map[string]string, error) {
	return g.titles, nil
}

func TestRecorderWritesOneRowPerProgrammeAndExtendsIt(t *testing.T) {
	st := historyStore(t)
	tr := New()
	g := &guide{titles: map[string]string{"ch-cartoons": "Bluey"}}
	rec := NewRecorder(tr, st, g.programmes, slog.New(slog.DiscardHandler))
	ctx := context.Background()
	sync := func(at time.Duration) {
		t.Helper()
		if err := rec.Sync(ctx, t0.Add(at)); err != nil {
			t.Fatal(err)
		}
	}

	poll(tr, adaTV, "ch-cartoons", 0)
	poll(tr, adaTV, "ch-cartoons", 5*time.Second)
	sync(10 * time.Second)
	poll(tr, adaTV, "ch-cartoons", 20*time.Second)
	sync(25 * time.Second)
	if got, want := history(t, st), `u-ada "Living room TV" Cartoons "Bluey" 0s-20s`; got != want {
		t.Fatalf("after one programme:\n%s\nwant\n%s", got, want)
	}

	// A gap between programmes keeps the row going; the next programme starts a new row.
	g.titles = map[string]string{"ch-cartoons": ""}
	poll(tr, adaTV, "ch-cartoons", 30*time.Second)
	sync(35 * time.Second)
	g.titles = map[string]string{"ch-cartoons": "Peppa Pig"}
	poll(tr, adaTV, "ch-cartoons", 40*time.Second)
	sync(45 * time.Second)
	poll(tr, adaTV, "ch-cartoons", 50*time.Second)
	sync(55 * time.Second)

	// The player stops: after the TTL the tracker forgets it, and the row keeps its last poll.
	sync(50*time.Second + TTL + time.Second)
	want := `u-ada "Living room TV" Cartoons "Bluey" 0s-40s` + "\n" +
		`u-ada "Living room TV" Cartoons "Peppa Pig" 40s-50s`
	if got := history(t, st); got != want {
		t.Fatalf("after the player stopped:\n%s\nwant\n%s", got, want)
	}
	if len(rec.open) != 0 {
		t.Errorf("%d viewings still open after the player stopped", len(rec.open))
	}
}

func TestRecorderAdoptsAProgrammeTheGuideLearnsLater(t *testing.T) {
	st := historyStore(t)
	tr := New()
	g := &guide{}
	rec := NewRecorder(tr, st, g.programmes, nil)
	ctx := context.Background()

	poll(tr, boTV, "ch-unknown", 0)
	poll(tr, boTV, "ch-unknown", 5*time.Second)
	if err := rec.Sync(ctx, t0.Add(6*time.Second)); err != nil {
		t.Fatal(err)
	}
	g.titles = map[string]string{"ch-unknown": "News"}
	poll(tr, boTV, "ch-unknown", 10*time.Second)
	if err := rec.Sync(ctx, t0.Add(11*time.Second)); err != nil {
		t.Fatal(err)
	}
	// The channel isn't in the store, so the row has no channel name.
	if got, want := history(t, st), `u-bo "Den TV"  "News" 0s-10s`; got != want {
		t.Fatalf("history:\n%s\nwant\n%s", got, want)
	}
}

func TestRecorderStartsAgainAfterAnAdminClears(t *testing.T) {
	st := historyStore(t)
	tr := New()
	rec := NewRecorder(tr, st, nil, nil)
	ctx := context.Background()

	poll(tr, adaWeb, "ch-cartoons", 0)
	poll(tr, adaWeb, "ch-cartoons", 5*time.Second)
	if err := rec.Sync(ctx, t0.Add(6*time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DeleteViewings(ctx, ""); err != nil {
		t.Fatal(err)
	}
	poll(tr, adaWeb, "ch-cartoons", 10*time.Second)
	if err := rec.Sync(ctx, t0.Add(11*time.Second)); err != nil {
		t.Fatal(err)
	}
	poll(tr, adaWeb, "ch-cartoons", 20*time.Second)
	if err := rec.Sync(ctx, t0.Add(21*time.Second)); err != nil {
		t.Fatal(err)
	}
	if got, want := history(t, st), `u-ada "Firefox on Linux" Cartoons "" 10s-20s`; got != want {
		t.Fatalf("history after a clear:\n%s\nwant\n%s", got, want)
	}
}

func TestRecorderWritesNothingWhileNobodyWatches(t *testing.T) {
	st := historyStore(t)
	tr := New()
	rec := NewRecorder(tr, st, func(context.Context, time.Time) (map[string]string, error) {
		t.Fatal("the guide was read with nobody watching")
		return nil, nil
	}, nil)
	// One fetch is the channel warmer, not a viewer.
	poll(tr, adaTV, "ch-cartoons", 0)
	if err := rec.Sync(context.Background(), t0.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if got := history(t, st); got != "" {
		t.Errorf("history = %q, want none", got)
	}
}
