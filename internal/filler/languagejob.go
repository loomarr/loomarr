package filler

import (
	"context"
	"log/slog"
	"path/filepath"
	"time"
)

// The language gate's background job (§10 V40).
//
// ⚠ **It runs AFTER the clip is already catalogued, not during the scan.** On the local backend a
// clip costs ~3s natively and ~341s under QEMU, so an inline pass would turn a 100-clip folder
// into a ~9.5-hour scan on arm64 and the sync timer would overlap itself. This is the same shape
// the AI tagger has, for the same reason.
//
// The consequence, stated because it is a real window rather than a technicality: a foreign clip
// CAN air once before the job reaches it. That is the accepted cost of not blocking the scan, and
// it is why the gate is a background reject rather than a scan-time one.

// LanguageStore is the narrow slice of the store this job needs.
type LanguageStore interface {
	// ListClips is the work list. The job filters to "not yet checked" itself rather than asking
	// for a dedicated query, because `language = ''` is not a concept the store should learn.
	ListClips(ctx context.Context, f ClipQuery) ([]StoreClip, error)
	// SetClipLanguage records what was heard. Called for EVERY outcome including `none` and a
	// rejection — an unrecorded answer is indistinguishable from "not yet checked", so the job
	// would re-run whisper on the same clip forever (~341s a time under QEMU).
	SetClipLanguage(ctx context.Context, path, language string, at time.Time) error
	// SetClipsRemoved tombstones a rejected clip. ⚠ A TOMBSTONE, not a delete: `clips` is a
	// synced cache, so a hard delete would be undone by the next scan finding the file still on
	// disk, and the clip would air again until the job reached it once more.
	SetClipsRemoved(ctx context.Context, paths []string, at time.Time) (int, error)
}

// ClipQuery is the job's view of a clip filter. Deliberately tiny — the job wants "everything
// currently in the catalog" and does its own filtering.
type ClipQuery struct {
	// IncludeHeld covers clips still awaiting review. A held clip is a fine candidate: knowing
	// its language BEFORE a human looks is strictly more useful than after.
	IncludeHeld bool
}

// LanguageResult is what one pass did.
type LanguageResult struct {
	Considered int // clips that needed checking
	Detected   int // answered with a language
	Silent     int // answered `none` — no speech to judge
	Rejected   int // confidently not the wanted language, tombstoned
	Failed     int // could not tell; left for the next pass
}

// LanguageJob checks clips that have not been checked yet.
type LanguageJob struct {
	store    LanguageStore
	detector LanguageDetector
	// dir is FILLER_DIR; clip paths are relative to it.
	dir func() string
	// want is the language filler is expected to be in, read live so a settings change applies
	// on the next pass (config-design §3 hot-apply). Empty ⇒ the gate is off.
	want func() string
	now  func() time.Time
	log  *slog.Logger
	// max bounds one pass. Without it the first run on a fresh catalog would try every clip in
	// one go — on the local backend that is hours, holding a job worker the whole time.
	max int
}

// LanguageBatch is how many clips one pass checks.
//
// Small on purpose. The job runs on a timer, so a large catalog drains over several passes rather
// than one long one — and a pass that cannot finish is worse than a pass that does less, because a
// cancelled context loses whatever it had not written.
const LanguageBatch = 25

// NewLanguageJob builds the job. A nil detector or an empty `want` makes Run a no-op, so an
// install that has not opted in pays nothing.
func NewLanguageJob(
	store LanguageStore, detector LanguageDetector,
	dir, want func() string, now func() time.Time, log *slog.Logger,
) *LanguageJob {
	return &LanguageJob{
		store: store, detector: detector, dir: dir, want: want,
		now: now, log: log, max: LanguageBatch,
	}
}

// Run checks up to one batch of unchecked clips.
func (j *LanguageJob) Run(ctx context.Context) (LanguageResult, error) {
	var res LanguageResult
	if j.store == nil || j.detector == nil {
		return res, nil
	}
	want := ""
	if j.want != nil {
		want = j.want()
	}
	if want == "" {
		return res, nil // gate off
	}
	dir := ""
	if j.dir != nil {
		dir = j.dir()
	}
	if dir == "" {
		return res, nil
	}

	// ⚠ IncludeHeld: a clip awaiting review is a fine candidate. Knowing its language before a
	// human looks is strictly more useful than after — and a held foreign clip should never
	// reach the point where someone files it by hand.
	clips, err := j.store.ListClips(ctx, ClipQuery{IncludeHeld: true})
	if err != nil {
		return res, err
	}

	for _, clip := range clips {
		if res.Considered >= j.max {
			break
		}
		select {
		case <-ctx.Done():
			return res, ctx.Err()
		default:
		}
		// ⚠ Already checked. `""` is the ONLY value that means "not yet" — `none` is a final
		// answer, and re-checking it would spend ~341s under QEMU to learn the same thing.
		if clip.Language != "" {
			continue
		}
		res.Considered++

		start, end := LanguageSpan(clip.DurationMs)
		full := filepath.Join(dir, filepath.FromSlash(clip.Path))
		detected, detErr := j.detector.DetectLanguage(ctx, full, start, end)
		if detErr != nil {
			// ⚠ NOT recorded and NOT rejected. A backend failure says nothing about the clip, so
			// leaving `language` empty is what makes the next pass retry it.
			res.Failed++
			if j.log != nil {
				j.log.Warn("language detect failed; will retry", "clip", clip.Path, "err", detErr)
			}
			continue
		}
		if detected == LangUndetermined {
			// The backend ran and could not tell — same treatment, same reason.
			res.Failed++
			continue
		}

		// Record BEFORE acting. If the tombstone below fails, the answer is still stored and the
		// next pass will not re-detect; the clip simply stays until something removes it.
		if err := j.store.SetClipLanguage(ctx, clip.Path, detected, j.now()); err != nil {
			res.Failed++
			if j.log != nil {
				j.log.Warn("could not record clip language", "clip", clip.Path, "err", err)
			}
			continue
		}
		if detected == LangNone {
			res.Silent++
			continue
		}
		res.Detected++

		if !LanguageRejects(detected, want) {
			continue
		}
		if _, err := j.store.SetClipsRemoved(ctx, []string{clip.Path}, j.now()); err != nil {
			if j.log != nil {
				j.log.Warn("could not remove a foreign clip", "clip", clip.Path, "err", err)
			}
			continue
		}
		res.Rejected++
		// ⚠ Logged per clip, not just counted. This is the one gate that deletes something an
		// operator may have put there by hand, and "where did my Spanish advert go" needs an
		// answer that does not require reading the code.
		if j.log != nil {
			j.log.Info("removed a clip whose speech is not the expected language",
				"clip", clip.Path, "detected", detected, "expected", want)
		}
	}
	return res, nil
}
