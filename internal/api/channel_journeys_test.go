package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/binder"
	"github.com/loomarr/loomarr/internal/channels"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/reconcile"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/suggest"
	"github.com/loomarr/loomarr/internal/testkit"
	"github.com/loomarr/loomarr/internal/testkit/channeljourney"
	"github.com/loomarr/loomarr/internal/testkit/libraryfixture"
)

// These fixtures start at a submitted, controlled Proposal. They prove the
// approval-to-schedule seam, not inference or historical membership accuracy.
func TestReferenceChannelJourneys(t *testing.T) {
	for _, tc := range append(channeljourney.Cases(), channeljourney.EligibilityControls()...) {
		t.Run(tc.Name, func(t *testing.T) {
			ch, slots, _ := approveReferenceJourney(t, tc)
			var got []string
			for _, slot := range slots {
				if slot.IsProgram() {
					got = append(got, slot.LibraryItemID)
				}
			}
			if !slices.Equal(got, tc.Want) {
				t.Fatalf("scheduled identities = %v, want %v", got, tc.Want)
			}
			var persisted []string
			for _, slot := range ch.Desired {
				if slot.IsProgram() {
					persisted = append(persisted, slot.LibraryItemID)
				}
			}
			if !slices.Equal(persisted, got) {
				t.Fatalf("persisted schedule %v differs from preview %v", persisted, got)
			}
			for _, forbidden := range tc.Forbidden {
				if slices.Contains(got, forbidden) {
					t.Errorf("forbidden program %q aired", forbidden)
				}
			}
			t.Logf("%s/%s: ordinary approval and concrete schedule PASS (%d programs); provider qualification unrun", channeljourney.Version, tc.Name, len(got))
		})
	}
}

// approveReferenceJourney preserves the ordinary approval boundary for all reference
// variants. It returns durable scheduling state, the production preview, and the
// public Channel response so reconciliation notes cannot disappear at the API seam.
func approveReferenceJourney(t *testing.T, tc channeljourney.Case) (store.Channel, []schedule.Slot, api.ChannelDTO) {
	t.Helper()
	ctx := context.Background()
	st := testkit.MigratedSQLiteStore(t)
	log := testkit.Logger()
	b := binder.New(st, nil, nil, log)
	srv := httptest.NewServer(api.Router(log, api.Options{
		Store: st, Auth: testAuthorizer{}, Log: log, Binder: b,
		Approver: suggest.NewApprover(st, b, channeljourney.Clock),
	}))
	t.Cleanup(srv.Close)
	body, err := json.Marshal(tc.Proposal)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.CreateProposal(ctx, store.Proposal{
		ID: "reference", JobID: "reference-job", Status: "submitted",
		ProposalJSON: string(body), CreatedBy: "alice",
	}); err != nil {
		t.Fatal(err)
	}
	// A fixture may never seed available records or an approved Channel.
	before, err := st.ListTitlesByState(ctx, provision.Available)
	if err != nil || len(before) != 0 {
		t.Fatalf("before approval: %v, %v", before, err)
	}
	resp := do(t, srv, http.MethodPost, "/v1/proposals/reference/approve", adminToken, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("approve: %d", resp.StatusCode)
	}
	var out struct {
		ChannelID string `json:"channelId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.ChannelID == "" {
		t.Fatal("approval omitted Channel identity")
	}
	episodes := libraryfixture.NewEpisodes(tc.Episodes)
	avail := channels.NewStoreAvailability(ctx, st,
		func(context.Context, string) (int64, error) { return time.Hour.Milliseconds(), nil }, episodes.Resolve)
	engine := channels.New(st, nil, avail, nil, channels.Config{
		ResolvePlayoutBackendContext: func(context.Context) (string, error) { return schedule.PlayoutBackendInternal, nil },
	}, channeljourney.Clock, log)
	if err := engine.Reconcile(ctx, out.ChannelID); err != nil {
		t.Fatal(err)
	}
	ch, err := st.GetChannel(ctx, out.ChannelID)
	if err != nil {
		t.Fatal(err)
	}
	if ch.Status != schedule.StatusLive {
		t.Fatalf("status = %s", ch.Status)
	}
	at, slots, _, _, err := engine.CyclePreview(ctx, ch.ID, channeljourney.Clock())
	if err != nil {
		t.Fatal(err)
	}
	if !at.Equal(channeljourney.Clock()) {
		t.Fatalf("clock drift: %v", at)
	}

	response := do(t, srv, http.MethodGet, "/v1/channels/"+ch.ID, adminToken, "")
	if response.StatusCode != http.StatusOK {
		t.Fatalf("read Channel: %d", response.StatusCode)
	}
	var public api.ChannelDTO
	if err := json.NewDecoder(response.Body).Decode(&public); err != nil {
		t.Fatal(err)
	}
	return ch, slots, public
}

func TestReferenceChannelAcquisitionJourney(t *testing.T) {
	for _, arrives := range []bool{true, false} {
		name := "acquisition_failure"
		if arrives {
			name = "acquisition_arrival"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			now := channeljourney.Clock()
			st := testkit.MigratedSQLiteStore(t)
			log := testkit.Logger()
			b := binder.New(st, nil, nil, log)
			srv := httptest.NewServer(api.Router(log, api.Options{
				Store: st, Auth: testAuthorizer{}, Log: log, Binder: b,
				Approver: suggest.NewApprover(st, b, func() time.Time { return now }),
			}))
			t.Cleanup(srv.Close)
			// An entirely acquisition-dependent sparse library must not appear live.
			fixture := channeljourney.Cases()[1]
			pick := fixture.Proposal.Lineup[0]
			pick.InLibrary, pick.LibraryItemID = false, ""
			fixture.Proposal.Lineup = nil
			fixture.Proposal.Acquisitions = []suggest.ProposalItem{pick}
			raw, err := json.Marshal(fixture.Proposal)
			if err != nil {
				t.Fatal(err)
			}
			if err := st.CreateProposal(ctx, store.Proposal{ID: "sparse", JobID: "sparse-job", Status: "submitted", ProposalJSON: string(raw)}); err != nil {
				t.Fatal(err)
			}
			resp := do(t, srv, http.MethodPost, "/v1/proposals/sparse/approve", adminToken, "")
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("approve: %d", resp.StatusCode)
			}
			var out struct {
				ChannelID string `json:"channelId"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
				t.Fatal(err)
			}
			key, err := pick.Key()
			if err != nil {
				t.Fatal(err)
			}
			record, err := st.GetTitle(ctx, key)
			if err != nil || record.State != provision.Wanted {
				t.Fatalf("approved acquisition: %+v, %v", record, err)
			}
			materialize := func() store.Channel {
				t.Helper()
				// A fresh production availability adapter also proves reconstruction
				// from durable state without relying on an in-process arrival event.
				avail := channels.NewStoreAvailability(ctx, st, func(context.Context, string) (int64, error) { return time.Hour.Milliseconds(), nil }, nil)
				engine := channels.New(st, nil, avail, nil, channels.Config{
					ResolvePlayoutBackendContext: func(context.Context) (string, error) { return schedule.PlayoutBackendInternal, nil },
				}, func() time.Time { return now }, log)
				if err := engine.Reconcile(ctx, out.ChannelID); err != nil {
					t.Fatal(err)
				}
				ch, err := st.GetChannel(ctx, out.ChannelID)
				if err != nil {
					t.Fatal(err)
				}
				return ch
			}
			before := materialize()
			if before.Status == schedule.StatusLive {
				t.Fatal("acquisition-only Channel reported live")
			}
			for _, slot := range before.Desired {
				if slot.IsProgram() {
					t.Fatal("unavailable acquisition became playable")
				}
			}
			requester := &testkit.Requester{}
			lookup := libraryfixture.NewLookup(map[string]libraryfixture.LookupResult{"91001": {ItemID: "signal", Present: arrives}})
			rc := reconcile.New(st, requester, lookup, nil, reconcile.Config{RequestTTL: time.Hour}, func() time.Time { return now }, log)
			if count, err := rc.Tick(ctx); err != nil || count != 1 {
				t.Fatalf("request tick: %d, %v", count, err)
			}
			record, err = st.GetTitle(ctx, key)
			if err != nil || record.State != provision.Requested || requester.RequestCount() != 1 {
				t.Fatalf("request not durably accepted: %+v, %v", record, err)
			}
			now = now.Add(2 * time.Hour)
			if count, err := rc.Tick(ctx); err != nil || count != 1 {
				t.Fatalf("completion tick: %d, %v", count, err)
			}
			record, err = st.GetTitle(ctx, key)
			if err != nil {
				t.Fatal(err)
			}
			wantState := provision.Unavailable
			if arrives {
				wantState = provision.Available
			}
			if record.State != wantState {
				t.Fatalf("completion state = %s, want %s", record.State, wantState)
			}
			after := materialize()
			var programs []string
			for _, slot := range after.Desired {
				if slot.IsProgram() {
					programs = append(programs, slot.LibraryItemID)
				}
			}
			if arrives {
				if after.Status != schedule.StatusLive || !slices.Equal(programs, []string{"signal"}) {
					t.Fatalf("arrival schedule = %s %v", after.Status, programs)
				}
			} else if after.Status == schedule.StatusLive || len(programs) != 0 || requester.CancelCount() != 1 {
				t.Fatalf("failed acquisition reported success: %s %v cancels=%d", after.Status, programs, requester.CancelCount())
			}
		})
	}
}
