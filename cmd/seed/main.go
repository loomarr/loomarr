// Command seed populates a dev store with a believable slice of Loomarr state —
// an admin you can log in as, a member, titles across every provisioning stage, a
// live channel with a real desired lineup, and a handful of filler clips — so the
// UI can be exercised without a live media server / Tunarr / *arr stack.
//
// It exists to satisfy `make seed` (AGENTS.md harness contract). The one rule that
// shapes every line below: seed goes through the SAME domain paths the app does and
// NEVER writes raw rows that skip a gate. Concretely (AGENTS.md do-nots):
//
//   - `available` titles are produced ONLY by suggest.Approver — the single approval
//     gate. Seed builds a submitted proposal and approves it, exactly as the admin's
//     "approve" button does. It never UpsertTitle's an `available` record directly.
//   - the channel's desired lineup + any relaxation chips are the REAL output of
//     schedule.ComputeDesiredAt (the pure fn the reconciler runs), not hand-authored
//     literals — so what you see seeded is what the scheduler actually computes.
//   - the seeded member is passwordless (imported-style): the reworked identity model
//     (§11) has no "create a local member" path — members exist by import. A password
//     here would misrepresent the model, so the member logs in via the media server.
//
// Idempotent-ish: it's meant for a FRESH dev DB. Bootstrap closes after the first
// admin, so a second run against a populated DB reports "already bootstrapped" and
// stops rather than duplicating — delete the DB file to reseed.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"os"
	"time"

	"github.com/loomarr/loomarr/internal/auth"
	"github.com/loomarr/loomarr/internal/binder"
	"github.com/loomarr/loomarr/internal/config"
	"github.com/loomarr/loomarr/internal/demolibrary"
	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/fillerstore"
	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/suggest"
)

// Fixed credentials + seeds so a seeded dev install is predictable (and so the
// scheduler output is byte-stable run to run — determinism, §10).
const (
	adminUser = "admin"
	adminPass = "loomarr"
	chanSeed  = 42 // channel shuffle/ordering seed; any constant → reproducible slots
)

func main() {
	ctx := context.Background()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("seed: load config: %v", err)
	}
	if cfg.DatabaseURL == "" {
		// The app defaults to sqlite:///data/loomarr.db; for a local `make seed`
		// we point at a repo-local file so the dev backend reads the same store.
		cfg.DatabaseURL = "sqlite://./loomarr-dev.db"
		log.Printf("seed: DATABASE_URL unset — using %s", cfg.DatabaseURL)
	}

	st, err := fillerstore.Open(ctx, cfg.DatabaseURL, true) // autoMigrate: a fresh DB gets its schema
	if err != nil {
		log.Fatalf("seed: open store %q: %v", cfg.DatabaseURL, err)
	}
	defer func() { _ = st.Close() }()

	// SEED_LIBRARY_PICKS switches to real-library channels (library.go) on an existing store.
	if picks := os.Getenv("SEED_LIBRARY_PICKS"); picks != "" {
		if err := seedLibraryChannels(ctx, st, picks); err != nil {
			log.Fatalf("seed: library channels: %v", err)
		}
		fmt.Printf("\nseed complete: library channels from %s.\n", picks)
		return
	}

	// One shared user-id minter across admin + member: a per-call minter would reset
	// its counter and hand both users the same id (upsert-by-id → the member silently
	// overwrites the admin). Caught the first time seed actually ran.
	usrID := newID("usr")
	admin, err := seedAdmin(ctx, st, usrID)
	if err != nil {
		log.Fatalf("seed: admin: %v", err)
	}
	if err := seedMember(ctx, st, usrID); err != nil {
		log.Fatalf("seed: member: %v", err)
	}
	if err := seedTitlesAndChannel(ctx, st, admin.ID); err != nil {
		log.Fatalf("seed: titles/channel: %v", err)
	}
	if err := seedClips(ctx, st); err != nil {
		log.Fatalf("seed: clips: %v", err)
	}

	fmt.Printf("\nseed complete. Log in at the web UI as %q / %q.\n", adminUser, adminPass)
	fmt.Printf("store: %s\n", cfg.DatabaseURL)
}

// seedAdmin creates the first local admin through the sanctioned bootstrap path
// (Argon2id-hashed, Loomarr-owned id). If an admin already exists the bootstrap door
// is closed — we surface that and reuse it rather than forging a second one.
func seedAdmin(ctx context.Context, st store.Store, usrID func() string) (store.User, error) {
	// lib=nil: import is unavailable offline; seed only needs the bootstrap path.
	p := auth.NewProvisioner(st, nil, usrID, time.Now)
	admin, err := p.Bootstrap(ctx, adminUser, adminPass)
	if err == nil {
		log.Printf("seed: bootstrapped admin %q (id=%s)", admin.Name, admin.ID)
		return admin, nil
	}
	// Already seeded: bootstrap is once-only. Fetch the existing admin so the rest
	// of the seed (which needs an admin id for audit) still runs, and warn loudly.
	existing, gerr := st.GetUserByName(ctx, adminUser)
	if gerr != nil {
		return store.User{}, fmt.Errorf("bootstrap failed (%w) and no existing %q admin to reuse: %v", err, adminUser, gerr)
	}
	log.Printf("seed: admin already exists (%v) — reusing %q; delete the DB to reseed from scratch", err, adminUser)
	return existing, nil
}

// seedMember adds one non-admin user. The identity model (§11) has no local-member
// constructor — members are imported from the media server — so a seeded member is
// an imported-style row: RoleMember, no PasswordHash (can't log in locally), with a
// modest quota + auto-approve OFF so the Users page shows a realistic member.
func seedMember(ctx context.Context, st store.Store, usrID func() string) error {
	if _, err := st.GetUserByName(ctx, "casey"); err == nil {
		log.Printf("seed: member %q already present — skipping", "casey")
		return nil
	}
	now := time.Now()
	m := store.User{
		ID:        usrID(),
		Name:      "casey",
		Role:      store.RoleMember,
		Quota:     5, // pending-acquisition cap
		CreatedAt: now,
		UpdatedAt: now,
		// PasswordHash intentionally empty: imported-style, logs in via the media server.
	}
	if err := st.UpsertUser(ctx, m); err != nil {
		return err
	}
	log.Printf("seed: added member %q (imported-style, id=%s)", m.Name, m.ID)
	return nil
}

// seedTitlesAndChannel runs the real describe→approve→channel path offline:
//
//  1. record the suggest job + a submitted proposal (a lineup of in-library picks +
//     an acquisition list), exactly the shape the suggester emits;
//  2. suggest.Approver — THE gate — atomically turns in-library picks into
//     `available` records, acquisitions into `wanted` ones, flips the proposal to
//     approved, and creates its local channel;
//  3. walk two acquisitions forward through the pure provision.Apply state machine
//     (requested → downloading, and one all the way to available via a library
//     confirmation) so the Board shows titles in every stage — never by shortcutting
//     a `wanted` straight to `available`;
//  4. build a channel bound to that approved intent and compute its desired lineup
//     with schedule.ComputeDesiredAt (the reconciler's pure core) so programCount /
//     slotCount / policy.applied are genuinely what the scheduler produces.
func seedTitlesAndChannel(ctx context.Context, st store.Store, adminID string) error {
	now := time.Now()

	// A believable 90s movie-night lineup from the invented demo catalogue (the repo is
	// public: no real titles, #1587). In-library picks become available programs; the
	// acquisitions are missing titles the provisioner chases. Runtime rides on the local
	// `pick` (ProposalItem has no duration field) so the channel's slots carry real,
	// distinct durations. durMs is 0 for acquisitions (unknown until in-library).
	picks := []pick{
		libPick("demo-film-10", 8160000), // 136m
		libPick("demo-film-14", 8520000), // 142m
		libPick("demo-film-22", 6420000), // 107m
		libPick("demo-film-27", 7440000), // 124m
	}
	lineup := items(picks)
	acquisitions := []suggest.ProposalItem{
		acqItem("demo-film-08"),
		acqItem("demo-film-26"),
		acqItem("demo-film-29"),
	}

	jobID := newID("job")()
	intentJSON := `{"description":"90s movie night — mysteries, comedies and a little sci-fi"}`

	// The channel's grounded policy: a 90s era scope + explicit separation windows the
	// ~8.5h all-movie cycle can't fully honor, so the relaxation ladder descends a few
	// legible steps (no-repeat window → series gap → block) and records each on
	// policy.applied. Those chips are what the detail page's "To fill this channel,
	// Loomarr relaxed:" section renders. Whatever the ladder produces IS the seeded
	// value — it's the real computation (schedule.ComputeDesiredAt), not typed literals,
	// so if the ladder logic changes the seeded chips change with it. Windows are set
	// explicitly (not left to the 720h/168h built-in defaults) to keep the count small
	// and readable rather than the ladder emptying itself against an impossible default.
	policy := schedule.ChannelPolicy{
		ProposalPolicy: schedule.ProposalPolicy{
			Scope:    schedule.ScopePolicy{Dates: schedule.EraDates(schedule.Range{From: 1990, To: 1999})},
			Ordering: schedule.OrderSequential,
			Separation: schedule.SeparationPolicy{
				MovieNoRepeat:   schedule.Duration(15 * time.Hour),
				EpisodeNoRepeat: schedule.Duration(30 * time.Hour),
				SeriesMinGap:    schedule.Duration(24 * time.Hour),
				BlockMax:        8,
			},
		},
	}

	prop := suggest.Proposal{
		Intent:       suggest.Intent{Description: "90s movie night — mysteries, comedies and a little sci-fi", Era: "1990s"},
		Lineup:       lineup,
		Acquisitions: acquisitions,
		Policy:       policy,
	}
	propJSON, err := json.Marshal(prop)
	if err != nil {
		return fmt.Errorf("marshal proposal: %w", err)
	}
	propID := newID("prop")()
	// CreateSuggestionResult (built for #1720's no-generation proposals) writes the done
	// Job, its succeeded Attempt 1, and the submitted Proposal in one transaction — the
	// same shape the real suggester worker leaves behind. A raw CreateJob+CreateProposal
	// pair left no proposal_job_attempts row, so the workflow's own invariant check
	// rejected the seeded Job with ErrInvalidState and GET /v1/proposal-jobs 500'd (#1534).
	if err := st.CreateSuggestionResult(ctx, store.Job{
		ID:         jobID,
		Kind:       "suggest",
		Status:     "done",
		IntentJSON: intentJSON,
		IntentHash: "seed-90s-movie-night",
		CreatedBy:  adminID,
		CreatedAt:  now,
		UpdatedAt:  now,
	}, store.Proposal{
		ID:           propID,
		JobID:        jobID,
		Status:       "submitted",
		CreatedBy:    adminID,
		ProposalJSON: string(propJSON),
		CreatedAt:    now,
		UpdatedAt:    now,
	}); err != nil {
		return fmt.Errorf("create suggestion result: %w", err)
	}

	// Re-read the persisted proposal (Approve takes the stored row) and run the gate.
	stored, err := st.GetProposal(ctx, propID)
	if err != nil {
		return fmt.Errorf("get proposal: %w", err)
	}
	// nil edit: seed approves the proposal as generated. It goes through the real gate rather
	// than writing `available` rows directly — AGENTS.md's do-not list names that explicitly.
	channelPlanner := binder.New(st, nil, nil, slog.New(slog.DiscardHandler))
	approver := suggest.NewApprover(st, channelPlanner, time.Now)
	approved, err := approver.Approve(ctx, stored, nil, adminID)
	if err != nil {
		return fmt.Errorf("approve (the gate): %w", err)
	}
	log.Printf("seed: approved proposal %s — %d in-library picks → available, %d acquisitions → wanted", propID, len(lineup), approved.Enqueued)

	// Advance two of the wanted acquisitions through the real state machine so the
	// Board isn't all-or-nothing: one to `downloading`, one all the way to `available`
	// (as if the library confirmed it). This uses provision.Apply — the same pure
	// transition the reconciler/ingest use — never a raw state write.
	if err := advance(ctx, st, acquisitions[0], provision.Grabbed, ""); err != nil { // → downloading
		return err
	}
	if err := advance(ctx, st, acquisitions[1], provision.LibraryConfirmed, "demo-film-26"); err != nil { // → available
		return err
	}
	// acquisitions[2] stays `wanted` — a title still queued, so the Board's "Waiting"
	// stage is populated too.

	return seedChannel(ctx, st, approved.ChannelID, picks, policy)
}

// advance moves an acquisition (currently `wanted` from approval) forward by one
// domain event via provision.Apply, then persists it. Grabbed needs a fresh
// downloading deadline; LibraryConfirmed carries the library id. This is the honest
// way to reach a later state — it walks the machine rather than forging the target.
func advance(ctx context.Context, st store.Store, item suggest.ProposalItem, kind provision.EventKind, libID string) error {
	key, err := item.Key()
	if err != nil {
		return fmt.Errorf("key for %q: %w", item.Name, err)
	}
	rec, err := st.GetTitle(ctx, key)
	if err != nil {
		return fmt.Errorf("get title %q: %w", item.Name, err)
	}
	// A wanted acquisition must pass through `requested` before it can be grabbed
	// (the state machine ignores an out-of-order event), so accept the request first.
	now := time.Now()
	rec, _ = provision.Apply(rec, provision.Event{Kind: provision.RequestAccepted, Deadline: now.Add(24 * time.Hour)}, now)
	ev := provision.Event{Kind: kind, LibraryID: libID, Deadline: now.Add(6 * time.Hour)}
	rec, _ = provision.Apply(rec, ev, now)
	if err := st.UpsertTitle(ctx, rec); err != nil {
		return fmt.Errorf("upsert advanced title %q: %w", item.Name, err)
	}
	log.Printf("seed: advanced %q → %s", item.Name, rec.State)
	return nil
}

// seedChannel enriches the channel the approval gate already created with a computed
// desired schedule + applied relaxations, then persists it. The
// Desired slots reuse the approved titles' keys + library ids, so they are real
// SlotProgram entries (non-zero programCount) — not decorative placeholders.
func seedChannel(ctx context.Context, st store.Store, channelID string, picks []pick, policy schedule.ChannelPolicy) error {
	ch, err := st.GetChannel(ctx, channelID)
	if err != nil {
		return fmt.Errorf("get approved channel: %w", err)
	}
	dom := ch.Channel
	dom.Name = "90s Movie Night"
	dom.Number = 42
	dom.Group = "Loomarr"
	dom.Strategy = schedule.Sequential
	dom.Shuffle = schedule.ShuffleParams{Seed: chanSeed}
	// StatusBuilding, NOT StatusLive: seed has no Tunarr, so this channel was never
	// pushed and has no TunarrID. `live` is only reached BY the reconcile that assigns
	// a TunarrID — seeding `live` without one is an impossible state that renders as the
	// contradictory "On air" + "Nothing scheduled". `building` is the honest state:
	// created, lineup computed, awaiting its first push to Tunarr.
	dom.Status = schedule.StatusBuilding

	// Preserve the complete approved lineup, including acquisitions that are still pending.
	// Only enrich the in-library entries with the deterministic runtime/rating data this dev
	// fixture owns. Rebuilding from `picks` alone used to erase every wanted/downloading key,
	// so those titles could never backfill into the channel after landing.
	entries := append([]schedule.LineupEntry(nil), ch.Lineup...)
	entryByKey := make(map[provision.Key]int, len(entries))
	for i := range entries {
		entryByKey[entries[i].Key] = i
	}
	avail := seedAvailability{}
	for _, p := range picks {
		it := p.item
		key, err := it.Key()
		if err != nil {
			return fmt.Errorf("lineup key %q: %w", it.Name, err)
		}
		if i, ok := entryByKey[key]; ok {
			entries[i].Title = it.Name
			entries[i].DurationMs = p.durMs
			entries[i].Year = it.Year
			entries[i].OfficialRating = schedule.Rating(p.rating)
		}
		avail[key] = resolved{libID: it.LibraryItemID, dur: p.durMs, title: it.Name}
	}

	// The reconciler's pure core: filter → order → relaxation ladder → interleave.
	// now=time.Now() enables seasonality (no-op here); the seed drives determinism.
	desired := schedule.ComputeDesiredAt(dom, entries, avail, schedule.PodFill, policy, time.Now())

	ch.Channel = dom
	ch.Lineup = entries
	ch.Desired = desired.Slots
	ch.Policy = policy
	ch.Policy.Applied = desired.Applied // surface the relaxations the ladder actually applied
	if err := ch.Validate(); err != nil {
		return fmt.Errorf("validate channel: %w", err)
	}
	if _, err := st.SaveChannel(ctx, ch); err != nil {
		return fmt.Errorf("save channel: %w", err)
	}
	programs := 0
	for _, s := range desired.Slots {
		if s.IsProgram() {
			programs++
		}
	}
	log.Printf("seed: channel %q (#%d) — %d programs across %d slots, %d relaxation(s) applied",
		ch.Name, ch.Number, programs, len(desired.Slots), len(desired.Applied))
	return nil
}

// seedClips inserts a small filler catalog (commercials + a bumper), tagged so pod
// assembly can match them. Clips aren't gated — they're a parallel catalog, not
// titles — so a direct upsert is the correct construction.
func seedClips(ctx context.Context, st store.Store) error {
	type seededClip struct {
		clip filler.Clip
		tags []string
	}
	clips := []seededClip{
		// Invented products and brands (the repo is public, #1587).
		{clip("clip-starbit-crunch", "Starbit Crunch — Breakfast from Orbit!", filler.Commercial, 1992, filler.Kids, "cereal", "Starbit Foods", 30000), []string{"cereal"}},
		{clip("clip-blipotron", "Blip-o-Tron — Now You're Blipping", filler.Commercial, 1993, filler.Kids, "toys", "Blipco", 30000), []string{"toys", "kids-cue"}},
		{clip("clip-loopstep", "Loopstep — Go One More Block", filler.Commercial, 1994, filler.General, "apparel", "Loopstep", 30000), []string{"apparel"}},
		{clip("clip-bumper-nite", "You're watching 90s Movie Night", filler.Bumper, 0, filler.General, "", "", 8000), []string{"bumper"}},
	}
	now := time.Now()
	for _, seeded := range clips {
		if err := st.UpsertClip(ctx, store.Clip{Clip: seeded.clip, UpdatedAt: now}); err != nil {
			return fmt.Errorf("upsert clip %q: %w", seeded.clip.Name, err)
		}
		if err := st.SetClipTags(ctx, seeded.clip.Hash, seeded.tags); err != nil {
			return fmt.Errorf("tag clip %q: %w", seeded.clip.Name, err)
		}
	}
	log.Printf("seed: inserted %d filler clips", len(clips))
	return nil
}

// --- small helpers ---------------------------------------------------------------

// seedAvailability is an in-memory schedule.Availability: it answers "is this key
// available and what plays" from the seeded map. It satisfies the exact interface
// the reconciler consumes, so ComputeDesiredAt runs unchanged over seed data.
type resolved struct {
	libID string
	dur   int64
	title string
}
type seedAvailability map[provision.Key]resolved

func (a seedAvailability) Resolve(key provision.Key) (string, int64, bool) {
	r, ok := a[key]
	if !ok {
		return "", 0, false
	}
	return r.libID, r.dur, true
}

// No seeded series, so episode expansion never yields anything (movies only).
func (a seedAvailability) ResolveEpisodes(provision.Key) schedule.EpisodeResolution {
	return schedule.EpisodeResolution{}
}

// pick pairs an in-library ProposalItem with its runtime, which ProposalItem itself
// doesn't carry — the channel's slots need a real duration (Tunarr rejects <= 0).
type pick struct {
	item   suggest.ProposalItem
	durMs  int64
	rating string
}

// items projects the proposal-item view out of the picks (for the proposal body).
func items(ps []pick) []suggest.ProposalItem {
	out := make([]suggest.ProposalItem, len(ps))
	for i, p := range ps {
		out[i] = p.item
	}
	return out
}

// demoFilm is a film from the invented demo catalogue, keyed on its demo TMDB id. Its library
// item id is the stand-in media server's (`make demo-library`), so the two line up.
func demoFilm(id string) demolibrary.Title {
	t, ok := demolibrary.ByID(id)
	if !ok || t.Kind != demolibrary.Movie {
		panic("seed: unknown demo film " + id)
	}
	return t
}

func libPick(id string, durMs int64) pick {
	t := demoFilm(id)
	return pick{item: suggest.ProposalItem{
		MediaType: provision.Movie, TMDBID: t.ProviderID(), Name: t.Name, Year: t.Year,
		InLibrary: true, LibraryItemID: t.ID(),
	}, durMs: durMs, rating: t.Rating}
}

func acqItem(id string) suggest.ProposalItem {
	t := demoFilm(id)
	return suggest.ProposalItem{
		MediaType: provision.Movie, TMDBID: t.ProviderID(), Name: t.Name, Year: t.Year,
		InLibrary: false,
	}
}

func clip(id, name string, kind filler.Kind, era int, aud filler.Audience, cat, brand string, durMs int64) filler.Clip {
	hash := sha256.Sum256([]byte("loomarr-dev-seed:" + id))
	return filler.Clip{
		Hash: fmt.Sprintf("%x", hash), Path: "seed/" + id + ".mp4", TunarrProgramID: id,
		Name: name, Kind: kind, Era: era, Audience: aud, Category: cat, Brand: brand,
		DurationMs: durMs, Source: "seed",
	}
}

// newID returns a deterministic-ish id minter for a given prefix. Seeds run once on a
// fresh DB, so a simple counter is enough and keeps ids readable in the dev UI.
func newID(prefix string) func() string {
	n := 0
	return func() string {
		n++
		return fmt.Sprintf("%s_seed%d", prefix, n)
	}
}
