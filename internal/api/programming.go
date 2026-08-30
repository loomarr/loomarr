package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
)

// registerProgramming mounts the programming authoring surface's read endpoints (§6.6/§8.1,
// P6): the BE-authoritative rule vocabulary, and the whole-definition draft preview.
func (s *Server) registerProgramming(api huma.API) {
	huma.Register(api, withRole(huma.Operation{
		OperationID: "get-programming-vocabulary", Method: http.MethodGet, Path: "/v1/programming/vocabulary",
		Summary:     "The closed rule authoring vocabulary",
		Description: "The WHEN/WHAT/HOW curation-rule presets (§6.6): each token with its label and the value the BE lowers it to. The rules editor renders its picker from this and lowers identically to the server — so a hand-authored rule and an LLM-authored one are byte-identical, and the FE no longer hand-mirrors the lowering table. Read-only; any authenticated user.",
		Tags:        []string{"channels"},
	}, RoleMember), s.getProgrammingVocabulary)

	huma.Register(api, withRole(huma.Operation{
		OperationID: "preview-channel-programming", Method: http.MethodPost, Path: "/v1/channels/{id}/programming/preview",
		Summary:     "Preview an unsaved programming draft",
		Description: "Previews what a DRAFT {lineup?, policy?} would air — the cycle slots (which rule wins at `at`, the rolling window) AND the assembled break pool — WITHOUT saving or touching Tunarr. Runs the same ComputeDesiredAt + pod assembler as reconcile, so the preview cannot drift from what applying it would ship. Omitted lineup/policy fall back to the saved value. Admin-only (an authoring tool).",
		Tags:        []string{"channels"},
	}, RoleMember), s.previewChannelProgramming)
}

type programmingVocabularyOutput struct {
	Body schedule.Vocabulary
}

// getProgrammingVocabulary serves the closed authoring vocabulary (§6.6) so the rules editor
// stops hand-mirroring presets.go. Pure + read-only.
func (s *Server) getProgrammingVocabulary(_ context.Context, _ *struct{}) (*programmingVocabularyOutput, error) {
	return &programmingVocabularyOutput{Body: schedule.BuildVocabulary()}, nil
}

type previewProgrammingInput struct {
	ID   string `path:"id"`
	At   string `query:"at" doc:"RFC3339 wall-clock to preview (default: now). May be past or future."`
	Body struct {
		// Lineup is the draft lineup (omit to use the saved one). Lowered by key like a PATCH —
		// rich scheduling metadata carried forward, so the preview matches what a save would air.
		Lineup []LineupEntryDTO `json:"lineup,omitempty"`
		// Policy is the draft policy (omit to use the saved one). Validated like a policy write.
		Policy *schedule.ChannelPolicy `json:"policy,omitempty"`
	}
}

type previewProgrammingOutput struct {
	Body struct {
		At         string           `json:"at" doc:"The resolved wall-clock this preview was computed for (RFC3339)"`
		ActiveRule ActiveRuleDTO    `json:"activeRule"`
		WindowMs   int64            `json:"windowMs" doc:"Resolved rolling-window horizon in ms (0 = the whole run, no truncation)"`
		Slots      []CycleSlotDTO   `json:"slots" doc:"Leading slots of the resolved cycle, in play order (capped)"`
		Pods       PodPoolDTO       `json:"pods" doc:"The assembled break pool for the draft filler selection (§10)"`
		Excluded   ExcludedDTO      `json:"excluded" doc:"What the hard filters REFUSED and why — the answer to \"why isn't X on my channel\" (§4)"`
		Trace      ScheduleTraceDTO `json:"trace" doc:"Bounded scheduler-owned reasons emitted by the exact draft computation"`
	}
}

// ExcludedDTO renders schedule.ExclusionReport (§4). ⚠ The domain type has carried JSON tags
// since it was written and could be returned directly — it is restated here because the API
// owns its wire shape (a domain rename must not silently rewrite the contract), and because
// `reason` is a closed set the FE switches on, so it is declared as an enum for the generated
// client rather than an open string.
type ExcludedDTO struct {
	OverCeiling int               `json:"overCeiling" doc:"Titles refused for being rated above the channel's audience ceiling"`
	Unrated     int               `json:"unrated" doc:"Titles refused for carrying no usable rating under a kids ceiling (§4 fails closed)"`
	Items       []ExcludedItemDTO `json:"items" doc:"The refused items themselves, each with its reason"`
}

// ExcludedItemDTO is one refused item. ⚠ `key` is the PROVISIONING key, which for an episode
// refused by the per-episode ceiling is its SERIES key — several items can share one. `title`
// is what distinguishes them (it carries the SxxEyy for an episode), so it is the label to
// render, never the key.
type ExcludedItemDTO struct {
	Key    string `json:"key" doc:"Provisioning key of the refused title (a series key for a refused episode)"`
	Title  string `json:"title" doc:"Display label — for a refused episode this carries its season/episode"`
	Reason string `json:"reason" enum:"over_ceiling,unrated,out_of_scope,out_of_season" doc:"Which hard filter refused it"`
}

// excludedToDTO renders the report. It never returns a nil Items slice: the FE distinguishes
// "nothing was refused" from "the field is missing" by length, and a JSON `null` reads as
// neither.
func excludedToDTO(r schedule.ExclusionReport) ExcludedDTO {
	items := make([]ExcludedItemDTO, 0, len(r.Items))
	for _, it := range r.Items {
		items = append(items, ExcludedItemDTO{Key: string(it.Key), Title: it.Title, Reason: it.Reason})
	}
	return ExcludedDTO{OverCeiling: r.OverCeiling, Unrated: r.Unrated, Items: items}
}

// previewChannelProgramming is the whole-definition draft preview (P6): cycle slots + break
// pool for an unsaved {lineup?, policy?}, through the same code paths as reconcile so the
// preview can't disagree with what applying the draft would ship. Read-only.
func (s *Server) previewChannelProgramming(ctx context.Context, in *previewProgrammingInput) (*previewProgrammingOutput, error) {
	if s.channels == nil {
		return nil, errNotImplemented("Scheduling isn't set up", "Connect Tunarr in Settings → Connections to preview programming.")
	}
	at := time.Time{} // zero ⇒ the engine uses "now"
	if strings.TrimSpace(in.At) != "" {
		t, err := time.Parse(time.RFC3339, in.At)
		if err != nil {
			return nil, errBadRequest("Invalid time", "`at` must be an RFC3339 timestamp like 2026-12-25T09:00:00Z.")
		}
		at = t
	}

	// Draft lineup: validate + convert the lossy DTOs; the engine lowers them by key like a
	// PATCH (ApplyLineup PreserveByKey). Nil ⇒ the saved lineup.
	var draftLineup []schedule.LineupEntry
	if in.Body.Lineup != nil {
		entries, err := lineupEntriesFromDTOs(in.Body.Lineup)
		if err != nil {
			return nil, errBadRequest("Invalid lineup", err.Error())
		}
		draftLineup = entries
	}
	// Draft policy: validate it the same way a policy write does (§4 safety). Nil ⇒ saved.
	draftPolicy := in.Body.Policy
	if draftPolicy != nil {
		if err := draftPolicy.Validate(); err != nil {
			return nil, apiErrWithCause(http.StatusUnprocessableEntity, "Invalid policy",
				"Some programming policy settings are invalid. Check the audience and filler options, then try again.", err)
		}
	}

	cycle, err := s.channels.CyclePreviewDraft(ctx, in.ID, at, draftLineup, draftPolicy)
	if errors.Is(err, store.ErrNotFound) {
		return nil, errNotFound("Channel not found", "That channel doesn't exist — it may have been removed.")
	} else if err != nil {
		return nil, err
	}

	// Pods: the assembled break pool for the DRAFT filler (or the saved one when no policy
	// draft is given). Same assembler + seed as reconcile, so preview == reality.
	pool := PodPoolDTO{Entries: []PodEntryDTO{}}
	if s.pods != nil {
		var pod filler.Pod
		if draftPolicy != nil {
			var sel schedule.FillerSelection
			if draftPolicy.Filler != nil {
				sel = *draftPolicy.Filler
			}
			// ⚠ The DRAFT's scope era, not the saved channel's — this preview is answering
			// "what would this policy play", and an unset filler era inherits from scope (V51f).
			pod, err = s.pods.PreviewDraft(ctx, in.ID, fillerSelectionToDomain(sel, draftPolicy.Scope.Era))
		} else {
			pod, err = s.pods.Preview(ctx, in.ID)
		}
		if err != nil {
			return nil, err
		}
		pool = podToPoolDTO(pod)
	}

	out := &previewProgrammingOutput{}
	out.Body.At = cycle.At.UTC().Format(time.RFC3339)
	out.Body.ActiveRule = ActiveRuleDTO{
		ID: cycle.Active.ID, Label: cycle.Active.Label, Priority: cycle.Active.Priority, Matched: cycle.Active.Matched,
	}
	out.Body.WindowMs = cycle.Window.Milliseconds()
	// ⚠ Slots are CAPPED (cyclePreviewSlotCap) and the exclusion report is NOT: they answer
	// different questions. A truncated "what airs" is still useful; a truncated "what was
	// refused" would understate a safety filter, which is the one thing this must not do.
	out.Body.Slots = cycleSlotsToDTO(cycle.Slots, cyclePreviewSlotCap)
	out.Body.Pods = pool
	out.Body.Excluded = excludedToDTO(cycle.Excluded)
	out.Body.Trace = scheduleTraceToDTO(cycle.Trace)
	return out, nil
}
