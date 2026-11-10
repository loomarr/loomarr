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

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/api/httpcore"
	"github.com/loomarr/loomarr/internal/channels"
	"github.com/loomarr/loomarr/internal/channels/approval"
	"github.com/loomarr/loomarr/internal/channels/schedule"
	"github.com/loomarr/loomarr/internal/filler/clips"
	"github.com/loomarr/loomarr/internal/filler/pod"
	"github.com/loomarr/loomarr/internal/filler/records"
	"github.com/loomarr/loomarr/internal/library/provision"
	"github.com/loomarr/loomarr/internal/library/titlesearch"
	"github.com/loomarr/loomarr/internal/library/tmdb"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
	"github.com/loomarr/loomarr/internal/playout/tunarr/tunarrtest"
	"github.com/loomarr/loomarr/internal/proposals"
	"github.com/loomarr/loomarr/internal/proposals/accept"
	"github.com/loomarr/loomarr/internal/proposals/suggest"
	"github.com/loomarr/loomarr/internal/proposals/workflow"
	"github.com/loomarr/loomarr/internal/testkit"
)

const adminToken = "admin-integration-token"

// clipCatalog adapts the store to pod.CatalogReader (mirrors main.go's
// clipCatalogAdapter — the pod assembler's catalog source).
type clipCatalog struct{ st store.Store }

func (a clipCatalog) AllClips(ctx context.Context) ([]clips.Clip, error) {
	rows, err := a.st.ListClips(ctx, clipstore.ClipFilter{})
	if err != nil {
		return nil, err
	}
	out := make([]clips.Clip, len(rows))
	for i, c := range rows {
		out[i] = c.Clip
	}
	return out, nil
}

// libraryDurationResolver reads an item's runtime from the Library's facts (mirrors the app's
// itemMetadata), so a program slot gets its real runtime — which is what makes the
// commercial-break interleave fire.
func libraryDurationResolver(facts librarystore.LibraryItemFactsReader) channels.DurationResolver {
	return func(ctx context.Context, itemID string) (int64, error) {
		got, err := facts.LibraryItemFacts(ctx, []string{itemID})
		return got[itemID].RuntimeMs, err
	}
}

// rig is the fully-wired system under test: a real API server backed by the real
// suggester (over the scriptable LLM + the store's Library + TMDB), the real worker pool,
// and the real channel engine + filler assembler. External boundaries faked: LLM
// and Tunarr.
type rig struct {
	srv   *httptest.Server
	store store.Store
	tun   *tunarrtest.Tunarr
	llm   *testkit.LLM
	clock func() time.Time
}

func newRig(t *testing.T, llmMock *testkit.LLM) *rig {
	t.Helper()
	st, err := records.Open(context.Background(), testkit.MigratedSQLiteDSN(t), true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	tun := tunarrtest.NewTunarr()
	clock := func() time.Time { return time.Unix(1_800_000_000, 0).UTC() }

	// The REAL suggester over the real catalog (real search of the store's Library + real
	// TMDB double) and the scripted LLM. This is the actual grounding path — the LLM can
	// only pick ids the catalog surfaced.
	mt := testkit.NewTMDB(t)
	tm := tmdb.NewWithBase(mt.URL, "key")
	cat := titlesearch.New(titlesearch.LibrarySearch{Titles: st, Facts: st}, tm)
	suggester := suggest.New(llmMock, cat, tm, 10)

	// The REAL worker service that runs jobs → persists proposals.
	ids := newIDGen()
	svc := suggest.NewService(st, workflow.New(st, ids, clock, suggest.IntentHash), suggester,
		suggest.Config{Workers: 1, Timeout: 30 * time.Second}, clock, testkit.Logger())

	// Real availability (over the store's title records) WITH a real duration
	// resolver, so program slots carry runtime and the break interleave fires.
	avail := channels.NewStoreAvailability(context.Background(), st, libraryDurationResolver(st), nil)

	engine := channels.New(st, tun, avail, nil, channels.Config{
		Policy: schedule.PodFill, ReconcileTTL: 10 * time.Minute, BreaksPerHour: 4,
	}, clock, testkit.Logger())
	engine.WithPods(pod.NewAdapter(clipCatalog{st}, nil, nil, testkit.Logger()))

	// Run the worker pool for the life of the test (real async path).
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go svc.Run(ctx)

	log := slog.New(slog.DiscardHandler)
	chBinder := approval.New(st, engine, nil, log)
	approver := accept.NewApprover(st, chBinder, clock)
	// engine (*channels.Engine) satisfies approval.Reconciler directly, so the shared
	// binder here reconciles through the SAME real engine the rig otherwise wires —
	// no drift between what the API's approve/create paths and the binder see.
	h := api.Router(log, api.Options{
		Store:    st,
		Auth:     httpcore.NewTokenAuthorizer(adminToken),
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
func (r *rig) awaitProposal(t *testing.T, jobID string) (string, proposals.Proposal) {
	t.Helper()
	deadline := time.Now().Add(workerWait)
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
				var prop proposals.Proposal
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
	return "", proposals.Proposal{}
}

// seedFillerClips lands a catalog of matched commercials + bumpers as a Tunarr sync
// would. Content hash is the catalog identity; Tunarr's program uuid is the playback
// handle for the same distinct clip.
func seedFillerClips(t *testing.T, st store.Store) {
	t.Helper()
	clips := []clips.Clip{
		{Hash: strings.Repeat("1", 64), TunarrProgramID: "bump-1", Name: "Bumper", Role: clips.Bumper, Era: 1992, Audience: clips.Kids, DurationMs: 5000, Source: "tunarr-local"},
		{Hash: strings.Repeat("2", 64), TunarrProgramID: "ad-1", Name: "Cereal ad", Role: clips.Commercial, Era: 1992, Audience: clips.Kids, Category: "cereal", DurationMs: 30000, Source: "tunarr-local"},
		{Hash: strings.Repeat("3", 64), TunarrProgramID: "ad-2", Name: "Toy ad", Role: clips.Commercial, Era: 1992, Audience: clips.Kids, Category: "toys", DurationMs: 30000, Source: "tunarr-local"},
		{Hash: strings.Repeat("4", 64), TunarrProgramID: "ad-3", Name: "Snack ad", Role: clips.Commercial, Era: 1992, Audience: clips.Kids, Category: "snacks", DurationMs: 30000, Source: "tunarr-local"},
		{Hash: strings.Repeat("5", 64), TunarrProgramID: "ad-adult", Name: "Late-night ad", Role: clips.Commercial, Era: 1992, Audience: clips.LateNight, Category: "alcohol", DurationMs: 30000, Source: "tunarr-local"},
	}
	for _, c := range clips {
		if err := st.UpsertClip(context.Background(), clipstore.Clip{Clip: c, UpdatedAt: time.Unix(1_800_000_000, 0)}); err != nil {
			t.Fatal(err)
		}
	}
}

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
	// The scripted LLM: turn 1 searches the catalog by term (grounding it to the real
	// library results — every kidsCartoons name holds "cartoon"); turn 2 returns picks for ALL
	// FOUR real ids it saw + a policy that a kids-cartoon intent implies (TV-Y7
	// ceiling). The model proposes the adult toon too — the ENFORCER must be what
	// drops it, not the model's discretion.
	llmMock := testkit.NewLLM(t,
		testkit.ToolCallResponse("catalog_search", map[string]any{
			"query": "cartoon",
			"dateMeaning": map[string]any{
				"kind":    "constraints",
				"anchors": []any{map[string]any{"field": "description", "start": 0, "end": 3}},
				"axes": []any{map[string]any{
					"kind": "movie_release", "combine": "any",
					"intervals": []any{map[string]any{"anchor": 0, "start": 1990, "end": 1999}},
				}},
			},
		}),
		testkit.FinalResponse(`{
			"rationale":"90s Saturday morning cartoons",
			"dateMeaning":{"kind":"constraints","anchors":[{"field":"description","start":0,"end":3}],"axes":[{"kind":"movie_release","combine":"any","intervals":[{"anchor":0,"start":1990,"end":1999}]}]},
			"picks":[
				{"mediaType":"movie","key":"movie:tmdb:90005001","name":"Sunny Cartoon Hour"},
				{"mediaType":"movie","key":"movie:tmdb:90005002","name":"Robo Rangers Cartoon"},
				{"mediaType":"movie","key":"movie:tmdb:90005003","name":"Critter Cartoon Club"},
				{"mediaType":"movie","key":"movie:tmdb:90005004","name":"Midnight Mayhem Cartoons"}
			],
			"policy":{"audience":{"ceiling":"TV-Y7"},"genres":{"include":["Animation"]},"ordering":"syndication"}
		}`),
	)

	r := newRig(t, llmMock)
	seedLibraryMovies(t, r.store, kidsCartoons...)
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
	if prop.Refused[0].Item.Name != "Midnight Mayhem Cartoons" || prop.Refused[0].Reason != "over_ceiling" {
		t.Errorf("refused = %+v, want Midnight Mayhem Cartoons / over_ceiling", prop.Refused[0])
	}
	// ⚠ Refused, NOT deleted. The operator's likely fix is to raise the ceiling, and a pick
	// that vanished between the model's answer and the approval card is indistinguishable from
	// one the model never made.
	for _, it := range prop.Lineup {
		if it.TMDBID == 90005004 {
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
	for _, key := range []string{"movie:tmdb:90005001", "movie:tmdb:90005002", "movie:tmdb:90005003"} {
		if rec, err := r.store.GetTitle(context.Background(), provision.Key(key)); err != nil || rec.State != "available" {
			t.Fatalf("approve should mark %s available, got %v/%v", key, rec.State, err)
		}
	}
	// ⚠ The REFUSED pick is not provisioned at all. Approval acts on what was offered, and
	// offering was the thing #259 fixed — provisioning a title the ceiling will drop spends an
	// acquisition (on the out-of-library path, a real download) for something that can never air.
	if _, err := r.store.GetTitle(context.Background(), provision.Key("movie:tmdb:90005004")); err == nil {
		t.Error("the refused TV-MA toon was provisioned — approval must act only on what it offered")
	}

	// 4. The same approval kicked a real reconcile through the real engine → Tunarr.
	// The grounded policy landed on the channel row (enforcement input).
	ch, _ := r.store.GetChannel(context.Background(), approved.ChannelID)
	if ch.Policy.Audience.Ceiling != "TV-Y7" {
		t.Fatalf("channel should carry the TV-Y7 policy, got %q", ch.Policy.Audience.Ceiling)
	}
	if ch.Policy.Filler == nil || ch.Policy.Filler.Audience != "kids" {
		t.Fatalf("approved kids intent did not seed a kids-safe filler selection: %+v", ch.Policy.Filler)
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
		t.Fatal("TV-MA 'Midnight Mayhem Cartoons' reached a TV-Y7 kids channel — audience enforcement was violated end-to-end")
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
		if id == "ad-adult" {
			t.Fatal("late-night commercial reached a TV-Y7 kids Channel filler list")
		}
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
