package filler

import (
	"context"
	"encoding/json"
	"log/slog"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mantonx/loomarr/internal/llm"
	"github.com/mantonx/loomarr/internal/taxonomy"
)

// The vision-tagging job (§10 V44) — the EXPENSIVE LAST TIER.
//
// ⚠ **Same shape and same arm64 reality as the language gate and the transcribe job.** A vision
// pass extracts keyframes (ffmpeg) and asks a multimodal model about them — the local Ollama path
// decodes an image model, the hosted path is a per-clip API call — so an inline pass would stall
// the scan the same way Whisper does, and every existing job runs AFTER the clip is catalogued, on
// a timer, in the background, off by default. This one is off unless a VisionProvider is configured.
//
// It is the LAST tier on purpose. The cheaper text tiers run first: filename/sidecar tagging, then
// on-demand transcription (transcribejob.go). Vision only earns its cost where those left a gap —
// a clip a vision pass has NOT already read (VisionTagged false) that is STILL short on tags. The
// spot this tier exists for is the WORDLESS one: a silent visual advert whose only signal is an
// on-screen logo or a "1987" burned into the corner — no dialogue for Whisper, often no sidecar
// text either, so the frame is the only place its brand or era can be grounded.
//
// GROUNDING IS THE ERA RULE, APPLIED TO PIXELS (§8, §10). The model returns {brand,category,era,
// visibleText}; `visibleText` is the on-screen text it says it can SEE, and it is the ONLY thing a
// brand/era/category is grounded against. A brand the frame does not spell out is dropped, exactly
// as validateTags drops an era whose year is absent from the text — the model reading "KELLOGG'S"
// off a box grounds the brand, the model *feeling* the ad is for Coke does not. Ungrounded ⇒
// dropped, never persisted. This mirrors validateTags rather than weakening it.

// VisionStore is the narrow slice of the store this job needs — ListClips for the work list (the
// job filters candidates itself) and SetClipVisionTags to record a grounded result and stamp
// vision_tagged. Declared here, mirroring LanguageStore/TranscribeStore, so filler does not import
// store; the adapter in main bridges them.
type VisionStore interface {
	ListClips(ctx context.Context, f ClipQuery) ([]StoreClip, error)
	// ListTaxa is the taxonomy vocabulary (§10 V45a): the vision tier grounds its category against
	// the graph (resolve-or-drop) instead of the deleted hardcoded knownCategories map — one
	// vocabulary for text and vision alike. Loaded once per run (the reindex pattern).
	ListTaxa(ctx context.Context) ([]taxonomy.Taxon, error)
	// SetClipVisionTags records what the vision pass GROUNDED — the on-screen text it read, plus a
	// brand/era/category only where visibleText supported them — and stamps vision_tagged. It is the
	// single writer of visible_text/vision_tagged on the branch (it SHARES `brand` with the text
	// tagger's SetClipBrand). ⚠ Called for EVERY clip a pass looked at, INCLUDING one that grounded
	// nothing: the stamp is what says "vision already read this", so an unstamped clip is
	// indistinguishable from "not yet looked at" and a re-run would pay for the same frames again —
	// the same not-yet-vs-recorded discipline the language gate and transcribe job turn on, here paid
	// for by a multimodal call rather than Whisper.
	SetClipVisionTags(ctx context.Context, path, brand, visibleText string, era, suggestedEra int, category string, at time.Time) error
}

// VisionResult is what one pass did.
type VisionResult struct {
	Considered int // candidate clips a vision pass looked at
	Grounded   int // clips where the frame supported at least one tag (brand/era/category)
	ReadOnly   int // clips the model returned visibleText for but nothing grounded — still stamped
	Failed     int // keyframes/model unrunnable; NOT stamped, left for the next pass
}

// VisionJob reads keyframes for candidate clips and grounds vision tags off them.
type VisionJob struct {
	store    VisionStore
	tools    MediaTools
	provider llm.VisionProvider
	// dir is FILLER_DIR; clip paths are relative to it (for Keyframes' file arg).
	dir func() string
	// enabled reports whether vision tagging is switched on. Read live on every pass so a settings
	// change applies on the next one (config-design §3 hot-apply). nil or false ⇒ Run is a no-op, so
	// an install that has not opted in pays nothing — the same opt-in gate the transcribe job has.
	enabled func() bool
	now     func() time.Time
	log     *slog.Logger
	// max bounds one pass. Without it the first run on a fresh catalog would ask the model about
	// every candidate clip in one go — on the hosted path that is a bill, on the local path hours of
	// image decoding, holding a job worker the whole time.
	max int
}

// VisionBatch is how many clips one pass reads.
//
// Small on purpose, and SMALLER than TranscribeBatch (10): a vision call extracts keyframes AND runs
// a multimodal model — the most expensive tier per clip, and on the hosted path it is a per-clip API
// charge. The job runs on a timer, so a large catalog drains over several passes rather than one long
// (or costly) one — and a pass that cannot finish is worse than a pass that does less, because a
// cancelled context loses whatever it had not written.
const VisionBatch = 5

// VisionKeyframes is how many stills one pass samples per clip (§10 V44). A commercial's brand card,
// its B&W transfer, and its end slate can each fall in a different part of the runtime, so one frame
// is not enough signal — but a fistful of 320px JPEGs is tens of KB, which is what the inline
// multimodal request is sized for. Three is the spread the keyframe sampler (mediatools.go) tolerates
// on even a short clip.
const VisionKeyframes = 3

// NewVisionJob builds the job. A nil `provider`, a nil `tools`, a nil/false `enabled`, or an empty
// `dir` makes Run a no-op, so an install with no vision model (the common case) pays nothing — the
// provider is nil unless the configured LLM implements llm.VisionProvider, which the caller
// type-asserts exactly as it does for Warmer.
func NewVisionJob(
	store VisionStore, tools MediaTools, provider llm.VisionProvider,
	dir func() string, enabled func() bool,
	now func() time.Time, log *slog.Logger,
) *VisionJob {
	return &VisionJob{
		store: store, tools: tools, provider: provider, dir: dir, enabled: enabled,
		now: now, log: log, max: VisionBatch,
	}
}

// Run reads up to one batch of candidate, not-yet-vision-tagged clips.
func (j *VisionJob) Run(ctx context.Context) (VisionResult, error) {
	var res VisionResult
	// A nil provider is the un-opted-in default: no vision model, no work. Checked alongside the
	// store/tools so an install without one pays nothing and never touches ffmpeg.
	if j.store == nil || j.tools == nil || j.provider == nil {
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

	// Load the taxonomy ONCE per run (§10 V45a) — the vocabulary the vision tier grounds its category
	// against, the same forest the text tagger uses. A nil/empty forest grounds no category (the safe
	// direction), matching every other grounding gate here.
	taxa, err := j.store.ListTaxa(ctx)
	if err != nil {
		return res, err
	}
	forest := taxonomy.New(taxa)

	// ⚠ IncludeHeld, like every other clip job: a clip awaiting review is a fine candidate. Reading
	// its logo before a human looks is strictly more useful than after — the reviewer files it from
	// exactly the tags this pass grounds.
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
		if !j.isCandidate(clip) {
			continue
		}
		res.Considered++

		full := filepath.Join(dir, filepath.FromSlash(clip.Path))
		frames, kErr := j.tools.Keyframes(ctx, full, VisionKeyframes)
		if kErr != nil || len(frames) == 0 {
			// ⚠ NOT stamped. A keyframe-extraction failure (or a clip with no video stream) says
			// nothing about what the frames hold, so leaving vision_tagged false is what makes the
			// next pass retry it — the exact treatment the transcribe job gives a Whisper failure.
			res.Failed++
			if j.log != nil && kErr != nil {
				j.log.Warn("keyframe extract failed; will retry", "clip", clip.Path, "err", kErr)
			}
			continue
		}
		resp, aErr := j.provider.AskAboutImages(ctx, visionPrompt, frames)
		if aErr != nil {
			// Same treatment: a model/backend failure is a retry, not a fact about the clip.
			res.Failed++
			if j.log != nil {
				j.log.Warn("vision model failed; will retry", "clip", clip.Path, "err", aErr)
			}
			continue
		}
		var out visionOutput
		// ⚠ Unwrap a code fence before parsing (§10 V44, live-found). A vision model — llava:7b in
		// the dev test — wraps its JSON in ```json … ```, and json.Unmarshal rejects the leading
		// backtick. llm.ExtractJSONObject returns the balanced {...} span, so a fenced-but-valid
		// answer grounds instead of being thrown away as "not JSON". Genuine prose (no object at
		// all) still falls through to the retry branch below, which is correct.
		if err := json.Unmarshal([]byte(llm.ExtractJSONObject(resp.Content)), &out); err != nil {
			// The model answered but not in JSON. Same as a backend failure — we learned nothing we
			// can trust, so leave it un-stamped for the next pass rather than persisting garbage.
			res.Failed++
			if j.log != nil {
				j.log.Warn("vision output not JSON; will retry", "clip", clip.Path, "err", err)
			}
			continue
		}

		// GROUND against the on-screen text the model says it read — the pixel analogue of
		// validateTags grounding a text tag against the clip's text signals.
		v := groundVisionTags(out, forest)

		// The free frame-heuristic tier (§10 V44): if the model grounded NO era, read one off the
		// pixels we already decoded — a monochrome 4:3 transfer is a "pre-1970s?" hint no text or
		// model-read year could supply. It is only ever a SUGGESTION (SetClipVisionTags writes it to
		// suggested_era, and only when there is no grounded era and no existing suggestion), so the
		// worst case is a one-click-dismissible prompt on a clip that had no era signal at all —
		// never a fact. A grounded era makes this a no-op.
		suggestedEra := 0
		if v.Era == 0 {
			suggestedEra = SuggestedEraFrom(AnalyzeFrames(frames))
		}

		// Record BEFORE deciding "grounded vs read-only": both outcomes STAMP the clip so a re-run
		// does not pay for these frames again. The store method writes brand/visible_text
		// unconditionally and era/category only when > 0 / non-empty, so a pass that grounded a brand
		// but no era leaves an existing text-tagged era untouched (the CASE guard).
		if err := j.store.SetClipVisionTags(ctx, clip.Path, v.Brand, v.VisibleText, v.Era, suggestedEra, v.Category, j.now()); err != nil {
			res.Failed++
			if j.log != nil {
				j.log.Warn("could not record vision tags", "clip", clip.Path, "err", err)
			}
			continue
		}
		if v.Brand != "" || v.Era > 0 || v.Category != "" {
			res.Grounded++
		} else {
			// The model read text off the frame (or none) but nothing grounded — still stamped, so the
			// clip is not looked at again. This is the vision analogue of the transcribe job's wordless
			// sentinel: an outcome recorded precisely so it is never re-paid-for.
			res.ReadOnly++
		}
	}
	return res, nil
}

// isCandidate reports whether a clip earns the expensive vision tier (§10 V44).
//
// ⚠ TWO conditions, both required. It must NOT have been vision-read already (VisionTagged false) —
// the stamp is what stops a re-run paying for the same frames — AND it must still be short on tags
// after the cheaper text/transcribe tiers (`!Tagged()`). A clip the earlier tiers already resolved
// has nothing for vision to add and a real cost to avoid, so vision is skipped. The wordless spot
// this tier exists for satisfies both by construction: no dialogue and no sidecar means the text
// tiers left it untagged, and no vision pass has looked yet.
func (j *VisionJob) isCandidate(clip StoreClip) bool {
	if clip.VisionTagged {
		return false
	}
	return !clip.Tagged()
}

// visionTags is a grounded vision classification for one clip — the fields SetClipVisionTags writes.
type visionTags struct {
	Brand       string
	Category    string
	Era         int
	VisibleText string
}

// visionOutput is the model's raw multimodal answer (untrusted — grounded before use).
type visionOutput struct {
	// VisibleText is the on-screen text the model says it can literally SEE — a logo, a product name,
	// a year burned into the frame. It is BOTH persisted (the auditable record of what vision read)
	// AND the grounding signal every other field here is checked against.
	VisibleText string `json:"visibleText"`
	Brand       string `json:"brand"`
	Category    string `json:"category"`
	Era         int    `json:"era"`
}

// groundVisionTags is the grounding pass for the vision tier — validateTags, but the signal a tag
// must appear in is the model's OWN visibleText rather than the clip's text signals (§10 V44).
//
// ⚠ **This is the era rule generalised to pixels, and it must not be weakened.** A vision model that
// reports {brand:"Coca-Cola"} with a visibleText that never spells "Coca-Cola" has INFERRED the brand
// from the imagery — the exact confidently-wrong metadata §8 keeps out — so the brand is dropped. A
// brand/category/era survives ONLY when it appears literally in the visibleText the model claims to
// have read. The asymmetry is safe the same way it is for text: dropping a true positive leaves the
// clip untagged for a later pass or a human, while accepting a fabrication corrupts matching silently.
//
// VisibleText itself is NOT grounded against anything — it IS the ground truth here, the record of
// what the pass read off the frame, kept verbatim so a reviewer can see WHY a tag was (or was not)
// grounded. Category is still enum-validated on top of grounding (an out-of-set category is dropped
// exactly as validateTags drops one), and era must be both a plausible year AND present in the text.
func groundVisionTags(out visionOutput, forest *taxonomy.Forest) visionTags {
	var v visionTags
	// The visibleText is the record of what vision saw — kept verbatim, whitespace-trimmed only. It
	// is what every other field is grounded against below.
	v.VisibleText = strings.TrimSpace(out.VisibleText)
	haystack := strings.ToLower(v.VisibleText)

	// BRAND — grounded exactly as the text tagger grounds it (case-insensitive, since a logo has a
	// case a year does not): kept only when it appears literally in the visibleText, dropped
	// otherwise. There is deliberately no SuggestedBrand — an ungrounded advertiser is a guess, not a
	// question worth a human, so it vanishes (the same call tag.go makes).
	if brand := strings.TrimSpace(out.Brand); brand != "" &&
		strings.Contains(haystack, strings.ToLower(brand)) {
		v.Brand = brand
	}
	// ERA — a plausible year (validateTags' own range) that ALSO appears literally in the visibleText.
	// A "1987" the model read off a corner grounds the era; a decade it inferred from the film stock
	// does not. Unlike the text tagger there is no SuggestedEra half: the frame-heuristic tier
	// (framehints.go) owns "this LOOKS old" suggestions, so an ungrounded vision year is simply
	// dropped rather than competing with it.
	if out.Era >= 1930 && out.Era <= 2035 && strings.Contains(v.VisibleText, strconv.Itoa(out.Era)) {
		v.Era = out.Era
	}
	// CATEGORY — TAXONOMY-grounded (§10 V45a) AND read off the frame: the model's category slug must
	// both RESOLVE against the taxonomy graph (forest.Resolve — the same resolve-or-drop gate the text
	// tagger uses, replacing the deleted hardcoded knownCategories map) AND appear in the visibleText.
	// The resolve check alone is not enough here — a vision model naming "cereal" for a box it cannot
	// actually read the label on is the same inference the grounding rule rejects everywhere else.
	// `v.Category` is the resolved canonical slug (the derived product-leaf shadow for this clip); the
	// vision tier records one category, and the tag job later fills the full taxonomy tag set.
	// ⚠ Grounded on the RAW claim, resolved for STORAGE. The visibleText check uses what the model
	// SAID it read (`raw`), because that is the token that could actually appear on a frame — a slug
	// like `fast_food` is a machine id, not on-screen text. Only after the raw claim is confirmed
	// present is it canonicalised via Resolve. So the anti-fabrication guarantee is unchanged (the
	// category must be in the text the model read) while resolution still maps `burgers`→`fast_food`.
	if raw := strings.TrimSpace(out.Category); raw != "" && forest != nil &&
		strings.Contains(haystack, strings.ToLower(raw)) {
		if slug, ok := forest.Resolve(raw); ok {
			v.Category = slug
		}
	}
	return v
}

// visionPrompt asks the multimodal model for exactly the fields groundVisionTags will check, and
// tells it the grounding rule in the same words the text tagger's prompt uses: read the on-screen
// text, and give a brand/era/category ONLY when that text spells it out. The model returning a value
// it did not read is not an error we can prevent at the prompt — it is why groundVisionTags exists —
// but asking for the honest answer costs nothing and reduces the drop rate.
const visionPrompt = `You are shown a few still frames from a short TV filler clip (a commercial/bumper/PSA).
First read any TEXT visible in the frames — a logo, a product name, a slogan, a year.
Return ONLY this JSON, no prose:
{"visibleText":"<the on-screen text you can read, verbatim; empty if none>","brand":"<advertiser name or empty>","era":<4-digit year visible in a frame, or 0>,"category":"toys|cereal|cars|tech|fast_food|movie_trailer|candy|games|psa|ident|bumper|general"}
Rules: put in visibleText only text you can actually READ in the frames; give brand ONLY when the advertiser's name is among that visible text — never guess it from the imagery or the products; give era ONLY when a 4-digit year is visible in a frame — never infer a decade from the film stock, colour, or style, use 0 otherwise; pick category from the list only, and only when the visible text supports it.`
