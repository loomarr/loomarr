package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/store"
)

// GET /v1/filler/watch — the Filler header's live status (§10 V38c).
//
// ⚠ **The health VERDICT is computed here, on the server, and that is the point of the route.**
// The first implementation derived it in the browser from `/v1/filler/sources`, which was wrong
// twice over:
//
//  1. `/v1/filler/sources` is ADMIN-ONLY — it names filesystem paths and library targets. A
//     member's pill would have sat permanently grey ("nothing set up") on a working install,
//     which is the most alarming possible reading of a healthy system.
//  2. "every source is dark" and "nothing has arrived in days" are domain rules. In TypeScript
//     they could only be tested against a hand-built array of fake rows; here they are tested
//     against the store, and the same answer serves any future caller (a status page, a health
//     probe) without a second implementation to drift from this one.
//
// ⚠ It deliberately carries NO paths, targets or ids — only counts and a verdict. That is what
// makes it safe for a member to read: it explains what the channels are doing without disclosing
// the infrastructure behind them.

// FillerWatchHealth is the pill's three-state verdict.
//
// ⚠ Three states rather than a boolean, because "nothing is arriving" has two causes with
// OPPOSITE remedies: a fresh install needs setting up, while a configured one that has gone quiet
// needs attention. Collapsing them sends half the operators to the wrong place.
type FillerWatchHealth string

const (
	// FillerWatchHealthy — sources are on and the catalog has clips.
	FillerWatchHealthy FillerWatchHealth = "healthy"
	// FillerWatchAttention — configured, but every source is off, the catalog is empty, or
	// everything that reports a fetch time went quiet days ago.
	FillerWatchAttention FillerWatchHealth = "attention"
	// FillerWatchUnconfigured — nothing set up yet. NOT a fault: a fresh install with work still
	// to do, and saying "attention" here would read as a problem the operator caused.
	FillerWatchUnconfigured FillerWatchHealth = "unconfigured"
)

// fillerWatchStaleAfter is how long a switched-on source may go without bringing anything in
// before the verdict turns to `attention`.
//
// ⚠ Deliberately GENEROUS. Auto-fetch defaults to every 6 hours and a drop-folder may sit
// unchanged for weeks — an operator who has finished curating is not experiencing a fault. Three
// days is long enough that a quiet catalog reads as normal, short enough that a mount which
// silently failed is noticed before a channel runs dry.
const fillerWatchStaleAfter = 3 * 24 * time.Hour

type fillerWatchOutput struct {
	Body struct {
		Health FillerWatchHealth `json:"health" enum:"healthy,attention,unconfigured" doc:"Whether filler is working right now"`
		// SourcesOn / SourcesTotal drive the "N of M sources on" clause.
		SourcesOn    int `json:"sourcesOn"`
		SourcesTotal int `json:"sourcesTotal"`
		// Clips is the whole catalog, NOT a filtered view — the header must not change meaning
		// when someone types in the search box.
		//
		// ⚠ HELD clips are excluded, because they are not in the catalog yet — they are waiting
		// for a human in Incoming. `Held` below is what stops that reading as "nothing arrived".
		Clips int `json:"clips"`
		// Held is how many downloaded clips are waiting for review (§10 V38).
		//
		// ⚠ **Reported SEPARATELY, and the header says so.** Auto-fetch holds everything it
		// downloads, so a fresh install that just fetched 12 clips has a catalog of ZERO and 12
		// in Incoming. Reporting only `clips` made the pill read "0 clips" on an install that had
		// just worked perfectly — indistinguishable from a fetch that failed. Found by fetching
		// real collections and then asking why the Catalog tab was empty.
		Held int `json:"held" doc:"Downloaded clips awaiting review in Incoming; NOT included in clips"`
		// LastScanAt is the most recent fetch across every source; absent when nothing has ever
		// reported one.
		//
		// ⚠ Absent is the ORDINARY case, not a fault: most sources are SCANNED rather than
		// fetched, so the client omits the clause entirely rather than rendering "never" — which
		// would read as a failure on a drop-folder working exactly as intended.
		LastScanAt string `json:"lastScanAt,omitempty" doc:"RFC3339; absent if nothing has ever been fetched"`
		// AutoFetch names an intentional ceiling that currently stops unattended acquisition. It is
		// omitted when this runtime has no auto-fetcher rather than guessed from settings alone.
		AutoFetch *FillerFetchStatusDTO `json:"autoFetch,omitempty"`
	}
}

type FillerFetchStatusDTO struct {
	Enabled      bool   `json:"enabled"`
	StoppedBy    string `json:"stoppedBy,omitempty" enum:"catalog,disk"`
	CatalogClips int    `json:"catalogClips"`
	MaxCatalog   int    `json:"maxCatalog,omitempty"`
	DiskBytes    int64  `json:"diskBytes,omitempty"`
	MaxDiskBytes int64  `json:"maxDiskBytes,omitempty"`
}

type fillerFetchStatusService interface {
	FetchStatus(context.Context) (filler.FetchStatus, error)
}

func (s *Server) registerFillerWatch(api huma.API) {
	huma.Register(api, withRole(huma.Operation{
		OperationID: "filler-watch", Method: http.MethodGet, Path: "/v1/filler/watch",
		Summary: "Is filler working right now",
		Description: "The Filler page header's live status (§10 V38c): how many sources are on, how many clips " +
			"the catalog holds, when anything last arrived, and a health verdict. " +
			"⚠ The verdict is computed on the SERVER — the rule ('every source dark', 'nothing has arrived in " +
			"days') is domain logic, and deriving it in the client would also have left members with a " +
			"permanently grey indicator, since the sources listing is admin-only. " +
			"Read-only and member-visible: it names no paths, targets or ids — only counts and a verdict.",
		Tags: []string{"filler"},
	}, RoleMember), s.fillerWatch)
}

func (s *Server) fillerWatch(ctx context.Context, _ *struct{}) (*fillerWatchOutput, error) {
	out := &fillerWatchOutput{}
	if s.store == nil {
		return nil, huma.Error501NotImplemented("no store configured")
	}

	srcs, err := s.store.ListFillerSources(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("list filler sources", err)
	}
	// The whole catalog, unfiltered — the header must not change meaning when someone types in
	// the search box. ⚠ This EXCLUDES held clips by default (see ClipFilter), which is correct:
	// a held clip is not in the catalog, it is waiting for a human.
	//
	// ⚠ Counted in SQL. Both of these loaded whole rows to call len() on the slice — this
	// endpoint polls the Filler page header, so it paid for two full catalog reads on a timer.
	clipCount, err := s.store.CountClips(ctx, store.ClipFilter{})
	if err != nil {
		return nil, huma.Error500InternalServerError("count filler clips", err)
	}
	// ...so the queue is counted separately, or an install that just auto-fetched reads as "0
	// clips" while holding a dozen.
	heldCount, err := s.store.CountClips(ctx, store.ClipFilter{HeldOnly: true})
	if err != nil {
		return nil, huma.Error500InternalServerError("count held clips", err)
	}

	// ⚠ The applied drop-folder is a SOURCE even though its desired value comes from settings
	// (see fillersources.go). Counting only table rows would report "unconfigured" on the most
	// common install of all — a zero-env `docker run` whose `filler.dir` default is doing the work.
	// Read the generation snapshot, never the live desired value: status must describe where the
	// running scanner and intake pipeline are operating until restart applies a new layout.
	dir := s.fillerLayout.ClipDir()

	var total, on int
	var newest time.Time
	for _, src := range srcs {
		// A seeded-but-unconfigured row is not a source the operator has. It exists so the list
		// can say "you could set this up but have not" (§10), which is the opposite of a source
		// doing work, and counting it would make a fresh install look configured.
		// Read as: no target of its own, AND not the folder source that gets one from settings.
		if src.URI == "" && (src.Kind != "folder" || dir == "") {
			continue
		}
		total++
		if src.Enabled {
			on++
			if src.LastFetchedAt.After(newest) {
				newest = src.LastFetchedAt
			}
		}
	}

	out.Body.SourcesOn, out.Body.SourcesTotal = on, total
	out.Body.Clips, out.Body.Held = clipCount, heldCount
	if svc, ok := s.filler.(fillerFetchStatusService); ok {
		status, serr := svc.FetchStatus(ctx)
		if serr != nil {
			if s.log != nil {
				s.log.Warn("read filler auto-fetch status", "err", serr)
			}
		} else {
			out.Body.AutoFetch = &FillerFetchStatusDTO{
				Enabled: status.Enabled, StoppedBy: status.StoppedBy,
				CatalogClips: status.CatalogClips, MaxCatalog: status.MaxCatalog,
				DiskBytes: status.DiskBytes, MaxDiskBytes: status.MaxDiskBytes,
			}
		}
	}
	if !newest.IsZero() {
		out.Body.LastScanAt = newest.UTC().Format(time.RFC3339)
	}
	// ⚠ `time.Now()` here, but the RULE takes an explicit `now` so the staleness cases are
	// testable without a clock seam on Server (which nothing else needs).
	//
	// ⚠ Clips AND held are both handed to the verdict: an install that has fetched a dozen clips
	// and is holding them for review is WORKING, not broken. Passing only the catalog count made
	// a successful first fetch report `attention` — the fetcher's own success looking like a
	// failure.
	out.Body.Health = fillerWatchVerdict(total, on, clipCount+heldCount, newest, time.Now())
	return out, nil
}

// fillerWatchVerdict is the health rule, separated from the request so it can be tested directly
// across the clock cases without constructing a server.
//
// ⚠ **`clips` is what separates "quiet" from "broken".** An install holding clips WORKS even if
// nothing has arrived lately — the operator finished curating. Sources on and NOTHING at all is
// the state worth flagging, and flagging it on day one beats someone noticing a channel playing
// silence a week later.
//
// ⚠ `clips` here means **catalogued PLUS held**, not the catalog alone. Auto-fetch holds
// everything it downloads, so a first fetch leaves the catalog at zero with a full review queue —
// counting only the catalog reported `attention` at the exact moment the fetcher had just
// succeeded.
func fillerWatchVerdict(total, on, haveClips int, newest, now time.Time) FillerWatchHealth {
	if total == 0 {
		return FillerWatchUnconfigured
	}
	if on == 0 {
		return FillerWatchAttention
	}
	if haveClips == 0 {
		return FillerWatchAttention
	}
	// ⚠ Only when something actually REPORTED a fetch. A zero timestamp is the ordinary state for
	// a scanned folder, and treating it as stale would light every drop-folder install amber
	// forever — the exact false alarm that teaches an operator to ignore the indicator.
	if !newest.IsZero() && now.Sub(newest) > fillerWatchStaleAfter {
		return FillerWatchAttention
	}
	return FillerWatchHealthy
}
