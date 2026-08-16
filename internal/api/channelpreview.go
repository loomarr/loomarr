package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/mantonx/loomarr/internal/channels"
	"github.com/mantonx/loomarr/internal/filler"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
)

// Channel PREVIEW surfaces — "what would this channel actually play?" (§10, §12).
//
// Split out of channels.go, which had accreted 15 handlers and 25 DTOs. These are not channel
// CRUD: they answer a different question, and their helpers (podToPoolDTO, cycleSlotsToDTO,
// fillerSelectionToDomain) are shared with programming.go and guide.go — so they were never
// channel-lifecycle code, they had just been living in the channel-lifecycle file.
//
// Two previews, one assembler. The pod endpoints and the cycle endpoint both run the SAME
// assembler and the SAME seed the reconciler uses (§10's one-assembler rule), so a preview
// cannot promise a break the channel will not play.

// PodEntryDTO is one clip placed in the previewed pool, in play order.
type PodEntryDTO struct {
	Path            string `json:"path,omitempty" doc:"The clip's identity, relative to FILLER_DIR. Empty for the embedded fallback bumper card, which is not a file."`
	TunarrProgramID string `json:"tunarrProgramId,omitempty" doc:"Tunarr's program id for this clip, when Tunarr knows it. Empty for the fallback bumper card AND on installs without Tunarr — key on path, not this."`
	Name            string `json:"name"`
	Kind            string `json:"kind" enum:"commercial,bumper,station_id,psa,trailer,interstitial"`
	DurationMs      int64  `json:"durationMs"`
	IsFallbackCard  bool   `json:"isFallbackCard" doc:"The embedded default bumper card — the bottom of the fallback ladder"`
	// Era and Quality are DISPLAY context for the guide's pod hover card (§12): they explain
	// why a break looks the way it does ("1994 · 480p" reads as an authentic capture rather
	// than a playback fault). Neither affects selection — see filler.Clip.Quality.
	Era     int    `json:"era,omitempty" doc:"The clip's tagged era (1994); 0 when untagged"`
	Quality string `json:"quality,omitempty" doc:"Resolution label from the probed video height (1080p, 480p); empty when unknown or audio-only"`
	// Brand, Audience, Category and VisibleText are the richer §10 V44 display context, the
	// same nature as Era/Quality: they explain what a break is FOR ("Kellogg's · cereal · kids"),
	// never affect selection, and are omitempty because "" is the honest common case for an
	// untagged clip. Each is GROUNDED at the source — it persists on the clip only when a text or
	// visual signal literally contained it (§8) — so what reaches the card is fact, not inference.
	Brand    string `json:"brand,omitempty" doc:"The advertiser the clip is for (Kellogg's, Ford); empty when ungrounded/untagged (V44)"`
	Audience string `json:"audience,omitempty" enum:"kids,family,general,late_night" doc:"Who the clip suits; empty when untagged"`
	Category string `json:"category,omitempty" doc:"The clip's category (cereal, toys, cars, …); empty when untagged"`
	// VisibleText is the on-screen text a vision pass read off the keyframes (V44). It is what
	// makes a vision-grounded brand/era AUDITABLE — the frame text the tag was read from — so the
	// card can show the evidence, not just the conclusion. Empty when no vision pass ran or it read nothing.
	VisibleText string `json:"visibleText,omitempty" doc:"On-screen text a vision pass read off the frame (V44); empty when none"`
}

type previewPodsInput struct {
	ID string `path:"id"`
}

// PodPoolDTO is a channel's assembled filler pool (§10) — the break preview. Named (not an
// inline body) so the pods endpoints AND the programming/preview endpoint share one shape.
type PodPoolDTO struct {
	Entries []PodEntryDTO `json:"entries"`
	TotalMs int64         `json:"totalMs"`
	// MatchLevel is how far down the §10 fallback ladder assembly had to go. This
	// is the answer to "why are my commercials wrong": exact means era+audience
	// matched, and bumper_card means nothing matched and the channel is running on
	// the embedded card alone.
	// Enumerated so orval generates a union the FE can switch on exhaustively. Left
	// as a bare string, the frontend would hand-mirror these values — and the one
	// that already did drifted out of sync with the ladder's real levels.
	MatchLevel string `json:"matchLevel" enum:"exact,widened,audience,bumper_card" doc:"How far down the fallback ladder assembly went (§10)"`
}

type previewPodsOutput struct {
	Body PodPoolDTO
}

// previewDraftPodsOutput is the DRAFT preview: the assembled pod AND the coverage the same draft
// resolves to (§10 V51f).
//
// ⚠ **They ship together because they were describing two different selections.** The coverage
// meter read the SAVED policy while the timeline directly beneath it rendered the DRAFT — so
// during an edit the page showed a meter for one selection above a pod for another, with nothing
// saying so. One response, one selection, computed from the same `filler.Selection`.
type previewDraftPodsOutput struct {
	Body struct {
		PodPoolDTO
		Coverage CoverageDTO `json:"coverage" doc:"What this DRAFT selection resolves to on the ladder — the same selection the pod above was assembled from."`
	}
}

// previewChannelPods shows the filler pool a channel would receive, without touching
// Tunarr (§12). It runs the SAME assembler and the SAME seed as reconcile, so preview
// and reality cannot disagree — a preview that could drift from what actually ships
// would be worse than none.
func (s *Server) previewChannelPods(ctx context.Context, in *previewPodsInput) (*previewPodsOutput, error) {
	if s.pods == nil {
		return nil, errNotImplemented("Filler isn't set up", "Set up commercials and filler before previewing a channel's pods.")
	}
	if _, err := s.store.GetChannel(ctx, in.ID); errors.Is(err, store.ErrNotFound) {
		return nil, errNotFound("Channel not found", "That channel doesn't exist — it may have been removed.")
	} else if err != nil {
		return nil, err
	}

	pod, err := s.pods.Preview(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	return podToPreviewOutput(pod), nil
}

// CoverageRungDTO is one rung of the §10 fallback ladder and how much material it holds.
type CoverageRungDTO struct {
	// Same enum as PodPoolDTO.MatchLevel, deliberately: a coverage answer and a pod outcome
	// describe the same ladder, and the meter's whole claim is that they agree. Two
	// vocabularies for one concept is how they start disagreeing.
	Level string `json:"level" enum:"exact,widened,audience,bumper_card" doc:"The ladder rung (§10)"`
	Clips int    `json:"clips" doc:"Eligible clips this rung holds, after the channel's kind/category/exclusion narrowing"`
}

// CoverageCriterionDTO is one of the channel's break SETTINGS and how much of the commercial
// catalog survives it on its own.
//
// ⚠ **Independent, not cumulative — that is what makes it actionable.** The operator's question is
// "which of my settings is costing me the clips", and a cumulative funnel cannot answer it: the
// order the predicates run in would decide which setting gets the blame. Counted in isolation,
// `audience: 0` beside `era: 214` names the culprit.
type CoverageCriterionDTO struct {
	Criterion string `json:"criterion" enum:"era,audience,category,kind,duration,quality" doc:"The channel setting this count is about"`
	Clips     int    `json:"clips" doc:"Break-body commercials passing THIS setting alone, ignoring the others"`
}

// CoverageDTO answers "what would this channel's breaks resolve to", from the same ladder
// reconcile uses (V29a/V29b).
type CoverageDTO struct {
	// ⚠ Every rung is always present since V51f. `EraStrict` (retired-ok) was the only thing that could skip
	// one, and it was unreachable — set in tests and nowhere else — so the absent-not-zero rule
	// it justified went with it. A rung at 0 now means what a reader assumes it means.
	Rungs []CoverageRungDTO `json:"rungs" doc:"Ladder rungs, TIGHTEST FIRST. Always all of them; a rung at 0 means nothing in the catalog reaches it."`
	// Criteria is the per-setting breakdown — which single setting is emptying the ladder (V51f).
	Criteria []CoverageCriterionDTO `json:"criteria" doc:"Per-setting counts, each measured independently, so a zero identifies the setting to change."`
	Level    string                 `json:"level" enum:"exact,widened,audience,bumper_card" doc:"The rung a break would actually be filled from — the tightest non-empty one. bumper_card means nothing matches and breaks run on the embedded card."`
	Total    int                    `json:"total" doc:"Distinct eligible clips across the widest rung. NOT a sum of the rungs — they nest, so adding them counts one clip up to three times."`
}

type channelCoverageOutput struct {
	Body CoverageDTO
}

// channelFillerCoverage reports which ladder rung this channel's breaks draw from.
//
// ⚠ **The whole point is that this cannot disagree with what airs.** The v2 mock's meter
// claims to come "from the same ladder reconcile uses" and does not — it recomputes its
// buckets inline, with five mutually inconsistent era/audience predicates. Here the answer
// comes from `filler.Coverage`, which calls the same `candidatePools` that `Assemble` calls,
// through the same `SelectionForChannel` derivation the previews use. A meter that says
// "exact" while breaks resolve at "audience" is a confident wrong answer about why a channel
// sounds the way it does, which is worse than no meter.
//
// Read-only, so any authenticated user may call it — same as the pod preview it describes.
func (s *Server) channelFillerCoverage(ctx context.Context, in *previewPodsInput) (*channelCoverageOutput, error) {
	if s.pods == nil {
		return nil, errNotImplemented("Filler isn't set up", "Set up commercials and filler before checking a channel's coverage.")
	}
	if _, err := s.store.GetChannel(ctx, in.ID); errors.Is(err, store.ErrNotFound) {
		return nil, errNotFound("Channel not found", "That channel doesn't exist — it may have been removed.")
	} else if err != nil {
		return nil, err
	}

	report, err := s.pods.Coverage(ctx, in.ID)
	if err != nil {
		return nil, err
	}

	// Non-nil even when empty: a JSON `null` here would make every consumer guard before
	// iterating, and "no rungs" is a real answer (an unconfigured catalog), not a missing one.
	return &channelCoverageOutput{Body: coverageDTO(report)}, nil
}

// coverageDTO renders a coverage report. Shared by the saved-coverage endpoint and the DRAFT
// preview (V51f) so the two cannot describe the same ladder differently.
func coverageDTO(report filler.CoverageReport) CoverageDTO {
	// Non-nil even when empty: a JSON `null` here would make every consumer guard before
	// iterating, and "no rungs" is a real answer (an unconfigured catalog), not a missing one.
	rungs := make([]CoverageRungDTO, 0, len(report.Rungs))
	for _, r := range report.Rungs {
		rungs = append(rungs, CoverageRungDTO{Level: string(r.Level), Clips: r.Clips})
	}
	criteria := make([]CoverageCriterionDTO, 0, len(report.Criteria))
	for _, c := range report.Criteria {
		criteria = append(criteria, CoverageCriterionDTO{Criterion: string(c.Criterion), Clips: c.Clips})
	}
	return CoverageDTO{
		Rungs:    rungs,
		Criteria: criteria,
		Level:    string(report.Level),
		Total:    report.Total,
	}
}

// CycleSlotDTO is one slot of the previewed cycle (§8.1): what airs, in play order. A
// program carries its title + provisioning key + series identity + multi-part index; a break
// gap is `kind:"break"` (Tunarr owns the clip that fills it — the preview shows the gap).
type CycleSlotDTO struct {
	Kind  string `json:"kind" enum:"program,pending,break" doc:"program = a playable title; pending = an acquisition not yet available; break = a commercial gap"`
	Title string `json:"title,omitempty" doc:"Display label; empty for a break gap"`
	Key   string `json:"key,omitempty" doc:"Provisioning key of the title (empty for a break)"`
	// Part is the 1-based play order within a multi-part/franchise group (§5) — >0 when this
	// slot is part of a two-parter or a movie franchise kept together, 0 for a standalone.
	Part int `json:"part,omitempty" doc:"Play order within a multi-part or franchise group (0 = standalone)"`
	// Season / Episode are the episode's numbers for a series program, so the UI can show
	// "S1E5" alongside the episode title. 0 for a movie or when the media server doesn't
	// number the item. The show *name* is not here — the client maps Key → its lineup entry's
	// name (it already has the lineup), so the DTO stays lookup-free.
	Season  int `json:"season,omitempty" doc:"Series season number (0 = movie/unknown)"`
	Episode int `json:"episode,omitempty" doc:"Series episode number (0 = movie/unknown)"`
}

// ActiveRuleDTO attributes the previewed cycle to the curation rule active at the previewed
// moment (§8.1) — the answer to "which rule is playing right now". Matched=false means no
// rule matched and the channel is on its base whole-policy behavior (label "Base policy").
type ActiveRuleDTO struct {
	ID       string `json:"id" doc:"Stable rule id ('' when no rule matched)"`
	Label    string `json:"label" doc:"Human-readable rule name, or 'Base policy' when none matched"`
	Priority int    `json:"priority" doc:"The rule's priority (higher wins overlaps); 0 for the base policy"`
	Matched  bool   `json:"matched" doc:"Whether a curation rule matched (false = base whole-policy behavior)"`
}

type previewCycleInput struct {
	ID string `path:"id"`
	At string `query:"at" doc:"RFC3339 wall-clock to preview (default: now). May be past or future — 'what airs next Christmas morning?'"`
}

type previewCycleOutput struct {
	Body struct {
		At         string         `json:"at" doc:"The resolved wall-clock this preview was computed for (RFC3339)"`
		ActiveRule ActiveRuleDTO  `json:"activeRule"`
		WindowMs   int64          `json:"windowMs" doc:"Resolved rolling-window horizon in ms (0 = the whole run, no truncation)"`
		Slots      []CycleSlotDTO `json:"slots" doc:"The leading slots of the resolved cycle, in play order (capped)"`
		Excluded   ExcludedDTO    `json:"excluded" doc:"What the hard filters REFUSED and why — the answer to \"why isn't X on my channel\" (§4)"`
	}
}

// cyclePreviewSlotCap bounds how many slots the preview returns — enough to see intermixing,
// marathons, and franchise/two-parter adjacency at a glance without shipping an 800-item deck.
const cyclePreviewSlotCap = 50

// previewChannelCycle answers "what airs at this moment, and which rule is active" (§8.1). It
// runs the SAME pure lineup builder as reconcile at the chosen wall-clock, so the preview and
// the real guide cannot disagree — WITHOUT touching Tunarr or persisting anything. Read-only,
// so any authenticated user may call it; the moment may be past or future.
func (s *Server) previewChannelCycle(ctx context.Context, in *previewCycleInput) (*previewCycleOutput, error) {
	if s.channels == nil {
		return nil, errNotImplemented("Scheduling isn't set up", "Connect Tunarr in Settings → Connections to preview a channel's cycle.")
	}
	at := time.Time{} // zero ⇒ the engine uses "now"
	if strings.TrimSpace(in.At) != "" {
		t, err := time.Parse(time.RFC3339, in.At)
		if err != nil {
			return nil, errBadRequest("Invalid time", "`at` must be an RFC3339 timestamp like 2026-12-25T09:00:00Z.")
		}
		at = t
	}

	// ⚠ CyclePreviewDraft with nil drafts, NOT CyclePreview — identical work (CyclePreview is
	// literally this call, unpacked), but it also carries the §4 exclusion report. This is the
	// SAVED preview, the one an operator sees without editing anything, so it is where "why
	// isn't X on my channel" actually gets asked; leaving the report to the draft-only endpoint
	// would answer the question only for someone already mid-edit.
	//
	// No cache is bypassed by this: the arranged-cycle cache lives in the playout adapter
	// (app/cyclecache.go), which reaches the engine directly and never comes through here.
	cycle, err := s.channels.CyclePreviewDraft(ctx, in.ID, at, nil, nil)
	if errors.Is(err, store.ErrNotFound) {
		return nil, errNotFound("Channel not found", "That channel doesn't exist — it may have been removed.")
	} else if err != nil {
		return nil, err
	}

	out := &previewCycleOutput{}
	out.Body.At = cycle.At.UTC().Format(time.RFC3339)
	out.Body.ActiveRule = ActiveRuleDTO{
		ID: cycle.Active.ID, Label: cycle.Active.Label, Priority: cycle.Active.Priority, Matched: cycle.Active.Matched,
	}
	out.Body.WindowMs = cycle.Window.Milliseconds()
	out.Body.Slots = cycleSlotsToDTO(cycle.Slots, cyclePreviewSlotCap)
	out.Body.Excluded = excludedToDTO(cycle.Excluded)
	return out, nil
}

// cycleSlotsToDTO maps the resolved cycle's slots to the preview DTO, capped at `limit`.
// Only program/pending/break kinds are meaningful to the preview; a filler slot with no clip
// yet renders as a "break" gap (that's what the guide shows). Franchise/multi-part order is
// preserved via Part.
func cycleSlotsToDTO(slots []schedule.Slot, limit int) []CycleSlotDTO {
	out := make([]CycleSlotDTO, 0, min(len(slots), limit))
	for _, sl := range slots {
		if len(out) >= limit {
			break
		}
		dto := CycleSlotDTO{Title: sl.Title, Part: sl.PartIndex}
		switch sl.Kind {
		case schedule.SlotProgram:
			dto.Kind = "program"
			dto.Key = string(sl.Key)
			dto.Season, dto.Episode = sl.Season, sl.Episode
		case schedule.SlotPending:
			dto.Kind = "pending"
			dto.Key = string(sl.Key)
		default: // SlotFiller / flex → a commercial break gap
			dto.Kind = "break"
			dto.Title = ""
		}
		out = append(out, dto)
	}
	return out
}

// podToPreviewOutput renders an assembled pod into the preview response — shared by the
// saved (GET) and draft (POST) preview handlers so their shapes cannot drift.
func podToPreviewOutput(pod filler.Pod) *previewPodsOutput {
	return &previewPodsOutput{Body: podToPoolDTO(pod)}
}

// podToPoolDTO maps an assembled pod to its DTO. Shared by the pods endpoints and the
// programming/preview endpoint (which embeds the pool alongside the cycle slots).
func podToPoolDTO(pod filler.Pod) PodPoolDTO {
	// Always a slice, never null: an empty catalog is a normal state the UI renders as
	// "no clips yet", and a null here would make the FE guard a case that isn't an error.
	entries := make([]PodEntryDTO, 0, len(pod.Entries))
	for _, e := range pod.Entries {
		entries = append(entries, PodEntryDTO{
			Path:            e.Path,
			TunarrProgramID: e.TunarrProgramID,
			Name:            e.Name,
			Kind:            string(e.Kind),
			DurationMs:      e.DurationMs,
			IsFallbackCard:  e.IsFallbackCard,
			Era:             e.Era,
			Quality:         e.Quality,
			Brand:           e.Brand,
			Audience:        string(e.Audience),
			Category:        e.Category,
			VisibleText:     e.VisibleText,
		})
	}
	return PodPoolDTO{Entries: entries, TotalMs: pod.TotalMs, MatchLevel: string(pod.MatchLevel)}
}

type previewDraftPodsInput struct {
	ID   string `path:"id"`
	Body struct {
		// Filler is the DRAFT selection to preview (§10) — the unsaved selection the
		// sandbox is experimenting with. Same shape as policy.filler; validated like a
		// policy write, then assembled without persisting anything.
		Filler schedule.FillerSelection `json:"filler"`
		// BreakDuration is the unsaved per-channel override. Nil inherits the live global
		// setting; zero is invalid because breaks_per_hour owns the off switch.
		BreakDuration *schedule.Duration `json:"breakDuration,omitempty"`
	}
}

// previewDraftChannelPods assembles the pool a DRAFT filler selection would produce,
// without saving it (§10/§12 — the channel-page sandbox). Admin-only: it's an authoring
// tool (applying the selection is a normal PATCH of policy.filler). Runs the SAME
// assembler + seed as the saved preview and reconcile, so the sandbox shows exactly what
// will air once applied — only the (draft) selection differs.
func (s *Server) previewDraftChannelPods(ctx context.Context, in *previewDraftPodsInput) (*previewDraftPodsOutput, error) {
	if s.pods == nil {
		return nil, errNotImplemented("Filler isn't set up", "Set up commercials and filler before previewing a channel's pods.")
	}
	// ⚠ The channel is KEPT, not discarded (V51f): its `scope.era` is what an unset filler era
	// inherits, so a draft preview that dropped it previewed a different pool than reconcile built.
	ch, err := s.store.GetChannel(ctx, in.ID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, errNotFound("Channel not found", "That channel doesn't exist — it may have been removed.")
	} else if err != nil {
		return nil, err
	}
	// Validate the draft with the same rules a policy write uses (bad audience/kind/
	// category/era → 422) — the sandbox must reject a nonsense selection, not assemble it.
	if err := (schedule.ChannelPolicy{OperatorPolicy: schedule.OperatorPolicy{
		Filler: &in.Body.Filler, BreakDuration: in.Body.BreakDuration,
	}}).Validate(); err != nil {
		return nil, apiErrWithCause(http.StatusUnprocessableEntity, "Invalid filler selection",
			"Some filler options are invalid. Check the audience, kinds, and categories, then try again.", err)
	}

	// ⚠ ONE resolved selection feeds both the pod and the meter (V51f). Resolving it twice would
	// reintroduce, inside a single handler, exactly the drift this pairing exists to remove.
	sel := fillerSelectionToDomain(in.Body.Filler, ch.Policy.Scope.Era)
	if in.Body.BreakDuration != nil {
		sel.BreakDurationMs = in.Body.BreakDuration.Std().Milliseconds()
	}

	pod, err := s.pods.PreviewDraft(ctx, in.ID, sel)
	if err != nil {
		return nil, err
	}
	report, err := s.pods.CoverageDraft(ctx, in.ID, sel)
	if err != nil {
		return nil, err
	}

	out := &previewDraftPodsOutput{}
	out.Body.PodPoolDTO = podToPoolDTO(pod)
	out.Body.Coverage = coverageDTO(report)
	return out, nil
}

// fillerSelectionToDomain translates a DRAFT FillerSelection into the domain Selection.
//
// ⚠ **It delegates to `channels.SelectionFrom` rather than mirroring it (V51f).** The rule had
// THREE implementations — `SelectionForChannel`, this one, and `podPreviewAdapter.PreviewDraft` —
// and this was the one that omitted era inheritance. Nothing looked broken only because the
// adapter downstream re-applied it, so two copies cancelled out. That accident stops working the
// moment "explicitly any era" is reachable: a fallback keyed on `Era == 0` cannot tell an unset
// era from a chosen one. `scopeEra` comes from the channel the handler already loads.
func fillerSelectionToDomain(f schedule.FillerSelection, scopeEra *schedule.Range) filler.Selection {
	return channels.SelectionFrom(&f, scopeEra)
}
