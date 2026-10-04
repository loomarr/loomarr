package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/loomarr/loomarr/internal/channels"
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

	huma.Register(api, withRole(huma.Operation{
		OperationID: "preview-channel-programming-changes", Method: http.MethodPost, Path: "/v1/channels/{id}/programming/changes",
		Summary:     "List the upcoming slots an unsaved programming draft changes",
		Description: "Compares the SAVED channel with a DRAFT {lineup?, policy?} (the same body and validation as programming/preview) over the next `horizonHours`, and returns only the slots where they air something different: when, what airs before → after, and which rule wins on each side. Both sides are forecast by the same engine calls from the same instant, so a slot the draft does not touch compares equal. Read-only; nothing is saved and no backend is called.",
		Tags:        []string{"channels"},
	}, RoleMember), s.previewChannelProgrammingChanges)
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
	at, err := parsePreviewInstant("at", in.At)
	if err != nil {
		return nil, err
	}
	draftLineup, draftPolicy, err := lowerProgrammingDraft(in.Body.Lineup, in.Body.Policy)
	if err != nil {
		return nil, err
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
			// The lineup the derivation reads is the draft's when given, else the saved one.
			lineup := draftLineup
			if lineup == nil {
				saved, gerr := s.store.GetChannel(ctx, in.ID)
				if gerr != nil {
					return nil, gerr
				}
				lineup = saved.Lineup
			}
			pod, err = s.pods.PreviewDraft(ctx, in.ID, fillerSelectionToDomain(sel, *draftPolicy, lineup))
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

// parsePreviewInstant reads an optional RFC3339 query instant; empty is the zero time, which the
// engine resolves to "now".
func parsePreviewInstant(name, raw string) (time.Time, error) {
	if strings.TrimSpace(raw) == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return time.Time{}, errBadRequest("Invalid time", "`"+name+"` must be an RFC3339 timestamp like 2026-12-25T09:00:00Z.")
	}
	return t, nil
}

// lowerProgrammingDraft validates and lowers a {lineup?, policy?} draft exactly as a save would.
// Shared by both draft previews so the change list cannot accept a draft the single-point
// preview refuses, or lower it differently.
func lowerProgrammingDraft(lineup []LineupEntryDTO, policy *schedule.ChannelPolicy) ([]schedule.LineupEntry, *schedule.ChannelPolicy, error) {
	// Draft lineup: validate + convert the lossy DTOs; the engine lowers them by key like a
	// PATCH (ApplyLineup PreserveByKey). Nil ⇒ the saved lineup.
	var draftLineup []schedule.LineupEntry
	if lineup != nil {
		entries, err := lineupEntriesFromDTOs(lineup)
		if err != nil {
			return nil, nil, errBadRequest("Invalid lineup", err.Error())
		}
		draftLineup = entries
	}
	// Draft policy: validate it the same way a policy write does (§4 safety). Nil ⇒ saved.
	// A lone `era` was already folded into Dates on decode; `era` with `dates` is ambiguous.
	if policy != nil {
		if err := policy.DateAliasConflict(); err != nil {
			return nil, nil, ambiguousDatesError(err)
		}
		if err := policy.Validate(); err != nil {
			return nil, nil, apiErrWithCause(http.StatusUnprocessableEntity, "Invalid policy",
				"Some programming policy settings are invalid. Check the audience and filler options, then try again.", err)
		}
	}
	return draftLineup, policy, nil
}

type previewProgrammingChangesInput struct {
	ID           string `path:"id"`
	From         string `query:"from" doc:"RFC3339 instant the comparison starts at (default: now)."`
	HorizonHours int    `query:"horizonHours" minimum:"0" maximum:"168" doc:"How far ahead to compare, in hours. 0 = the channel's schedule window (24h when unbounded); at most 168."`
	// Body is the SAME draft shape as programming/preview, deliberately restated rather than
	// shared: naming the type would rename the existing endpoint's generated request type.
	Body struct {
		Lineup []LineupEntryDTO        `json:"lineup,omitempty"`
		Policy *schedule.ChannelPolicy `json:"policy,omitempty"`
	}
}

// ProgrammingChangeSideDTO is what one side (saved or draft) airs at a changed slot.
type ProgrammingChangeSideDTO struct {
	Kind    string        `json:"kind" enum:"program,break" doc:"program = a title; break = a commercial gap"`
	Title   string        `json:"title,omitempty" doc:"Episode or film name; empty for a break"`
	Series  string        `json:"series,omitempty" doc:"The show's name for an episode; absent for a film or break"`
	Key     string        `json:"key,omitempty" doc:"Provisioning key; empty for a break"`
	Season  int           `json:"season,omitempty"`
	Episode int           `json:"episode,omitempty"`
	StartMs int64         `json:"startMs" doc:"When this airing starts, epoch ms (cut where the arrangement changes)"`
	StopMs  int64         `json:"stopMs" doc:"When this airing ends, epoch ms, exclusive"`
	Rule    ActiveRuleDTO `json:"rule" doc:"The curation rule whose arrangement this airing comes from"`
}

// ProgrammingChangeDTO is one upcoming slot the draft changes.
type ProgrammingChangeDTO struct {
	StartMs int64                     `json:"startMs" doc:"Where the difference begins, epoch ms"`
	EndMs   int64                     `json:"endMs" doc:"Where either side's airing ends, epoch ms"`
	Before  *ProgrammingChangeSideDTO `json:"before,omitempty" doc:"What the SAVED channel airs here; absent when nothing airable does"`
	After   *ProgrammingChangeSideDTO `json:"after,omitempty" doc:"What the DRAFT would air here; absent when nothing airable would"`
}

type previewProgrammingChangesOutput struct {
	Body struct {
		FromMs    int64                  `json:"fromMs" doc:"Start of the compared span, epoch ms"`
		ToMs      int64                  `json:"toMs" doc:"End of the compared span, epoch ms. Never past what both sides were forecast for."`
		Compared  int                    `json:"compared" doc:"Programmes the SAVED channel airs in the span — tells 'nothing changes' from 'nothing is scheduled'"`
		Count     int                    `json:"count" doc:"Total changed slots in the span, before the response cap"`
		Truncated bool                   `json:"truncated" doc:"Whether changes were omitted by the response cap"`
		Changes   []ProgrammingChangeDTO `json:"changes" doc:"Changed slots in time order, capped; [] when the draft changes nothing"`
	}
}

// programmingChangesCap bounds the change list. A sweeping edit (a new ordering) changes every
// slot of the week; past this the count says how many, and the list is not what the editor reads.
const programmingChangesCap = 100

// previewChannelProgrammingChanges is the change list: the upcoming slots where an unsaved draft
// airs something different from the saved channel (#1877). Same draft, validation and
// authorization as programming/preview; the comparison is the engine's (ScheduleDiffDraft).
func (s *Server) previewChannelProgrammingChanges(ctx context.Context, in *previewProgrammingChangesInput) (*previewProgrammingChangesOutput, error) {
	if s.channels == nil {
		return nil, errNotImplemented("Scheduling isn't set up", "Connect Tunarr in Settings → Connections to preview programming.")
	}
	from, err := parsePreviewInstant("from", in.From)
	if err != nil {
		return nil, err
	}
	draftLineup, draftPolicy, err := lowerProgrammingDraft(in.Body.Lineup, in.Body.Policy)
	if err != nil {
		return nil, err
	}

	diff, err := s.channels.ScheduleDiffDraft(ctx, in.ID, from, time.Duration(in.HorizonHours)*time.Hour, draftLineup, draftPolicy)
	if errors.Is(err, store.ErrNotFound) {
		return nil, errNotFound("Channel not found", "That channel doesn't exist — it may have been removed.")
	} else if err != nil {
		return nil, err
	}

	out := &previewProgrammingChangesOutput{}
	out.Body.FromMs, out.Body.ToMs = diff.From.UnixMilli(), diff.To.UnixMilli()
	out.Body.Compared = diff.Compared
	out.Body.Count = len(diff.Changes)
	out.Body.Truncated = len(diff.Changes) > programmingChangesCap
	out.Body.Changes = make([]ProgrammingChangeDTO, 0, min(len(diff.Changes), programmingChangesCap))
	for _, c := range diff.Changes[:min(len(diff.Changes), programmingChangesCap)] {
		out.Body.Changes = append(out.Body.Changes, ProgrammingChangeDTO{
			StartMs: c.Start.UnixMilli(), EndMs: c.End.UnixMilli(),
			Before: changeSideToDTO(c.Before), After: changeSideToDTO(c.After),
		})
	}
	return out, nil
}

func changeSideToDTO(a *channels.ForecastAiring) *ProgrammingChangeSideDTO {
	if a == nil {
		return nil
	}
	kind := "program"
	if a.Kind != schedule.SlotProgram {
		kind = "break"
	}
	return &ProgrammingChangeSideDTO{
		Kind: kind, Title: a.Title, Series: a.SeriesTitle, Key: string(a.Key),
		Season: a.Season, Episode: a.Episode, StartMs: a.Start.UnixMilli(), StopMs: a.Stop.UnixMilli(),
		Rule: ActiveRuleDTO{ID: a.Rule.ID, Label: a.Rule.Label, Priority: a.Rule.Priority, Matched: a.Rule.Matched},
	}
}
