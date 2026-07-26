package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

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

	resolvedAt, slots, active, window, err := s.channels.CyclePreview(ctx, in.ID, at)
	if errors.Is(err, store.ErrNotFound) {
		return nil, errNotFound("Channel not found", "That channel doesn't exist — it may have been removed.")
	} else if err != nil {
		return nil, err
	}

	out := &previewCycleOutput{}
	out.Body.At = resolvedAt.UTC().Format(time.RFC3339)
	out.Body.ActiveRule = ActiveRuleDTO{ID: active.ID, Label: active.Label, Priority: active.Priority, Matched: active.Matched}
	out.Body.WindowMs = window.Milliseconds()
	out.Body.Slots = cycleSlotsToDTO(slots, cyclePreviewSlotCap)
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
	}
}

// previewDraftChannelPods assembles the pool a DRAFT filler selection would produce,
// without saving it (§10/§12 — the channel-page sandbox). Admin-only: it's an authoring
// tool (applying the selection is a normal PATCH of policy.filler). Runs the SAME
// assembler + seed as the saved preview and reconcile, so the sandbox shows exactly what
// will air once applied — only the (draft) selection differs.
func (s *Server) previewDraftChannelPods(ctx context.Context, in *previewDraftPodsInput) (*previewPodsOutput, error) {
	if err := requireAdmin(ctx); err != nil {
		return nil, err
	}
	if s.pods == nil {
		return nil, errNotImplemented("Filler isn't set up", "Set up commercials and filler before previewing a channel's pods.")
	}
	if _, err := s.store.GetChannel(ctx, in.ID); errors.Is(err, store.ErrNotFound) {
		return nil, errNotFound("Channel not found", "That channel doesn't exist — it may have been removed.")
	} else if err != nil {
		return nil, err
	}
	// Validate the draft with the same rules a policy write uses (bad audience/kind/
	// category/era → 422) — the sandbox must reject a nonsense selection, not assemble it.
	if err := (schedule.ChannelPolicy{OperatorPolicy: schedule.OperatorPolicy{Filler: &in.Body.Filler}}).Validate(); err != nil {
		return nil, apiErrWithCause(http.StatusUnprocessableEntity, "Invalid filler selection",
			"Some filler options are invalid. Check the audience, kinds, and categories, then try again.", err)
	}

	pod, err := s.pods.PreviewDraft(ctx, in.ID, fillerSelectionToDomain(in.Body.Filler))
	if err != nil {
		return nil, err
	}
	return podToPreviewOutput(pod), nil
}

// fillerSelectionToDomain translates the policy/DTO FillerSelection into the filler-
// package Selection the assembler consumes (the API's boundary translation, mirroring
// channels.SelectionForChannel but for a draft that isn't tied to a stored channel).
func fillerSelectionToDomain(f schedule.FillerSelection) filler.Selection {
	sel := filler.Selection{
		Audience:   filler.Audience(f.Audience),
		Categories: f.Categories,
		Kinds:      f.Kinds,
		Pinned:     f.Pinned,
		Excluded:   f.Excluded,
	}
	if f.Era != nil {
		sel.Era = f.Era.From
	}
	return sel
}
