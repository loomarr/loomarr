package fillerreview

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/loomarr/loomarr/internal/fillereval"
)

const (
	TemporalStructureOpenRouterPromptVersion = "filler-temporal-structure-direct-video-v4"
	temporalStructureRoleNone                = "none"
	temporalStructureReasonMaximumCharacters = 512
)

const temporalStructureOpenRouterContentFormat = "Complete video duration: %d milliseconds. Inspect the complete supplied video and return the closed temporal-structure assessment."

const temporalStructureOpenRouterSystemPrompt = `Classify the temporal structure of one complete identity-blind video. Judge the boundaries of the supplied file, not whether its topic resembles an advertisement.

unit definitions:
- standalone: exactly one independently bounded, self-contained inserted item with one cohesive communicative purpose. It may contain many shots or scenes when they belong to that one item.
- compilation: two or more separately bounded items joined in the supplied file. A montage or multiple shots inside one cohesive item is not a compilation.
- programme_excerpt: material whose beginning or ending depends on a larger programme, including an ordinary scene, programme opening, sustained performance, credits/title fragment, or an interior cut from a longer programme.
- programme_with_spots: programme material with one or more inserted, independently bounded filler items. Return every programme and filler interval separately.
- unusable: corruption or degradation prevents reliable temporal assessment. Age, poor image quality, or recording overlays alone are not enough while the structure remains assessable.
- unclear: the complete video is available but structural evidence remains genuinely insufficient to choose.

Return decisive timestamps in milliseconds. For a compilation, include the observed internal item joins. For a programme excerpt, include evidence at or near the dependent start/end edges. For standalone, include the moments that establish independent framing. Use at most eight sorted unique timestamps.

Also return segments covering every millisecond from 0 through the supplied duration: sorted, adjacent, no gaps, no overlaps. Give every segment its own role: commercial, promo, bumper, psa, station_id, trailer, interstitial, programme_fragment, non_filler, ambiguous, or unusable. A compilation's items may have different roles. Never copy one role across the file merely because the source appears commercial. Each segment cites decisive timestamps inside its own interval. Use ambiguous rather than guessing; use unusable only for materially unassessable media.

role applies only when unit is standalone. Choose commercial, promo, bumper, psa, station_id, trailer, interstitial, or unclear. For every non-standalone unit return role=none, an empty roleDecisiveAtMs array, and an empty roleReason.

Do not infer source identity, do not mention filenames or aliases, and do not claim content suitability. Return only the requested JSON.`

type temporalStructureOpenRouterWire struct {
	Unit             string                                   `json:"unit"`
	UnitDecisiveAtMS []int64                                  `json:"unitDecisiveAtMs"`
	UnitReason       string                                   `json:"unitReason"`
	Role             string                                   `json:"role"`
	RoleDecisiveAtMS []int64                                  `json:"roleDecisiveAtMs"`
	RoleReason       string                                   `json:"roleReason"`
	Segments         []temporalStructureOpenRouterSegmentWire `json:"segments"`
}

type temporalStructureOpenRouterSegmentWire struct {
	StartMS      int64   `json:"startMs"`
	EndMS        int64   `json:"endMs"`
	Role         string  `json:"role"`
	DecisiveAtMS []int64 `json:"decisiveAtMs"`
	Reason       string  `json:"reason"`
}

func temporalStructureOpenRouterSchema(durationMS int64) map[string]any {
	units := []string{
		string(fillereval.UnitStandalone), string(fillereval.UnitCompilation), string(fillereval.UnitProgrammeExcerpt), string(fillereval.UnitProgrammeSpots),
		string(fillereval.UnitUnusable), string(fillereval.UnitUnclear),
	}
	roles := []string{
		temporalStructureRoleNone, string(fillereval.TemporalRoleCommercial), string(fillereval.TemporalRolePromo),
		string(fillereval.TemporalRoleBumper), string(fillereval.TemporalRolePSA), string(fillereval.TemporalRoleStationID),
		string(fillereval.TemporalRoleTrailer), string(fillereval.TemporalRoleInterstitial), string(fillereval.TemporalRoleUnclear),
	}
	segmentRoles := []string{
		string(fillereval.TemporalSegmentCommercial), string(fillereval.TemporalSegmentPromo),
		string(fillereval.TemporalSegmentBumper), string(fillereval.TemporalSegmentPSA),
		string(fillereval.TemporalSegmentStationID), string(fillereval.TemporalSegmentTrailer),
		string(fillereval.TemporalSegmentInterstitial), string(fillereval.TemporalSegmentProgrammeFragment),
		string(fillereval.TemporalSegmentNonFiller), string(fillereval.TemporalSegmentAmbiguous),
		string(fillereval.TemporalSegmentUnusable),
	}
	times := map[string]any{"type": "array", "maxItems": temporalStructureMaximumDecisiveTimes, "items": map[string]any{"type": "integer", "minimum": 0, "maximum": durationMS}}
	segment := map[string]any{
		"type": "object", "additionalProperties": false,
		"required": []string{"startMs", "endMs", "role", "decisiveAtMs", "reason"},
		"properties": map[string]any{
			"startMs":      map[string]any{"type": "integer", "minimum": 0, "maximum": durationMS},
			"endMs":        map[string]any{"type": "integer", "minimum": 1, "maximum": durationMS},
			"role":         map[string]any{"type": "string", "enum": segmentRoles},
			"decisiveAtMs": times,
			"reason":       map[string]any{"type": "string", "maxLength": temporalStructureReasonMaximumCharacters},
		},
	}
	return map[string]any{
		"type": "object", "additionalProperties": false,
		"required": []string{"unit", "unitDecisiveAtMs", "unitReason", "role", "roleDecisiveAtMs", "roleReason", "segments"},
		"properties": map[string]any{
			"unit":             map[string]any{"type": "string", "enum": units},
			"unitDecisiveAtMs": times,
			"unitReason":       map[string]any{"type": "string", "maxLength": temporalStructureReasonMaximumCharacters},
			"role":             map[string]any{"type": "string", "enum": roles},
			"roleDecisiveAtMs": times,
			"roleReason":       map[string]any{"type": "string", "maxLength": temporalStructureReasonMaximumCharacters},
			"segments":         map[string]any{"type": "array", "minItems": 1, "maxItems": 128, "items": segment},
		},
	}
}

func temporalStructureOpenRouterContent(durationMS int64) string {
	return fmt.Sprintf(temporalStructureOpenRouterContentFormat, durationMS)
}

func normalizeTemporalStructureOpenRouterWire(wire *temporalStructureOpenRouterWire) {
	sort.Slice(wire.UnitDecisiveAtMS, func(i, j int) bool { return wire.UnitDecisiveAtMS[i] < wire.UnitDecisiveAtMS[j] })
	sort.Slice(wire.RoleDecisiveAtMS, func(i, j int) bool { return wire.RoleDecisiveAtMS[i] < wire.RoleDecisiveAtMS[j] })
	for index := range wire.Segments {
		sort.Slice(wire.Segments[index].DecisiveAtMS, func(i, j int) bool {
			return wire.Segments[index].DecisiveAtMS[i] < wire.Segments[index].DecisiveAtMS[j]
		})
	}
	sort.Slice(wire.Segments, func(i, j int) bool { return wire.Segments[i].StartMS < wire.Segments[j].StartMS })
}

func validateTemporalStructureOpenRouterWire(wire temporalStructureOpenRouterWire, durationMS int64) error {
	unit := fillereval.UnitKind(wire.Unit)
	if !validTemporalStructureUnit(unit) || strings.TrimSpace(wire.UnitReason) == "" || utf8.RuneCountInString(wire.UnitReason) > temporalStructureReasonMaximumCharacters || !validTemporalStructureTimes(wire.UnitDecisiveAtMS, durationMS, unit == fillereval.UnitUnclear) {
		return fmt.Errorf("direct-video structure unit claim is invalid")
	}
	if unit == fillereval.UnitStandalone {
		role := fillereval.TemporalRole(wire.Role)
		if !validHumanRole(role) || strings.TrimSpace(wire.RoleReason) == "" || utf8.RuneCountInString(wire.RoleReason) > temporalStructureReasonMaximumCharacters || !validTemporalStructureTimes(wire.RoleDecisiveAtMS, durationMS, role == fillereval.TemporalRoleUnclear) {
			return fmt.Errorf("direct-video standalone role claim is invalid")
		}
	} else if wire.Role != temporalStructureRoleNone || len(wire.RoleDecisiveAtMS) != 0 || wire.RoleReason != "" {
		return fmt.Errorf("direct-video non-standalone claim carries role output")
	}
	if !slices.IsSorted(wire.UnitDecisiveAtMS) || !slices.IsSorted(wire.RoleDecisiveAtMS) {
		return fmt.Errorf("direct-video decisive timestamps are not sorted")
	}
	assessment := temporalStructureAssessmentFromWire("validation", wire, time.Unix(1, 0), fillereval.TemporalInferenceCall{Axis: "structure", Attempt: 1, ResponseSHA256: strings.Repeat("a", 64)})
	if err := validateTemporalStructureSegments(assessment, durationMS); err != nil {
		return fmt.Errorf("direct-video segment plan is invalid: %w", err)
	}
	return nil
}

func temporalStructureAssessmentFromWire(alias string, wire temporalStructureOpenRouterWire, assessedAt time.Time, call fillereval.TemporalInferenceCall) TemporalStructureAssessment {
	unitTimes := slices.Clone(wire.UnitDecisiveAtMS)
	sort.Slice(unitTimes, func(i, j int) bool { return unitTimes[i] < unitTimes[j] })
	assessment := TemporalStructureAssessment{
		Alias:     alias,
		Unit:      &TemporalStructureUnitClaim{Kind: fillereval.UnitKind(wire.Unit), DecisiveAtMS: unitTimes, Reason: strings.TrimSpace(wire.UnitReason)},
		Inference: temporalInferenceFromCalls(assessedAt, []fillereval.TemporalInferenceCall{call}),
	}
	if assessment.Unit.Kind == fillereval.UnitStandalone {
		roleTimes := slices.Clone(wire.RoleDecisiveAtMS)
		sort.Slice(roleTimes, func(i, j int) bool { return roleTimes[i] < roleTimes[j] })
		assessment.Role = &TemporalStructureRoleClaim{Kind: fillereval.TemporalRole(wire.Role), DecisiveAtMS: roleTimes, Reason: strings.TrimSpace(wire.RoleReason)}
	}
	for _, segment := range wire.Segments {
		assessment.Segments = append(assessment.Segments, TemporalStructureSegmentClaim{
			StartMS: segment.StartMS, EndMS: segment.EndMS, Role: fillereval.TemporalSegmentRole(segment.Role),
			DecisiveAtMS: slices.Clone(segment.DecisiveAtMS), Reason: strings.TrimSpace(segment.Reason),
		})
	}
	return assessment
}

func temporalStructureOpenRouterPromptSHA256() string {
	// A sentinel duration makes the dynamic user message and schema generator
	// part of the prompt identity while each request digest binds its real
	// duration and video bytes.
	contract := struct {
		Version      string         `json:"version"`
		System       string         `json:"system"`
		Content      string         `json:"content"`
		SchemaName   string         `json:"schemaName"`
		Schema       map[string]any `json:"schema"`
		MaxTokens    int            `json:"maxTokens"`
		RequestTitle string         `json:"requestTitle"`
	}{
		Version: TemporalStructureOpenRouterPromptVersion, System: temporalStructureOpenRouterSystemPrompt,
		Content: temporalStructureOpenRouterContent(1), SchemaName: temporalStructureOpenRouterSchemaName,
		Schema: temporalStructureOpenRouterSchema(1), MaxTokens: temporalStructureOpenRouterMaxTokens,
		RequestTitle: temporalStructureOpenRouterTitle,
	}
	raw, err := json.Marshal(contract)
	if err != nil {
		panic(err)
	}
	return hashBytes(raw)
}
