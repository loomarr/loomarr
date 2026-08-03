package filler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path/filepath"
	"time"
)

// This file is the catalog sync (§10, revised by §9.1): loomarr scans FILLER_DIR
// itself and probes each clip with ffprobe. Path + name + DURATION come from that
// scan; loomarr owns the match metadata (era/audience/category) too, and PRESERVES
// it across syncs so a re-sync never clobbers hand-edited or AI-assigned tags.
// `/v1/filler/sync` triggers it; a periodic sync runs alongside the reconciler
// (FILLER_SYNC_EVERY, §15).
//
// It previously synced FROM the Tunarr `local` filler source, with clip identity
// being the Tunarr program uuid. See Clip for why that could not serve internal
// playout (a uuid is not a playable input) and why the dependency ran the wrong
// way (no Tunarr ⇒ no catalog ⇒ no commercials, on a service §9.1 makes optional).

// RawClip is one clip as discovered by a scan, before catalog metadata is merged.
type RawClip struct {
	// ID is the identity: the clip's sparse content hash (§10 V38c).
	//
	// ⚠ Separate from Path since V38c. Identity used to BE the path, which broke the moment many
	// watched folders were allowed — two folders each holding `ads/coke.mp4` produced one id and
	// silently overwrote each other. Hashing the bytes also answers the question a path cannot:
	// *is this the same advert?*, which is what lets intake skip duplicates.
	ID string
	// Path is the clip's LOCATION under the clip folder — `a3/f9/<hash>.mp4` for anything intake
	// has filed. Data, not identity.
	Path string
	// TunarrProgramID is set only when the clip was ALSO seen through Tunarr's local
	// source, so Tunarr-backed channels can still build filler-lists. Empty on an
	// install with no Tunarr, which is a supported configuration, not a degraded one.
	TunarrProgramID string
	Name            string
	DurationMs      int64
	Kind            Kind
	Era             int // initial era from filename; 0 if none
	// Quality is the resolution label ("1080p", "480p") derived from the probed video
	// height; "" when the file has no video stream or was never re-probed. Display-only —
	// the guide's pod hover card shows it so a grainy 240p advert is explicable rather
	// than surprising. It never affects selection (V17c adds an opt-in floor).
	Quality string
	// License is the licence URL the source declared, read from the clip's info-JSON sidecar
	// at scan time (V33). "" means UNKNOWN — the common case, ~92% of archive.org items — and
	// never "public domain".
	License string
	// Thumbnail is the extracted frame's path relative to the thumbnail cache dir; "" when
	// extraction failed or has not run. Filled by a SEPARATE pass (GenerateThumbnails), not
	// by the probe — see thumbnail.go for why it cannot ride along with ffprobe.
	Thumbnail string
}

// FillerSource discovers the clips in FILLER_DIR.
//
// Implemented by DirSource (the local scan, §9.1) and satisfied in tests by a double. It was
// previously implemented by the Tunarr client — EnsureLocalSource registered the drop-folder
// as a Tunarr `local` media source and ListLocalClips read back what Tunarr had scanned.
//
// EnsureLocalSource is retained because Tunarr-backed channels still need that registration
// (their filler-lists reference Tunarr program ids). It is now BEST-EFFORT: an install with no
// Tunarr fails it harmlessly and still gets a full catalog from the local scan, which is the
// whole point of the change.
type FillerSource interface {
	EnsureLocalSource(ctx context.Context, dir string) error
	ListLocalClips(ctx context.Context) ([]RawClip, error)
}

// Store is the slice of the store the sync needs.
type Store interface {
	UpsertClip(ctx context.Context, c StoreClip) error
	GetClip(ctx context.Context, id string) (StoreClip, bool, error)
	DeleteClipsNotIn(ctx context.Context, keepIDs []string) (int, error)
}

// StoreClip is the persistence view the sync round-trips (mirrors store.Clip;
// declared here so filler doesn't import store — the adapter in main bridges them).
type StoreClip struct {
	Clip
	UpdatedAt time.Time
}

// wasFetchedByUs reports whether Loomarr DOWNLOADED this clip, rather than an operator putting it
// there (§10 V38c) — the held/filed fork.
//
// ⚠ **Reads a FIELD inside the sidecar, not the sidecar's existence** (changed from V38b). The old
// test was "does `<name>.info.json` exist", which worked only while Loomarr never wrote sidecars.
// V38c writes tags back for hand-dropped clips too, so existence now says nothing — every tagged
// clip would look downloaded, and a hand-dropped one would start being held for review.
//
// The field is also the better signal, not merely a repair: an operator who copies a clip TOGETHER
// with its sidecar gets the honest answer, and one who tidies sidecars away no longer flips a
// clip's lifecycle by accident. Explicit beats inferred.
//
// A missing drop-folder path answers false, which files the clip. That is the right failure: an
// install whose FILLER_DIR we cannot read should behave as it did before this phase rather than
// holding every clip it finds.
func (s *Syncer) wasFetchedByUs(clipPath string) bool {
	if s.dir == "" || clipPath == "" {
		return false
	}
	return SidecarFetchedByUs(filepath.Join(s.dir, clipPath))
}

// ErrSourceDisabled reports that the drop-folder is switched off on the Sources tab (§10 V35).
//
// ⚠ A distinct error rather than a silent zero result, and that is the difference between the
// switch working and the switch lying. A no-op success makes "Fetch now" a button that appears
// to run and changes nothing, which reads as a broken sync rather than a disabled source; the
// caller turns this into an answer that names the switch to flip.
var ErrSourceDisabled = errors.New("filler: the drop-folder source is switched off")

// Syncer reconciles the clip catalog against the Tunarr `local` filler source.
type Syncer struct {
	source FillerSource
	store  Store
	dir    string // FILLER_DIR (§15) — the drop-folder registered as a Tunarr local source
	log    *slog.Logger
	now    func() time.Time
	// enabled reports whether the drop-folder source is switched on (§10 V35). Read on every
	// sync rather than captured, because the setting hot-applies (config-design §3) — an
	// operator switching the folder off expects the NEXT scheduled pass to stop, not a restart
	// to be required. nil means always on, which keeps every existing construction unchanged.
	enabled func() bool
	// watch reports the configured `filler.watch_dir` (§10 V38c). Read live for the same reason
	// as `enabled` — it hot-applies. nil (or an empty answer) falls back to the derived default
	// under the clip folder, so a Syncer built without one still drains `<dir>/_watch`.
	watch func() string
	// scanSources lists the registered folders and libraries to drain (§10 V38c). nil ⇒ only the
	// configured clip folder is scanned, which is exactly the pre-V38c behaviour — so every
	// existing construction keeps working without opting in.
	scanSources ScanSourceStore
	// libraries copies clips out of a media-server library. nil ⇒ library rows do no work, which
	// is the honest state for an install with no media server configured.
	libraries *LibraryScanner
}

// watchDir resolves the watch folder for this pass.
func (s *Syncer) watchDir() string {
	configured := ""
	if s.watch != nil {
		configured = s.watch()
	}
	return WatchDir(s.dir, configured)
}

// drainScanSources reads every registered folder and library into the watch folder (§10 V38c),
// leaving the actual filing to the one intake that runs immediately afterwards.
//
// ⚠ **Every source is isolated from every other one.** A media server that is down, a folder whose
// mount has gone, a library name that no longer exists — each is logged against the row an
// operator can see, and the loop continues. Returning an error from here would mean one
// unreachable source costs a channel every commercial it already had on disk, which is the
// dependency §9.1 removed reappearing one level down.
//
// ⚠ Nothing is returned at all, deliberately. There is no aggregate outcome worth reporting: the
// clips that arrived are counted by the intake that follows, and a per-source failure belongs on
// that source's row rather than in a number the caller would have to interpret.
func (s *Syncer) drainScanSources(ctx context.Context) {
	if s.scanSources == nil {
		return // no per-source scanning wired (the single-folder path still runs)
	}
	srcs, err := s.scanSources.ListScanSources(ctx)
	if err != nil {
		if s.log != nil {
			s.log.Warn("filler: could not list scan sources; scanning the configured folder only",
				"err", err)
		}
		return
	}
	watch := s.watchDir()
	if watch == "" {
		return
	}

	for _, src := range srcs {
		select {
		case <-ctx.Done():
			return
		default:
		}
		switch src.Kind {
		case "folder":
			// ⚠ A registered folder is drained into the WATCH folder rather than scanned in
			// place, so its clips are hashed, deduplicated and filed exactly like everything
			// else. Scanning it in place would make it a second catalog location — the divergent
			// path §10 forbids — and two folders holding the same advert would be two clips.
			//
			// The configured clip folder is skipped: it is where clips already live, and draining
			// it into the watch folder would move the whole catalog back through intake.
			if src.URI == "" || src.URI == s.dir {
				continue
			}
			if res, err := TakeIn(src.URI, s.dir, false, s.logAttrs); err != nil {
				s.warnSource("filler: could not drain a watched folder", src, err)
			} else if s.log != nil && res.Taken > 0 {
				s.log.Info("filler: filed clips from a watched folder",
					"source", src.ID, "folder", src.URI, "taken", res.Taken)
			}
		case "library":
			if s.libraries == nil {
				continue // no media server configured; the row simply does no work
			}
			res, err := s.libraries.Scan(ctx, src.URI, watch)
			if err != nil {
				// ⚠ ErrLibraryUnreachableStorage is logged like any other failure HERE, but it
				// stays a distinct error so the Sources row can say "mount the volume" rather
				// than "this library is empty" — the two look identical to an operator and have
				// completely different remedies.
				s.warnSource("filler: could not scan a media-server library", src, err)
				continue
			}
			if s.log != nil && res.Copied > 0 {
				s.log.Info("filler: copied clips from a media-server library",
					"source", src.ID, "library", src.URI, "copied", res.Copied)
			}
		}
	}
}

// warnSource logs a per-source failure against the row an operator can see.
func (s *Syncer) warnSource(msg string, src ScanSource, err error) {
	if s.log != nil {
		s.log.Warn(msg, "source", src.ID, "kind", src.Kind, "uri", src.URI, "err", err)
	}
}

// logAttrs adapts the Syncer's logger to the plain function intake takes.
//
// ⚠ Intake deliberately does NOT take an *slog.Logger. It is filesystem plumbing with no other
// dependency, and a nil-able function keeps it callable from a test with one argument rather than
// a constructed logger.
func (s *Syncer) logAttrs(msg string, args ...any) {
	if s.log != nil {
		s.log.Warn(msg, args...)
	}
}

// NewSyncer builds a catalog syncer. dir is the drop-folder path (FILLER_DIR, §15)
// registered with Tunarr as a `local` media source.
func NewSyncer(source FillerSource, store Store, dir string, now func() time.Time, log *slog.Logger) *Syncer {
	if now == nil {
		now = time.Now
	}
	return &Syncer{source: source, store: store, dir: dir, log: log, now: now}
}

// WithEnabled gates the syncer on the drop-folder's on/off switch.
//
// ⚠ The gate lives HERE rather than at each caller, because there are three (the manual sync
// route, the Sources tab's "Fetch now", and the scheduled job) and a switch that only some of
// them honour is not a switch. A test asserts the disabled sync neither scans nor writes.
func (s *Syncer) WithEnabled(enabled func() bool) *Syncer {
	s.enabled = enabled
	return s
}

// WithWatchDir supplies the configured watch folder (§10 V38c), read live so the setting
// hot-applies. Without it the syncer drains the derived `<filler.dir>/_watch`.
func (s *Syncer) WithWatchDir(watch func() string) *Syncer {
	s.watch = watch
	return s
}

// WithScanSources gives the syncer the registered folders and libraries to drain (§10 V38c).
//
// ⚠ `libraries` may be nil on an install with no media server, and that is a supported
// configuration rather than a degraded one: folder rows still drain, and library rows simply do
// no work. This is the same rule EnsureLocalSource follows for Tunarr — an optional service is
// never allowed to become a precondition for having a catalog.
func (s *Syncer) WithScanSources(srcs ScanSourceStore, libraries *LibraryScanner) *Syncer {
	s.scanSources = srcs
	s.libraries = libraries
	return s
}

// SyncResult reports what a sync did (for the API + logs).
type SyncResult struct {
	Total   int // clips in the Tunarr local filler source
	Added   int // new clips
	Updated int // existing clips whose server-derived fields changed
	Pruned  int // clips removed (gone from the source)
}

// Sync ensures the Tunarr local source exists, then reconciles the catalog (§10):
//   - upsert every scanned clip, PRESERVING loomarr-owned tags (era/audience/
//     category/ai) on clips we already know — Tunarr only owns id/name/duration/
//     kind-hint;
//   - prune clips no longer in the source (identity = Tunarr program uuid).
//
// Duration always comes from Tunarr's scan. Idempotent: a no-change re-sync makes
// no tag edits.
func (s *Syncer) Sync(ctx context.Context) (SyncResult, error) {
	// Checked BEFORE the FILLER_DIR check and before any filesystem work: a switched-off
	// source must not scan, and must not report a configuration problem it does not have.
	if s.enabled != nil && !s.enabled() {
		return SyncResult{}, ErrSourceDisabled
	}
	if s.dir == "" {
		return SyncResult{}, fmt.Errorf("filler sync: no FILLER_DIR configured")
	}
	// Register the drop-folder with Tunarr, when there is one (idempotent, §10).
	//
	// BEST-EFFORT since §9.1, and the distinction is the whole point of that change: this
	// registration exists so TUNARR-backed channels can reference clips in a filler-list, and
	// it has nothing to do with discovering the files — Loomarr scans FILLER_DIR itself. An
	// install with no Tunarr, or one whose Tunarr is momentarily down, must still get a full
	// catalog; failing here would restore exactly the dependency §9.1 removed.
	if err := s.source.EnsureLocalSource(ctx, s.dir); err != nil && s.log != nil {
		s.log.Warn("filler: could not register the drop-folder with Tunarr; "+
			"scanning locally anyway (Tunarr-backed channels may lack filler until it returns)",
			"dir", s.dir, "err", err)
	}

	// Every registered folder and library drains into the watch folder FIRST, so the intake below
	// files what they brought in on this same pass (§10 V38c).
	s.drainScanSources(ctx)

	// ⚠ INTAKE RUNS BEFORE THE LISTING, and the order is the whole difference between "I dropped
	// a file in and it appeared" and "I dropped a file in and nothing happened". Draining after
	// the scan would file clips that this pass has already finished looking for, so every arrival
	// would wait a full sync interval — up to 15 minutes by default — to be catalogued.
	//
	// Failures are logged, not returned: a watch folder that cannot be drained (a permissions
	// problem, a full disk) must not take the catalog down with it. The clips already filed are
	// still there, and the arrivals stay put for the next pass.
	if taken, err := TakeIn(s.watchDir(), s.dir, false, s.logAttrs); err != nil {
		if s.log != nil {
			s.log.Warn("filler: could not drain the watch folder; scanning what is already filed",
				"watch", s.watchDir(), "err", err)
		}
	} else if s.log != nil && (taken.Taken > 0 || taken.Duplicates > 0) {
		s.log.Info("filler: filed new clips from the watch folder",
			"taken", taken.Taken, "duplicates", taken.Duplicates, "skipped", taken.Skipped)
	}

	raw, err := s.source.ListLocalClips(ctx)
	if err != nil {
		return SyncResult{}, fmt.Errorf("list filler clips: %w", err)
	}

	res := SyncResult{Total: len(raw)}
	keep := make([]string, 0, len(raw))
	for _, rc := range raw {
		// ⚠ IDENTITY IS rc.ID (the content hash), NOT rc.Path — since V38c. Keying on the path
		// here survived the re-key unnoticed because `DirSource` fills both and today they agree
		// for every clip under one folder. The moment two watched folders each hold `ads/coke.mp4`
		// the path stops being unique, one clip overwrites the other, and `keep` prunes a live
		// row — which is the exact failure V38c moved identity off the path to prevent.
		keep = append(keep, rc.ID)

		existing, found, err := s.store.GetClip(ctx, rc.ID)
		if err != nil {
			return res, fmt.Errorf("get clip %s: %w", rc.ID, err)
		}

		merged := StoreClip{UpdatedAt: s.now()}
		merged.Hash = rc.ID
		merged.Path = rc.Path
		// Scan-owned fields (always taken fresh — the filesystem is source of truth).
		merged.Name = rc.Name
		merged.DurationMs = rc.DurationMs
		merged.Kind = rc.Kind
		// Quality is scan-owned like duration: it is a property of the FILE, so a re-encode
		// that changes the resolution should be reflected, and there is no hand-edited value
		// to protect. (Contrast the match tags below, which a human or the AI may have set.)
		merged.Quality = rc.Quality
		// Thumbnail is scan-owned for the same reason: it is derived from the FILE, and the
		// generator adopts an existing image rather than re-extracting, so this is already
		// the previous value whenever one exists.
		merged.Thumbnail = rc.Thumbnail
		// Licence is scan-owned (it comes from the source's sidecar, never from a human), but
		// ⚠ a BLANK scan value must not erase a known one. The sidecar can go missing for
		// reasons that say nothing about the licence — an operator tidying `.info.json` files
		// out of the drop-folder, or a clip moved in by hand beside one that has one — and
		// "we stopped being able to see it" is not "it became unknown". Losing the record
		// silently is the failure worth preventing; a re-fetch restores it either way.
		merged.License = rc.License
		if merged.License == "" {
			merged.License = existing.License
		}
		// Carry the Tunarr uuid when the scan found one. Taken fresh rather than preserved:
		// a re-registered Tunarr local source mints new program ids, and a stale uuid would
		// build a filler-list referencing programs Tunarr no longer has.
		merged.TunarrProgramID = rc.TunarrProgramID
		if found {
			// PRESERVE loomarr-owned match tags across syncs (§10) — never clobber a
			// hand-edited or AI-assigned era/audience/category.
			merged.Era = existing.Era
			merged.Audience = existing.Audience
			merged.Category = existing.Category
			merged.AITagged = existing.AITagged
			merged.Rating = existing.Rating
			merged.Source = existing.Source
			// The era suggestion (V34) is loomarr-owned too — a scan knows nothing
			// about it, so it survives like the tags above.
			merged.SuggestedEra = existing.SuggestedEra
			// ⚠ The lifecycle fields (§10 V38) are loomarr-owned and a scan knows nothing about
			// any of them. `Held` is the sharpest: this scan sets it false for every file it
			// finds on disk, so without this line a single pass would FILE every held clip —
			// emptying the review queue into live channels with no operator action.
			//
			// Belt and braces with UpsertClip's ON CONFLICT list, which omits all three. Either
			// alone would do; the file's own comment about the play counters argues for both,
			// because a future edit to one that forgot the other fails silently.
			merged.Held = existing.Held
			merged.Confidence = existing.Confidence
			merged.AutoFiled = existing.AutoFiled
			// ⚠ Play counters are PRESERVED, not re-derived: a scan knows nothing about what
			// aired. Belt and braces with UpsertClip's ON CONFLICT list, which also omits them
			// — either one alone would be enough, but a future edit to either that forgot the
			// other would silently reset every counter, and "usage never goes up" is a bug
			// nobody reports.
			merged.PlayCount = existing.PlayCount
			merged.LastPlayedAt = existing.LastPlayedAt
			if existing.TunarrProgramID != "" && rc.TunarrProgramID == "" {
				// Keep a known uuid when THIS scan could not see Tunarr (it is offline, or
				// this install has none). Losing it would silently strip filler from a
				// Tunarr-backed channel on the next reconcile.
				merged.TunarrProgramID = existing.TunarrProgramID
			}
			if serverFieldsUnchanged(existing.Clip, merged.Clip) {
				continue // idempotent: nothing changed, skip the write
			}
			res.Updated++
		} else {
			// New clip: seed era from the filename hint; leave audience/category
			// untagged for AI/manual tagging.
			merged.Era = rc.Era
			merged.Source = "filler-dir"
			// ⚠ The lifecycle fork (§10 V38), and it is decided ONLY for a clip this scan has
			// never seen — an existing clip's `Held` is preserved above, so re-scanning can
			// never re-hold something a human already filed.
			//
			// Ingest downloads into this same folder, so at catalogue time a downloaded file and
			// a hand-copied one are both just files on disk. The sidecar's `fetchedBy` field is
			// what tells them apart: Loomarr FETCHED this ⇒ HOLD it for review; a person put it
			// here ⇒ file it on sight. Holding a hand-copied clip would mean a file you placed
			// yourself sits invisible until you approve it, which is the ceremony §7 warns
			// teaches people to click through gates.
			//
			// ⚠ V38c moved this from "a sidecar EXISTS" to the field. Existence stopped working
			// the moment Loomarr began writing tags back for hand-dropped clips too — every
			// tagged clip would have looked downloaded, and would have started being held.
			merged.Held = s.wasFetchedByUs(rc.Path)
			res.Added++
		}
		if err := s.store.UpsertClip(ctx, merged); err != nil {
			return res, fmt.Errorf("upsert clip %s: %w", rc.ID, err)
		}
	}

	pruned, err := s.store.DeleteClipsNotIn(ctx, keep)
	if err != nil {
		return res, fmt.Errorf("prune clips: %w", err)
	}
	res.Pruned = pruned
	if s.log != nil {
		s.log.Info("filler catalog synced", "total", res.Total, "added", res.Added, "updated", res.Updated, "pruned", res.Pruned)
	}
	return res, nil
}

// serverFieldsUnchanged reports whether the Tunarr-owned fields match (so a
// re-sync is a no-op write). Tags aren't compared — they're loomarr-owned and
// preserved, not synced.
func serverFieldsUnchanged(a, b Clip) bool {
	return a.Name == b.Name && a.DurationMs == b.DurationMs && a.Kind == b.Kind
}
