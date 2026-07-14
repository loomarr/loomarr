// Package integration holds the Phase-12.5 end-to-end gate: it wires the REAL
// domain objects (store, channel engine, reconciler, programmer, filler pod
// assembler, approval path) and drives the whole chain through the real HTTP API,
// faking only the external boundary (Tunarr via the testkit double). This is the
// test whose ABSENCE let the live smoke find unwired seams — each per-phase unit
// gate passed while the composition was never exercised together. The gate
// (design §21 phase 12.5): intent → approve → create → reconcile → a pushed Tunarr
// lineup with real programs AND commercial pod breaks.
package integration_test

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/channels"
	"github.com/mantonx/loomarr/internal/filler"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/testkit"
)

const adminToken = "admin-integration-token"

// clipCatalog adapts the store to filler.CatalogReader (mirrors main.go's
// clipCatalogAdapter — the pod assembler's catalog source).
type clipCatalog struct{ st store.Store }

func (a clipCatalog) AllClips(ctx context.Context) ([]filler.Clip, error) {
	clips, err := a.st.ListClips(ctx, store.ClipFilter{})
	if err != nil {
		return nil, err
	}
	out := make([]filler.Clip, len(clips))
	for i, c := range clips {
		out[i] = c.Clip
	}
	return out, nil
}

// rig is the fully-wired system under test: a real API server backed by the real
// channel engine (real availability + real filler pod assembler), with the
// external Tunarr faked by the testkit double.
type rig struct {
	srv   *httptest.Server
	store store.Store
	tun   *testkit.Tunarr
}

func newRig(t *testing.T) *rig {
	t.Helper()
	st, err := store.Open(context.Background(), "sqlite://"+t.TempDir()+"/integration.db", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	tun := testkit.NewTunarr()
	clock := func() time.Time { return time.Unix(1_800_000_000, 0).UTC() }

	// Real availability over the store's title records (production path): approve
	// writes `available` records, and this resolves them — no mapAvail fake.
	avail := channels.NewStoreAvailability(context.Background(), st, nil, nil)

	// Real channel engine with the real filler pod assembler wired in, plus the
	// commercial-break density so ComputeDesired interleaves break gaps (§10).
	engine := channels.New(st, tun, avail, nil, channels.Config{
		Policy:        schedule.PodFill,
		ReconcileTTL:  10 * time.Minute,
		BreaksPerHour: 4, // ~every 15 min of program runtime (§10)
	}, clock, testkit.Logger())
	engine.WithPods(filler.NewPodAdapter(clipCatalog{st}, filler.Policy{}, testkit.Logger()))

	h := api.Router(slog.New(slog.DiscardHandler), api.Options{
		Store:    st,
		Auth:     api.NewTokenAuthorizer(adminToken),
		Log:      slog.New(slog.DiscardHandler),
		Channels: engine, // the REAL engine — createChannel drives a real reconcile
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &rig{srv: srv, store: st, tun: tun}
}

func (r *rig) do(t *testing.T, method, path, body string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(method, r.srv.URL+path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Loomarr-Csrf", "1")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

// seedFillerClips lands a catalog of matched commercials + bumpers as a Tunarr
// sync would (identity = Tunarr program uuid). Enough for a pod (bumpers +
// era/audience-matched commercials of varied categories).
func seedFillerClips(t *testing.T, st store.Store) {
	t.Helper()
	clips := []filler.Clip{
		{TunarrProgramID: "bump-1", Name: "Bumper", Kind: filler.Bumper, Era: 1992, Audience: filler.General, DurationMs: 5000, Source: "tunarr-local"},
		{TunarrProgramID: "ad-1", Name: "Cereal ad", Kind: filler.Commercial, Era: 1992, Audience: filler.General, Category: "cereal", DurationMs: 30000, Source: "tunarr-local"},
		{TunarrProgramID: "ad-2", Name: "Toy ad", Kind: filler.Commercial, Era: 1992, Audience: filler.General, Category: "toys", DurationMs: 30000, Source: "tunarr-local"},
		{TunarrProgramID: "ad-3", Name: "Tech ad", Kind: filler.Commercial, Era: 1992, Audience: filler.General, Category: "tech", DurationMs: 30000, Source: "tunarr-local"},
	}
	for _, c := range clips {
		if err := st.UpsertClip(context.Background(), store.Clip{Clip: c, UpdatedAt: time.Unix(1_800_000_000, 0)}); err != nil {
			t.Fatal(err)
		}
	}
}

// seedSubmittedProposal writes a grounded, SUBMITTED proposal (as the suggester +
// approval gate would persist it) with several in-library picks (enough program
// runtime that the break interleave fires) and one acquisition. The maintainer
// decision was to seed here rather than drive the live LLM: the suggester's own
// grounding is covered by phase-11 gates; this test is about the composition seams.
func seedSubmittedProposal(t *testing.T, st store.Store, jobID, propID string) {
	t.Helper()
	// Three in-library movies (long runtimes → the 4/hr break cadence interleaves
	// break gaps between them) + one acquisition (not yet in library).
	proposalJSON := `{
		"intent": {"description":"90s movie marathon"},
		"lineup": [
			{"mediaType":"movie","tmdbId":1,"name":"Movie One","year":1992,"inLibrary":true,"libraryItemId":"lib-1"},
			{"mediaType":"movie","tmdbId":2,"name":"Movie Two","year":1994,"inLibrary":true,"libraryItemId":"lib-2"},
			{"mediaType":"movie","tmdbId":3,"name":"Movie Three","year":1995,"inLibrary":true,"libraryItemId":"lib-3"}
		],
		"acquisitions": [
			{"mediaType":"movie","tmdbId":9,"name":"Missing Movie","year":1993,"inLibrary":false}
		]
	}`
	if err := st.CreateProposal(context.Background(), store.Proposal{
		ID: propID, JobID: jobID, Status: "submitted", ProposalJSON: proposalJSON,
	}); err != nil {
		t.Fatal(err)
	}
}

// TestPipeline_ApproveToChannelWithProgramsAndPodBreaks is the Phase-12.5 gate:
// drive approve → create(intentRef) → reconcile through the REAL API + engine and
// assert the pushed Tunarr lineup has real programs AND commercial pod breaks (a
// filler-list attached), then that a second reconcile is a no-op.
func TestPipeline_ApproveToChannelWithProgramsAndPodBreaks(t *testing.T) {
	r := newRig(t)
	seedFillerClips(t, r.store)
	seedSubmittedProposal(t, r.store, "job-1", "prop-1")

	// 1. Approve (admin, real gate): in-library picks → `available` records, the
	//    acquisition → a `wanted` title. This is the only proposal→titles path.
	if resp := r.do(t, http.MethodPost, "/v1/suggestions/prop-1/approve", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("approve → %d, want 200", resp.StatusCode)
	}
	// The in-library picks are now available (the real approve path wrote them).
	if rec, err := r.store.GetTitle(context.Background(), "movie:tmdb:1"); err != nil || rec.State != "available" {
		t.Fatalf("approve should mark in-library pick available, got %v/%v", rec.State, err)
	}
	// The acquisition is a wanted title (the approval gate's only titles output).
	if rec, err := r.store.GetTitle(context.Background(), "movie:tmdb:9"); err != nil || rec.State != "wanted" {
		t.Fatalf("approve should enqueue the acquisition as wanted, got %v/%v", rec.State, err)
	}

	// 2. Create the channel from the approved intent → the real createChannel binds
	//    the lineup AND kicks a real reconcile (→ real engine → testkit Tunarr).
	body := `{"id":"marathon","name":"90s Marathon","number":42,"strategy":"sequential","intentRef":"job-1"}`
	if resp := r.do(t, http.MethodPost, "/v1/channels", body); resp.StatusCode != http.StatusOK {
		t.Fatalf("create channel → %d, want 200", resp.StatusCode)
	}

	// 3. The reconcile ran end-to-end: Tunarr has the channel + a pushed lineup.
	if r.tun.Creates != 1 {
		t.Fatalf("Tunarr channel not created: creates=%d", r.tun.Creates)
	}
	if r.tun.Pushes == 0 {
		t.Fatal("no lineup pushed to Tunarr")
	}
	ch, _ := r.store.GetChannel(context.Background(), "marathon")
	tunarrID := ch.TunarrID
	if tunarrID == "" {
		t.Fatal("server-assigned TunarrID not persisted")
	}

	// 4a. PROGRAMS: the pushed lineup has real program slots (the in-library picks),
	//     not all-flex.
	lineup, err := r.tun.GetLineup(context.Background(), tunarrID)
	if err != nil {
		t.Fatal(err)
	}
	programs, flex := 0, 0
	for _, s := range lineup {
		switch s.Kind {
		case schedule.SlotProgram:
			programs++
		case schedule.SlotFlex:
			flex++
		}
	}
	if programs < 3 {
		t.Errorf("want ≥3 real program slots (the in-library picks), got %d in %+v", programs, lineup)
	}

	// 4b. POD BREAKS: the break interleave inserted flex gaps between programs...
	if flex == 0 {
		t.Error("no flex break gaps between programs — the commercial-break interleave didn't run")
	}
	// ...and the commercials themselves are a Tunarr filler-list attached to the
	// channel (the §10 redesign: Tunarr plays the list INTO those flex gaps).
	pool := r.tun.FillerListFor(tunarrID)
	if len(pool) == 0 {
		t.Error("no filler-list attached — commercials (pod breaks) never reached Tunarr")
	}
	// The pool is drawn from the seeded catalog (real clip uuids, grounded).
	for _, id := range pool {
		if !strings.HasPrefix(id, "ad-") && !strings.HasPrefix(id, "bump-") {
			t.Errorf("filler-list has an ungrounded clip id %q (not from the catalog)", id)
		}
	}

	// 5. IDEMPOTENCY: a second reconcile with nothing changed makes no new Tunarr
	//    writes — neither lineup pushes nor filler-list writes (§9/§10).
	pushesBefore, fillerBefore := r.tun.Pushes, r.tun.FillerWrites
	if resp := r.do(t, http.MethodPost, "/v1/channels/marathon/reconcile", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("reconcile → %d, want 200", resp.StatusCode)
	}
	if r.tun.Pushes != pushesBefore {
		t.Errorf("second reconcile re-pushed the lineup: %d → %d", pushesBefore, r.tun.Pushes)
	}
	if r.tun.FillerWrites != fillerBefore {
		t.Errorf("second reconcile re-wrote the filler list: %d → %d", fillerBefore, r.tun.FillerWrites)
	}
}
