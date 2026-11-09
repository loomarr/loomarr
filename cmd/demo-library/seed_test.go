package main

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/channels/approval"
	"github.com/loomarr/loomarr/internal/demolibrary"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/proposals/accept"
	"github.com/loomarr/loomarr/internal/proposals/workflow"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/testkit"
)

// `make demo-seed` must be safe to rerun: the docs capture script runs it before every shoot.
// Running the store half twice must bind each demo channel once, keep its id, and leave every
// lineup title available through the approval gate. The open pack's channels go through the same
// gate, so both libraries are checked.
func TestDemoSeedChannelsAreIdempotent(t *testing.T) {
	for _, content := range []demolibrary.Content{demolibrary.ContentInvented, demolibrary.ContentOpen} {
		t.Run(string(content), func(t *testing.T) {
			lib, err := demolibrary.Load(content)
			if err != nil {
				t.Fatal(err)
			}
			seedTwice(t, lib)
		})
	}
}

func seedTwice(t *testing.T, lib *demolibrary.Library) {
	ctx := context.Background()
	st, err := store.Open(ctx, testkit.MigratedSQLiteDSN(t), true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	// What scanLibrary returns once the scan wrote every title: its library item id.
	items := map[string]string{}
	for _, ti := range lib.Titles {
		items[ti.ID()] = "lib-" + ti.ID()
	}
	run := func() map[int]string {
		admin, _, err := ensureAdmin(ctx, st)
		if err != nil {
			t.Fatal(err)
		}
		approver := accept.NewApprover(st, approval.New(st, nil, nil, slog.New(slog.DiscardHandler)), time.Now)
		ids := map[int]string{}
		for _, c := range lib.Channels {
			id, err := ensureChannel(ctx, st, approver, lib, items, admin.ID, c)
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
	if len(channels) != len(lib.Channels) {
		t.Fatalf("channels after two runs = %d, want %d", len(channels), len(lib.Channels))
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
	// #1534: GET /v1/proposal-jobs lists through the proposal flow, which rejects a done job
	// with no succeeded attempt. A raw seeded job made every demo-seeded Requests page error.
	flow := workflow.New(st, func() string { return "unused" }, time.Now)
	journeys, err := flow.List(ctx, workflow.Viewer{UserID: "demo-admin", Admin: true}, workflow.ListOptions{})
	if err != nil {
		t.Fatalf("list demo-seeded proposal jobs through the workflow: %v", err)
	}
	if len(journeys) != len(lib.Channels) {
		t.Errorf("journeys = %d, want one per demo channel (%d)", len(journeys), len(lib.Channels))
	}
	for _, c := range lib.Channels {
		for _, id := range c.Titles {
			ti, _ := lib.ByID(id)
			rec, err := st.GetTitle(ctx, provision.Key(ti.Key()))
			if err != nil {
				t.Fatalf("title %s: %v", ti.Key(), err)
			}
			// The demo plays only its scanned files (#2151 step 16): every title names its library item.
			if rec.State != provision.Available || rec.LibraryID != items[ti.ID()] {
				t.Errorf("title %q = %s in library %q, want available as %s", ti.Name, rec.State, rec.LibraryID, items[ti.ID()])
			}
		}
	}
}
