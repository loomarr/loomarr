package main

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/binder"
	"github.com/loomarr/loomarr/internal/demolibrary"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/suggest"
)

// `make demo-seed` must be safe to rerun: the docs capture script runs it before every shoot.
// Running the store half twice must bind each demo channel once, keep its id, and leave every
// lineup title available through the approval gate.
func TestDemoSeedChannelsAreIdempotent(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(ctx, "sqlite://"+t.TempDir()+"/demo.db", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	run := func() map[int]string {
		admin, _, err := ensureAdmin(ctx, st)
		if err != nil {
			t.Fatal(err)
		}
		approver := suggest.NewApprover(st, binder.New(st, nil, nil, slog.New(slog.DiscardHandler)), time.Now)
		ids := map[int]string{}
		for _, c := range demolibrary.Channels {
			id, err := ensureChannel(ctx, st, approver, admin.ID, c)
			if err != nil {
				t.Fatalf("channel %d: %v", c.Number, err)
			}
			ids[c.Number] = id
		}
		return ids
	}
	first, second := run(), run()

	channels, err := st.ListChannels(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(channels) != len(demolibrary.Channels) {
		t.Fatalf("channels after two runs = %d, want %d", len(channels), len(demolibrary.Channels))
	}
	for n, id := range first {
		if second[n] != id {
			t.Errorf("channel %d moved from %s to %s on rerun", n, id, second[n])
		}
	}
	users, err := st.ListUsers(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 {
		t.Errorf("users after two runs = %d, want the one demo admin", len(users))
	}
	for _, c := range demolibrary.Channels {
		for _, id := range c.Titles {
			ti, _ := demolibrary.ByID(id)
			rec, err := st.GetTitle(ctx, provision.Key(ti.Key()))
			if err != nil {
				t.Fatalf("title %s: %v", ti.Key(), err)
			}
			if rec.State != provision.Available || rec.LibraryID != ti.ID() {
				t.Errorf("title %q = %s in library %q, want available as %s", ti.Name, rec.State, rec.LibraryID, ti.ID())
			}
		}
	}
}
