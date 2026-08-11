package api_test

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/binder"
	"github.com/mantonx/loomarr/internal/store"
)

// fakeNumberSource stands in for Tunarr's channel list (binder.NumberSource).
//
// `err` models the case that matters most: Tunarr unreachable. The contract is best-effort —
// numbering must fall back to Loomarr's store rather than refuse to create a channel — so a
// test that only ever returns a healthy map cannot tell a correct fallback from a check that
// silently does nothing.
type fakeNumberSource struct {
	taken map[int]bool
	err   error
	calls int
}

func (f *fakeNumberSource) TakenChannelNumbers(context.Context) (map[int]bool, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.taken, nil
}

// newServerWithNumbers is newServerWithScheduler plus a Tunarr number source, so the create
// and renumber paths can be exercised against numbers that exist ONLY in Tunarr.
func newServerWithNumbers(t *testing.T, nums *fakeNumberSource) (*httptest.Server, store.Store) {
	t.Helper()
	st := openTestStore(t, t.TempDir()+"/api.db")
	t.Cleanup(func() { _ = st.Close() })
	log := slog.New(slog.DiscardHandler)
	chSvc := &fakeChannelSvc{}
	h := api.Router(log, api.Options{
		Store:    st,
		Auth:     testAuthorizer{},
		Log:      log,
		Channels: chSvc,
		Binder:   binder.New(st, chSvc, nil, log).WithChannelNumbers(nums),
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv, st
}

// A number that exists only in TUNARR must be refused at create time, exactly as a number
// held by one of Loomarr's own channels already was (design §2, §9 V54).
//
// Before this, the two answers disagreed for a reason no operator could see: a clash with a
// Loomarr channel got a clean 409 from this handler, while a clash with a Tunarr-only channel
// got a 201 and was then renumbered underneath the operator by the reconcile. #258 unioned the
// two sources for the APPROVE path's numbering and left the handler an operator actually types
// a number into checking `GetChannelByNumber` alone.
func TestCreateChannel_RejectsANumberOnlyTunarrHolds(t *testing.T) {
	nums := &fakeNumberSource{taken: map[int]bool{7: true}}
	srv, st := newServerWithNumbers(t, nums)

	resp := do(t, srv, http.MethodPost, "/v1/channels", adminToken,
		`{"id":"c1","name":"A","number":7,"strategy":"sequential"}`)
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("create on a Tunarr-held number → %d, want 409", resp.StatusCode)
	}
	if nums.calls == 0 {
		t.Error("Tunarr's channel numbers were never consulted")
	}
	// The 409 must also mean NOTHING was written — a rejected create that still persisted a row
	// would leave a channel the operator was told they did not create.
	if _, err := st.GetChannel(context.Background(), "c1"); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("rejected create still wrote a channel row (err = %v)", err)
	}
}

// A free number is still free: the new check must not refuse everything, which is the failure
// mode a conflict-only assertion cannot distinguish from a correct one.
func TestCreateChannel_AllowsANumberNeitherSideHolds(t *testing.T) {
	nums := &fakeNumberSource{taken: map[int]bool{7: true}}
	srv, _ := newServerWithNumbers(t, nums)

	resp := do(t, srv, http.MethodPost, "/v1/channels", adminToken,
		`{"id":"c1","name":"A","number":8,"strategy":"sequential"}`)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("create on a free number → %d, want 200", resp.StatusCode)
	}
}

// ⚠ An unreachable Tunarr must NOT block channel creation. The collision it would have caught
// is recoverable (the reconcile re-checks occupancy at push time and renumbers Loomarr's own
// channel); a create that fails because Tunarr is briefly down is not. This is the same
// best-effort bargain `nextFreeChannelNumber` makes.
func TestCreateChannel_UnreachableTunarrFallsBackToStoreOnly(t *testing.T) {
	nums := &fakeNumberSource{err: errors.New("tunarr unreachable")}
	srv, _ := newServerWithNumbers(t, nums)

	resp := do(t, srv, http.MethodPost, "/v1/channels", adminToken,
		`{"id":"c1","name":"A","number":7,"strategy":"sequential"}`)
	if resp.StatusCode != http.StatusOK {
		t.Errorf("create with Tunarr unreachable → %d, want 200 (store-only fallback)", resp.StatusCode)
	}
	// ...and the Loomarr half of the check still applies while Tunarr is down.
	dup := do(t, srv, http.MethodPost, "/v1/channels", adminToken,
		`{"id":"c2","name":"B","number":7,"strategy":"sequential"}`)
	if dup.StatusCode != http.StatusConflict {
		t.Errorf("duplicate of a LOOMARR number while Tunarr is down → %d, want 409", dup.StatusCode)
	}
}

// Renumbering onto a Tunarr-held number is refused the same way creating on one is.
func TestUpdateChannel_RejectsRenumberOntoANumberOnlyTunarrHolds(t *testing.T) {
	nums := &fakeNumberSource{taken: map[int]bool{7: true}}
	srv, _ := newServerWithNumbers(t, nums)
	_ = do(t, srv, http.MethodPost, "/v1/channels", adminToken,
		`{"id":"c1","name":"A","number":3,"strategy":"sequential"}`)

	resp := do(t, srv, http.MethodPatch, "/v1/channels/c1", adminToken, `{"number":7}`)
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("renumber onto a Tunarr-held number → %d, want 409", resp.StatusCode)
	}
}

// NOTE: the `exceptChannelID` half of this rule is NOT reachable through HTTP — updateChannel
// only runs the check when the number actually CHANGES (`*in.Body.Number != ch.Number`), so a
// channel's own number is never queried against itself here. It is covered where it can be
// exercised honestly, as a binder unit test (internal/binder/numberinuse_test.go), rather than
// by an API test that would pass without the parameter existing.
