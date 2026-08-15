// Package integration holds the end-to-end gate: it wires the REAL domain objects
// (the suggester + its grounding loop, the worker pool, the store, the channel
// engine, the reconciler, the programmer, the filler pod assembler, the approval
// path) and drives the WHOLE chain — a natural-language intent all the way to a
// pushed Tunarr lineup — through the real HTTP API, faking only the two true
// external boundaries: the LLM (scripted testkit double) and Tunarr (testkit
// double). Nothing about Loomarr's own logic is stubbed or hand-rolled.
//
// This is the test whose ABSENCE let the live smoke find unwired seams. It proves,
// in one run:
//   - intent → the REAL suggester grounds picks + extracts a ChannelPolicy;
//   - approve (the real gate) → titles + the grounded channel commit together;
//   - reconcile → the pushed lineup ENFORCES the policy (an over-ceiling title is
//     excluded, in-scope titles air) with commercial pod breaks;
//   - a second reconcile is a no-op.
package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/binder"
	"github.com/mantonx/loomarr/internal/catalog"
	"github.com/mantonx/loomarr/internal/channels"
	"github.com/mantonx/loomarr/internal/filler"
	"github.com/mantonx/loomarr/internal/library"
	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
	"github.com/mantonx/loomarr/internal/suggest"
	"github.com/mantonx/loomarr/internal/testkit"
	"github.com/mantonx/loomarr/internal/tmdb"
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

// libDurationResolver adapts library.Client's ItemDurationMs to the engine's
// DurationResolver (mirrors main.go), so a program slot gets its real runtime from
// the media server — which is what makes the commercial-break interleave fire.
func libDurationResolver(lib *library.Client) channels.DurationResolver {
	return func(ctx context.Context, itemID string) (int64, error) { return lib.ItemDurationMs(ctx, itemID) }
}

// rig is the fully-wired system under test: a real API server backed by the real
// suggester (over the scriptable LLM + media server + TMDB), the real worker pool,
// and the real channel engine + filler assembler. External boundaries faked: LLM
// and Tunarr.
type rig struct {
	srv   *httptest.Server
	store store.Store
	tun   *testkit.Tunarr
	llm   *testkit.LLM
	clock func() time.Time
}

func newRig(t *testing.T, ms *testkit.MediaServer, llmMock *testkit.LLM) *rig {
	t.Helper()
	st, err := store.Open(context.Background(), "sqlite://"+t.TempDir()+"/integration.db", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	tun := testkit.NewTunarr()
	clock := func() time.Time { return time.Unix(1_800_000_000, 0).UTC() }

	// The REAL suggester over the real catalog (real library search against the
	// scriptable media server + real TMDB double) and the scripted LLM. This is the
	// actual grounding path — the LLM can only pick ids the catalog surfaced.
	lib := library.New(library.Emby, ms.URL, ms.AdminToken, "dev-1")
	mt := testkit.NewTMDB(t)
	tm := tmdb.NewWithBase(mt.URL, "key")
	cat := catalog.New(lib, tm)
	suggester := suggest.New(llmMock, cat, tm, 10)

	// The REAL worker service that runs jobs → persists proposals.
	ids := newIDGen()
	svc := suggest.NewService(st, suggester, suggest.Config{Workers: 1, Timeout: 30 * time.Second}, ids, clock, testkit.Logger())

	// Real availability (over the store's title records) WITH a real duration
	// resolver, so program slots carry runtime and the break interleave fires.
	avail := channels.NewStoreAvailability(context.Background(), st, libDurationResolver(lib), nil)

	engine := channels.New(st, tun, avail, nil, channels.Config{
		Policy: schedule.PodFill, ReconcileTTL: 10 * time.Minute, BreaksPerHour: 4,
	}, clock, testkit.Logger())
	engine.WithPods(filler.NewPodAdapter(clipCatalog{st}, nil, testkit.Logger()))

	// Run the worker pool for the life of the test (real async path).
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go svc.Run(ctx)

	log := slog.New(slog.DiscardHandler)
	chBinder := binder.New(st, engine, nil, log)
	approver := suggest.NewApprover(st, chBinder, clock)
	// engine (*channels.Engine) satisfies binder.Reconciler directly, so the shared
	// binder here reconciles through the SAME real engine the rig otherwise wires —
	// no drift between what the API's approve/create paths and the binder see.
	h := api.Router(log, api.Options{
		Store:    st,
		Auth:     api.NewTokenAuthorizer(adminToken),
		Log:      log,
		Channels: engine,
		Suggest:  svc, // *suggest.Service satisfies api.SuggestService directly
		Approver: approver,
		Binder:   chBinder,
	})
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return &rig{srv: srv, store: st, tun: tun, llm: llmMock, clock: clock}
}

// newIDGen returns a deterministic sequential id generator (job/proposal ids).
func newIDGen() func() string {
	n := 0
	return func() string { n++; return fmt.Sprintf("id-%d", n) }
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

// getJSON does a GET and decodes the JSON body into v.
func (r *rig) getJSON(t *testing.T, path string, v any) {
	t.Helper()
	resp := r.do(t, http.MethodGet, path, "")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s → %d", path, resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
}

// awaitProposal polls the submitted-proposals queue until the job's proposal
// appears (the worker ran) or the deadline passes. Returns the proposal id + the
// decoded proposal. A worker failure (no proposal) fails the test with the job's
// last error, so a broken suggester surfaces clearly rather than as a timeout.
func (r *rig) awaitProposal(t *testing.T, jobID string) (string, suggest.Proposal) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var list struct {
			Proposals []struct {
				ID       string          `json:"id"`
				JobID    string          `json:"jobId"`
				Proposal json.RawMessage `json:"proposal"`
			} `json:"proposals"`
		}
		r.getJSON(t, "/v1/proposals?status=submitted", &list)
		for _, p := range list.Proposals {
			if p.JobID == jobID {
				var prop suggest.Proposal
				if err := json.Unmarshal(p.Proposal, &prop); err != nil {
					t.Fatalf("decode proposal body: %v", err)
				}
				return p.ID, prop
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	// No proposal — surface the job's failure reason if there is one.
	job, err := r.store.GetJob(context.Background(), jobID)
	if err == nil {
		t.Fatalf("proposal never appeared for job %s (status=%s, err=%q)", jobID, job.Status, job.LastError)
	}
	t.Fatalf("proposal never appeared for job %s (job lookup failed: %v)", jobID, err)
	return "", suggest.Proposal{}
}

// seedFillerClips lands a catalog of matched commercials + bumpers as a Tunarr sync
// would (identity = Tunarr program uuid) — 90s, general/kids audience.
func seedFillerClips(t *testing.T, st store.Store) {
	t.Helper()
	clips := []filler.Clip{
		{TunarrProgramID: "bump-1", Name: "Bumper", Kind: filler.Bumper, Era: 1992, Audience: filler.Kids, DurationMs: 5000, Source: "tunarr-local"},
		{TunarrProgramID: "ad-1", Name: "Cereal ad", Kind: filler.Commercial, Era: 1992, Audience: filler.Kids, Category: "cereal", DurationMs: 30000, Source: "tunarr-local"},
		{TunarrProgramID: "ad-2", Name: "Toy ad", Kind: filler.Commercial, Era: 1992, Audience: filler.Kids, Category: "toys", DurationMs: 30000, Source: "tunarr-local"},
		{TunarrProgramID: "ad-3", Name: "Snack ad", Kind: filler.Commercial, Era: 1992, Audience: filler.Kids, Category: "snacks", DurationMs: 30000, Source: "tunarr-local"},
	}
	for _, c := range clips {
		if err := st.UpsertClip(context.Background(), store.Clip{Clip: c, UpdatedAt: time.Unix(1_800_000_000, 0)}); err != nil {
			t.Fatal(err)
		}
	}
}

const hourTicks = int64(3600) * 10_000_000 // 1h in Emby RunTimeTicks (100ns units)

// TestPipeline_KidsChannel_EndToEnd is the iron-clad gate. It drives a "90s
// Saturday morning cartoons for kids" intent through the ENTIRE real stack and
// proves the ChannelPolicy actually protects the channel:
//
//   - three kids-rated cartoons are in the library (TV-Y7/TV-G);
//   - one TV-MA "adult cartoon" is ALSO in the library and matches the theme;
//   - the real suggester grounds all four (they're real surfaced ids) and extracts
//     a TV-Y7 policy from the intent;
//   - enforcement puts the three kids titles on the channel and EXCLUDES the TV-MA
//     one — proving fail-closed audience runs end to end, not just in unit tests.
func TestPipeline_KidsChannel_EndToEnd(t *testing.T) {
	ms := testkit.NewMediaServer(t)
	// Real in-library titles with real ratings + runtimes. All four match the theme
	// term "cartoon" so the model sees them all — enforcement, not search, is what
	// must keep the adult one off a kids channel.
	ms.SetSearchItems(
		testkit.SearchStub{Terms: []string{"cartoon"}, LibraryItemID: "lib-1", Name: "Sunny Toon Hour", Type: "Movie", Year: 1992, TMDBID: 5001, Genres: []string{"Animation"}, OfficialRating: "TV-Y7", RunTimeTicks: hourTicks},
		testkit.SearchStub{Terms: []string{"cartoon"}, LibraryItemID: "lib-2", Name: "Robo Rangers", Type: "Movie", Year: 1993, TMDBID: 5002, Genres: []string{"Animation"}, OfficialRating: "TV-Y7", RunTimeTicks: hourTicks},
		testkit.SearchStub{Terms: []string{"cartoon"}, LibraryItemID: "lib-3", Name: "Critter Club", Type: "Movie", Year: 1994, TMDBID: 5003, Genres: []string{"Animation"}, OfficialRating: "TV-Y", RunTimeTicks: hourTicks},
		testkit.SearchStub{Terms: []string{"cartoon"}, LibraryItemID: "lib-4", Name: "Midnight Mayhem Toons", Type: "Movie", Year: 1994, TMDBID: 5004, Genres: []string{"Animation"}, OfficialRating: "TV-MA", RunTimeTicks: hourTicks},
	)

	// The scripted LLM: turn 1 searches the catalog by term (grounding it to the real
	// library results — the stubs answer "cartoon"); turn 2 returns picks for ALL
	// FOUR real ids it saw + a policy that a kids-cartoon intent implies (TV-Y7
	// ceiling). The model proposes the adult toon too — the ENFORCER must be what
	// drops it, not the model's discretion.
	llmMock := testkit.NewLLM(
		testkit.ToolCallResponse("catalog_search", map[string]any{"query": "cartoon"}),
		testkit.FinalResponse(`{
			"rationale":"90s Saturday morning cartoons",
			"picks":[
				{"mediaType":"movie","tmdbId":5001,"name":"Sunny Toon Hour"},
				{"mediaType":"movie","tmdbId":5002,"name":"Robo Rangers"},
				{"mediaType":"movie","tmdbId":5003,"name":"Critter Club"},
				{"mediaType":"movie","tmdbId":5004,"name":"Midnight Mayhem Toons"}
			],
			"policy":{"audience":{"ceiling":"TV-Y7"},"era":{"from":1990,"to":1999},"genres":{"include":["Animation"]},"ordering":"syndication"}
		}`),
	)

	r := newRig(t, ms, llmMock)
	seedFillerClips(t, r.store)

	// 1. SUBMIT the intent → the real worker runs the real suggester.
	resp := r.do(t, http.MethodPost, "/v1/proposals",
		`{"description":"90s Saturday morning cartoons for kids"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("submit → %d, want 200", resp.StatusCode)
	}
	var submitted struct {
		JobID string `json:"jobId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&submitted); err != nil {
		t.Fatalf("decode submit response: %v", err)
	}
	_ = resp.Body.Close()
	jobID := submitted.JobID
	if jobID == "" {
		t.Fatal("submit returned no jobId")
	}

	// 2. The worker generated a GROUNDED proposal (the real grounding loop ran).
	propID, prop := r.awaitProposal(t, jobID)

	// The suggester grounded all four real cartoons (grounding surfaced them; the model didn't
	// invent them) and then REFUSED the one its own extracted ceiling cannot air (#259), so the
	// operator is asked to approve three, not four.
	//
	// ⚠ This assertion used to be `len(prop.Lineup) != 4`, and it passing was the bug: the
	// approval screen offered a TV-MA title that the §4 gate would silently drop downstream, so
	// the operator authorised four titles and got three. The refusal is now visible BEFORE
	// approval — but the enforcement assertions further down are deliberately kept, because
	// "the proposal no longer offers it" must not become the only thing standing between a
	// kids channel and a TV-MA title.
	if len(prop.Lineup) != 3 {
		t.Fatalf("expected 3 airable in-library picks, got %d: %+v", len(prop.Lineup), prop.Lineup)
	}
	if len(prop.Refused) != 1 {
		t.Fatalf("expected the TV-MA toon to be refused at proposal time, got %+v", prop.Refused)
	}
	if prop.Refused[0].Item.Name != "Midnight Mayhem Toons" || prop.Refused[0].Reason != "over_ceiling" {
		t.Errorf("refused = %+v, want Midnight Mayhem Toons / over_ceiling", prop.Refused[0])
	}
	// ⚠ Refused, NOT deleted. The operator's likely fix is to raise the ceiling, and a pick
	// that vanished between the model's answer and the approval card is indistinguishable from
	// one the model never made.
	for _, it := range prop.Lineup {
		if it.TMDBID == 5004 {
			t.Fatal("the TV-MA toon is still in the lineup — it must move to Refused, not stay")
		}
	}
	// The suggester EXTRACTED + grounded the policy (TV-Y7 ceiling from the intent).
	if prop.Policy.Audience.Ceiling != "TV-Y7" {
		t.Fatalf("suggester should have extracted a TV-Y7 ceiling, got %q", prop.Policy.Audience.Ceiling)
	}

	// 3. APPROVE (the real gate) → in-library picks and the local channel commit together.
	resp = r.do(t, http.MethodPost, "/v1/proposals/"+propID+"/approve", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("approve → %d, want 200", resp.StatusCode)
	}
	var approved struct {
		ChannelID string `json:"channelId"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&approved); err != nil {
		t.Fatalf("decode approve response: %v", err)
	}
	_ = resp.Body.Close()
	if approved.ChannelID == "" {
		t.Fatal("approve returned no channel id")
	}
	for _, key := range []string{"movie:tmdb:5001", "movie:tmdb:5002", "movie:tmdb:5003"} {
		if rec, err := r.store.GetTitle(context.Background(), provision.Key(key)); err != nil || rec.State != "available" {
			t.Fatalf("approve should mark %s available, got %v/%v", key, rec.State, err)
		}
	}
	// ⚠ The REFUSED pick is not provisioned at all. Approval acts on what was offered, and
	// offering was the thing #259 fixed — provisioning a title the ceiling will drop spends an
	// acquisition (on the out-of-library path, a real download) for something that can never air.
	if _, err := r.store.GetTitle(context.Background(), provision.Key("movie:tmdb:5004")); err == nil {
		t.Error("the refused TV-MA toon was provisioned — approval must act only on what it offered")
	}

	// 4. The same approval kicked a real reconcile through the real engine → Tunarr.
	// The grounded policy landed on the channel row (enforcement input).
	ch, _ := r.store.GetChannel(context.Background(), approved.ChannelID)
	if ch.Policy.Audience.Ceiling != "TV-Y7" {
		t.Fatalf("channel should carry the TV-Y7 policy, got %q", ch.Policy.Audience.Ceiling)
	}
	if ch.TunarrID == "" {
		t.Fatal("server-assigned TunarrID not persisted")
	}

	// 5. THE IRON-CLAD ASSERTION: the pushed Tunarr lineup ENFORCES the policy.
	lineup, err := r.tun.GetLineup(context.Background(), ch.TunarrID)
	if err != nil {
		t.Fatal(err)
	}
	programItems := map[string]bool{}
	programs, flex := 0, 0
	for _, s := range lineup {
		switch s.Kind {
		case schedule.SlotProgram:
			programs++
			programItems[s.LibraryItemID] = true
		case schedule.SlotFlex:
			flex++
		}
	}
	// The three KIDS cartoons air...
	for _, want := range []string{"lib-1", "lib-2", "lib-3"} {
		if !programItems[want] {
			t.Errorf("kids cartoon %s should be on the channel, got programs=%v", want, programItems)
		}
	}
	// ...and the TV-MA "adult" cartoon is EXCLUDED — fail-closed audience, end to end.
	if programItems["lib-4"] {
		t.Fatal("TV-MA 'Midnight Mayhem Toons' reached a TV-Y7 kids channel — audience enforcement was violated end-to-end")
	}
	if programs != 3 {
		t.Errorf("expected exactly 3 program slots (the 3 kids cartoons), got %d", programs)
	}

	// 6. COMMERCIAL POD BREAKS: the interleave inserted flex gaps (real runtimes drove
	//    the cadence), and a grounded filler-list is attached for Tunarr to fill them.
	if flex == 0 {
		t.Error("no flex break gaps — the commercial-break interleave didn't run (durations missing?)")
	}
	pool := r.tun.FillerListFor(ch.TunarrID)
	if len(pool) == 0 {
		t.Error("no filler-list attached — commercials never reached Tunarr")
	}
	for _, id := range pool {
		if !strings.HasPrefix(id, "ad-") && !strings.HasPrefix(id, "bump-") {
			t.Errorf("filler-list has an ungrounded clip id %q", id)
		}
	}

	// 7. IDEMPOTENCY: a second reconcile with nothing changed makes no new Tunarr
	//    writes (§9/§10) — the enforced lineup is stable, not re-shuffled each sweep.
	pushesBefore, fillerBefore := r.tun.Pushes, r.tun.FillerWrites
	if resp := r.do(t, http.MethodPost, "/v1/channels/"+approved.ChannelID+"/reconcile", ""); resp.StatusCode != http.StatusOK {
		t.Fatalf("reconcile → %d, want 200", resp.StatusCode)
	}
	if r.tun.Pushes != pushesBefore {
		t.Errorf("second reconcile re-pushed the lineup: %d → %d", pushesBefore, r.tun.Pushes)
	}
	if r.tun.FillerWrites != fillerBefore {
		t.Errorf("second reconcile re-wrote the filler list: %d → %d", fillerBefore, r.tun.FillerWrites)
	}

	// 8. THE ENFORCER, STILL EXERCISED END TO END.
	//
	// ⚠ This step exists because #259 took coverage away from step 5. The TV-MA toon is now
	// refused at PROPOSAL time, so it never reaches the channel — which means step 5 no longer
	// proves the §4 gate would have dropped it, only that nothing offered it. Two independent
	// defences must both be provable, or the one that stays silent is the one that rots.
	//
	// So: tighten the LIVE channel's ceiling to TV-Y, which puts the two already-approved,
	// already-airing TV-Y7 cartoons above it. Nothing here bypasses the approval gate — the
	// titles are legitimately available, and lowering a ceiling is an ordinary operator edit.
	// ⚠ The full policy, not just the ceiling: a policy PATCH REPLACES wholesale, so era/genres
	// are restated or the tightening would also quietly widen the scope. And era/genres live
	// under `scope` — the flat shape is the LLM's, which groundPolicy converts.
	current, err := r.store.GetChannel(context.Background(), approved.ChannelID)
	if err != nil {
		t.Fatal(err)
	}
	if resp := r.do(t, http.MethodPatch, "/v1/channels/"+approved.ChannelID,
		fmt.Sprintf(`{"revision":%d,"policy":{"scope":{"era":{"from":1990,"to":1999},"genres":{"include":["Animation"]}},`, current.Revision)+
			`"audience":{"ceiling":"TV-Y"},"ordering":"syndication"}}`,
	); resp.StatusCode != http.StatusOK {
		t.Fatalf("tighten ceiling → %d, want 200", resp.StatusCode)
	}

	tightened, err := r.tun.GetLineup(context.Background(), ch.TunarrID)
	if err != nil {
		t.Fatal(err)
	}
	airing := map[string]bool{}
	for _, s := range tightened {
		if s.Kind == schedule.SlotProgram {
			airing[s.LibraryItemID] = true
		}
	}
	// lib-3 is TV-Y and survives; lib-1 and lib-2 are TV-Y7 and must now be dropped by the
	// enforcer alone — no proposal, no approval, just the fail-closed gate on a live channel.
	if !airing["lib-3"] {
		t.Errorf("the TV-Y cartoon should still air under a TV-Y ceiling, got %v", airing)
	}
	if airing["lib-1"] || airing["lib-2"] {
		t.Fatalf("a TV-Y7 title survived a TV-Y ceiling — the §4 enforcer did not run on the "+
			"tightened policy, got %v", airing)
	}
}
