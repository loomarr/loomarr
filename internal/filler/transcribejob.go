package filler

import (
	"context"
	"io/fs"
	"log/slog"
	"path/filepath"
	"time"
)

// The on-demand transcribe job (§10 V44).
//
// ⚠ **Same shape and same arm64 reality as the language gate** (languagejob.go). Transcription is
// `MediaTools.Transcribe` — whisper local (~341s per clip under QEMU) or the §8.1 hosted audio
// model — so an inline pass would turn a 100-clip folder into a ~9.5-hour scan on arm64 and the
// sync timer would overlap itself. It runs AFTER the clip is catalogued, on a timer, in the
// background, off by default. This is the same reason the language job and the AI tagger are
// background jobs.
//
// It is SELECTIVE by design (§10 "on-demand transcription"): Whisper is expensive, so a clip whose
// source already describes it never pays for it. A clip is transcribed only when its source text is
// THIN (an empty or near-empty sidecar description — the archive.org common case) OR it is still
// untagged after a text-only pass. The point of persisting the transcript is that it becomes the
// richest input the tagger gets: a cereal advert with no description still SAYS "Kellogg's", which
// is the only place a brand or era can be grounded for such a clip.

// TranscribeStore is the narrow slice of the store this job needs — ListClips for the work list
// (the job filters "not yet transcribed" itself, `transcript = ”` not being a query the store
// should learn) and SetClipTranscript to record the result. Declared here, mirroring LanguageStore,
// so filler does not import store; the adapter in main bridges them.
type TranscribeStore interface {
	ListClips(ctx context.Context, f ClipQuery) ([]StoreClip, error)
	// SetClipTranscript records what the clip says. Called for EVERY outcome — spoken text, or the
	// TranscriptNone sentinel for a wordless clip — because an unrecorded answer is indistinguishable
	// from "not yet transcribed", so the job would re-run Whisper on the same silent clip forever
	// (~341s a time under QEMU). This is the same not-yet-vs-recorded distinction the language gate
	// turns on, and it is why the wordless case records a NON-EMPTY marker (see TranscriptNone).
	SetClipTranscript(ctx context.Context, path, transcript string, at time.Time) error
}

// TranscribeResult is what one pass did.
type TranscribeResult struct {
	Considered  int // clips that needed transcribing (thin source or still untagged)
	Transcribed int // answered with spoken text
	Wordless    int // ran and heard nothing — recorded the TranscriptNone sentinel so it is never re-visited
	Failed      int // could not run (whisper unrunnable, hosted key missing); left for the next pass
	SkippedRich int // had a rich source description and were already tagged — never paid for Whisper
}

// TranscribeJob transcribes thin-source clips that have not been transcribed yet.
type TranscribeJob struct {
	store TranscribeStore
	tools MediaTools
	// dir is FILLER_DIR; clip paths are relative to it (both for Transcribe's file arg and for
	// reading the sidecar that decides "thin").
	dir func() string
	// drop is FILLER_DIR as an fs.FS, used to read the info-JSON sidecar that decides whether a
	// clip's source text is thin. nil ⇒ every clip reads as thin (no sidecar text), which is the
	// safe polarity: a clip we cannot read a description for is exactly the one Whisper helps.
	drop fs.FS
	// enabled reports whether transcription is switched on. Read live on every pass so a settings
	// change applies on the next one (config-design §3 hot-apply). nil or false ⇒ Run is a no-op,
	// so an install that has not opted in pays nothing — the same gate the language job has.
	enabled func() bool
	now     func() time.Time
	log     *slog.Logger
	// max bounds one pass. Without it the first run on a fresh catalog would try every clip in one
	// go — on the local backend that is hours, holding a job worker the whole time.
	max int
}

// TranscribeBatch is how many clips one pass transcribes.
//
// Small on purpose, and SMALLER than LanguageBatch: a transcription reads the whole span rather
// than classifying a few seconds of phonetics, so each clip costs more. The job runs on a timer, so
// a large catalog drains over several passes rather than one long one — and a pass that cannot
// finish is worse than a pass that does less, because a cancelled context loses whatever it had not
// written.
const TranscribeBatch = 10

// TranscriptNone is the sentinel a wordless clip records (§10 V44) — the transcript column's
// analogue of the language gate's LangNone.
//
// ⚠ **A NON-EMPTY marker is load-bearing, and here is exactly why.** The transcript column has the
// same two meanings `language` does — "not yet transcribed" and "transcribed, wordless" — and the
// row default for BOTH is `”`. The language gate distinguishes them by writing a NON-EMPTY value
// (`none`) for the second, so a re-list can filter "already handled" as `!= ”`. A transcribe job
// that recorded a literal `”` for a silent clip could not: `”`-vs-`”` is not a distinction, so
// every pass would re-run Whisper on every wordless clip forever — ~341s each under QEMU, the exact
// cost the "record every outcome" discipline (§10) exists to avoid. This value is that distinction.
//
// It is deliberately a bracketed marker, not real speech, so a search over transcripts never
// surfaces it as content and the tagger (which reads `Transcript` as a text signal) sees a token
// it will not ground a brand or era on — a wordless clip must still be tagged from its OTHER
// signals (its filename, its sidecar, or the vision tier), never from this placeholder.
//
// ⚠ **A documented tension with 00038's column note**, surfaced rather than papered over: that
// migration says the row stores `”` for a wordless clip and "does not need to tell [the two
// meanings] apart", while clip.go says the job "distinguishes [them] the same way the language gate
// does". Both cannot hold — a plain `”` row is indistinguishable from not-yet, and re-visiting a
// silent clip forever violates the never-re-run rule the same migration states as the whole point.
// This resolves it toward the never-re-run guarantee (stated three times, it is the purpose) via the
// language gate's own mechanism. If the maintainer instead wants the row to carry a truly empty
// transcript, the store needs a separate "transcribed_at" marker for the job to read — a schema
// change outside this job's files.
const TranscriptNone = "[no speech]"

// ThinSourceRunes is the description length below which a clip's source text is "thin" and Whisper
// earns its cost (§10 V44).
//
// ~40 characters: an archive.org item that declares only "commercial" or a bare title says almost
// nothing the tagger can ground a brand or era on, while a two-sentence real description does. The
// threshold is where a source description stops being a signal and starts being a placeholder — an
// empty sidecar (the common drop-folder case) is trivially below it.
const ThinSourceRunes = 40

// NewTranscribeJob builds the job. A nil `tools`, a nil/false `enabled`, or an empty `dir` makes Run
// a no-op, so an install that has not opted in pays nothing. `drop` may be nil (every clip then
// reads as thin, which only ever transcribes MORE, never fewer — the safe direction).
func NewTranscribeJob(
	store TranscribeStore, tools MediaTools,
	dir func() string, drop fs.FS, enabled func() bool,
	now func() time.Time, log *slog.Logger,
) *TranscribeJob {
	return &TranscribeJob{
		store: store, tools: tools, dir: dir, drop: drop, enabled: enabled,
		now: now, log: log, max: TranscribeBatch,
	}
}

// Run transcribes up to one batch of eligible, not-yet-transcribed clips.
func (j *TranscribeJob) Run(ctx context.Context) (TranscribeResult, error) {
	var res TranscribeResult
	if j.store == nil || j.tools == nil {
		return res, nil
	}
	if j.enabled == nil || !j.enabled() {
		return res, nil // off — the opt-in gate, read live
	}
	dir := ""
	if j.dir != nil {
		dir = j.dir()
	}
	if dir == "" {
		return res, nil
	}

	// ⚠ IncludeHeld, like the language job: a clip awaiting review is a fine candidate. Knowing
	// what it SAYS before a human looks is strictly more useful than after — the transcript is the
	// text the reviewer (and the tagger) reasons about, so producing it early is pure upside.
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
		// ⚠ Already transcribed. `""` is the ONLY value that means "not yet" — a recorded empty
		// string is a FINAL answer (a wordless clip), and re-transcribing it would spend ~341s
		// under QEMU to learn the same nothing. This is the ""-vs-recorded distinction the language
		// gate turns on, applied to the transcript column.
		if clip.Transcript != "" {
			continue
		}
		// SELECTIVITY (§10 V44): a clip whose source already describes it never pays for Whisper.
		// Whisper earns its cost only when the source text is THIN or the clip is still untagged
		// after a text-only pass — a rich, already-tagged clip has nothing to gain and a real cost
		// to avoid.
		if !j.needsTranscription(clip) {
			res.SkippedRich++
			continue
		}
		res.Considered++

		start, end := LanguageSpan(clip.DurationMs)
		full := filepath.Join(dir, filepath.FromSlash(clip.Path))
		segs, tErr := j.tools.Transcribe(ctx, full, start, end)
		if tErr != nil {
			// ⚠ NOT recorded. A backend failure (whisper unrunnable, hosted key missing) says
			// nothing about the clip, so leaving `transcript` empty is what makes the next pass
			// retry it — the exact treatment the language job gives a detector failure.
			res.Failed++
			if j.log != nil {
				j.log.Warn("transcribe failed; will retry", "clip", clip.Path, "err", tErr)
			}
			continue
		}

		text := TranscriptText(segs)
		wordless := text == ""
		if wordless {
			// ⚠ A wordless clip records the NON-EMPTY sentinel, not a literal "". Whisper heard no
			// speech; recording `TranscriptNone` is what stops the selective job re-visiting a silent
			// clip forever — the exact reason the language gate records `none` rather than "". A
			// literal "" here would be indistinguishable from "not yet transcribed" and every pass
			// would re-run Whisper (~341s under QEMU) on the same silent clip. See TranscriptNone for
			// the full rationale and the documented tension with 00038. The tagger treats the marker
			// as no speech and tags the clip from its other signals (or the vision tier does, §10).
			text = TranscriptNone
		}
		if err := j.store.SetClipTranscript(ctx, clip.Path, text, j.now()); err != nil {
			res.Failed++
			if j.log != nil {
				j.log.Warn("could not record clip transcript", "clip", clip.Path, "err", err)
			}
			continue
		}
		if wordless {
			res.Wordless++
			continue
		}
		res.Transcribed++
	}
	// A run summary, like the AI-tagging job's — so an operator (and a live test) can see what a
	// pass did without reading per-clip warnings. Its absence made a "ran but transcribed nothing"
	// pass indistinguishable from one that never ran.
	if j.log != nil {
		j.log.Info("filler transcribe run",
			"considered", res.Considered, "transcribed", res.Transcribed,
			"wordless", res.Wordless, "skippedRich", res.SkippedRich, "failed", res.Failed)
	}
	return res, nil
}

// needsTranscription decides whether a clip earns Whisper (§10 V44 "selective by design"): its
// source text is THIN, OR it is still untagged after the text-only pass.
//
// ⚠ The two conditions are an OR, and both matter. A clip with a THIN description is a candidate
// even if it happens to be tagged — its transcript is still the only place a brand can be grounded.
// A clip with a RICH description that is nonetheless still UNTAGGED is also a candidate — the
// description did not carry what the tagger needed, so the spoken text is the next signal to try.
// Only a clip that is BOTH richly-described AND already tagged has nothing to gain, and that is the
// one this returns false for.
func (j *TranscribeJob) needsTranscription(clip StoreClip) bool {
	if j.sourceTextThin(clip) {
		return true
	}
	// Rich source, but did it actually produce tags? `Tagged()` is the same completeness test pod
	// matching uses (era + audience + category) — an untagged clip has an unfilled gap the
	// transcript may close.
	return !clip.Tagged()
}

// sourceTextThin reports whether a clip's source description is empty or near-empty (§10 V44).
//
// ⚠ Reads the sidecar via `SidecarText`, the SAME signal the tagger reads (tagjob.go), so "thin"
// here means exactly "thin to the tagger". A nil `drop` (no readable clip folder) yields "" and
// reads as thin — the safe direction, since a clip we cannot read a description for is precisely
// the one Whisper helps, and reading thin only ever transcribes MORE clips, never wrongly SKIPS one.
func (j *TranscribeJob) sourceTextThin(clip StoreClip) bool {
	if j.drop == nil || clip.Path == "" {
		return true
	}
	return len([]rune(SidecarText(j.drop, clip.Path))) < ThinSourceRunes
}
