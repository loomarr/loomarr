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
	TemporalStructureOpenRouterPromptVersion = "filler-temporal-structure-direct-video-v6"
	temporalStructureReasonMaximumCharacters = 512
)

const temporalStructureOpenRouterContentFormat = "Complete video duration: %d milliseconds. Inspect the complete supplied video and return the closed temporal-structure assessment."

const temporalStructureOpenRouterSystemPrompt = `Segment one complete identity-blind video. Judge the supplied file's actual item boundaries, not whether its topic resembles an advertisement.

Return segments covering every millisecond from 0 through the supplied duration: sorted, adjacent, with no gaps or overlaps. Keep one independently bounded, self-contained item with one cohesive communicative purpose as one segment even when it contains many shots, scenes, title cards, or credits. Split whenever separately bounded items have been joined. For programme material with an inserted filler item, return the programme before the insertion, every inserted item, and the resumed programme as separate intervals. A scene change inside one continuing programme is not an item boundary.

Give every segment its own role: commercial, promo, bumper, psa, station_id, trailer, interstitial, programme_fragment, non_filler, ambiguous, or unusable. Use programme_fragment for material whose beginning or ending depends on a larger programme, including an ordinary scene, programme opening, sustained performance, credits/title fragment, or interior cut. Never copy one role across the file merely because the source appears commercial. Use ambiguous rather than guessing. Use unusable only when corruption or degradation prevents reliable temporal assessment; age, poor image quality, or recording overlays alone are insufficient.

Each segment must cite up to eight sorted, unique decisive timestamps inside its own interval and give a concise reason for its boundary and role. Do not infer source identity, mention filenames or aliases, or claim content suitability. Return only the requested JSON.`

type temporalStructureOpenRouterWire struct {
	Segments []temporalStructureOpenRouterSegmentWire `json:"segments"`
}

type temporalStructureOpenRouterSegmentWire struct {
	StartMS      int64   `json:"startMs"`
	EndMS        int64   `json:"endMs"`
	Role         string  `json:"role"`
	DecisiveAtMS []int64 `json:"decisiveAtMs"`
	Reason       string  `json:"reason"`
}

func temporalStructureOpenRouterSchema(durationMS int64) map[string]any {
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
		"required": []string{"segments"},
		"properties": map[string]any{
			"segments": map[string]any{"type": "array", "minItems": 1, "maxItems": 128, "items": segment},
		},
	}
}

func temporalStructureOpenRouterContent(durationMS int64) string {
	return fmt.Sprintf(temporalStructureOpenRouterContentFormat, durationMS)
}

func normalizeTemporalStructureOpenRouterWire(wire *temporalStructureOpenRouterWire) {
	for index := range wire.Segments {
		sort.Slice(wire.Segments[index].DecisiveAtMS, func(i, j int) bool {
			return wire.Segments[index].DecisiveAtMS[i] < wire.Segments[index].DecisiveAtMS[j]
		})
	}
	sort.Slice(wire.Segments, func(i, j int) bool { return wire.Segments[i].StartMS < wire.Segments[j].StartMS })
	wire.Segments = coalesceTemporalStructureProgrammeObservations(wire.Segments)
}

func coalesceTemporalStructureProgrammeObservations(segments []temporalStructureOpenRouterSegmentWire) []temporalStructureOpenRouterSegmentWire {
	coalesced := make([]temporalStructureOpenRouterSegmentWire, 0, len(segments))
	for _, segment := range segments {
		if len(coalesced) == 0 {
			coalesced = append(coalesced, segment)
			continue
		}
		previous := &coalesced[len(coalesced)-1]
		if previous.Role != string(fillereval.TemporalSegmentProgrammeFragment) || segment.Role != previous.Role || previous.EndMS != segment.StartMS {
			coalesced = append(coalesced, segment)
			continue
		}
		previous.EndMS = segment.EndMS
		previous.DecisiveAtMS = boundedTemporalStructureTimes(append(slices.Clone(previous.DecisiveAtMS), segment.DecisiveAtMS...))
		previous.Reason = "adjacent programme observations form one continuous programme interval"
	}
	return coalesced
}

func boundedTemporalStructureTimes(values []int64) []int64 {
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	values = slices.Compact(values)
	if len(values) <= temporalStructureMaximumDecisiveTimes {
		return values
	}
	bounded := make([]int64, 0, temporalStructureMaximumDecisiveTimes)
	for index := range temporalStructureMaximumDecisiveTimes {
		at := index * (len(values) - 1) / (temporalStructureMaximumDecisiveTimes - 1)
		bounded = append(bounded, values[at])
	}
	return bounded
}

func validateTemporalStructureOpenRouterWire(wire temporalStructureOpenRouterWire, durationMS int64) error {
	assessment := temporalStructureAssessmentFromWire("validation", wire, time.Unix(1, 0), fillereval.TemporalInferenceCall{Axis: "structure", Attempt: 1, ResponseSHA256: strings.Repeat("a", 64)})
	if err := validateTemporalStructureSegments(assessment, durationMS); err != nil {
		return fmt.Errorf("direct-video segment plan is invalid: %w", err)
	}
	if assessment.Unit == nil || !validTemporalStructureUnit(assessment.Unit.Kind) || strings.TrimSpace(assessment.Unit.Reason) == "" || utf8.RuneCountInString(assessment.Unit.Reason) > temporalStructureReasonMaximumCharacters || !validTemporalStructureTimes(assessment.Unit.DecisiveAtMS, durationMS, temporalStructureUnitMayLackDecisiveEvidence(assessment.Unit.Kind)) {
		return fmt.Errorf("derived direct-video structure unit claim is invalid")
	}
	if assessment.Role != nil && (!validHumanRole(assessment.Role.Kind) || strings.TrimSpace(assessment.Role.Reason) == "" || utf8.RuneCountInString(assessment.Role.Reason) > temporalStructureReasonMaximumCharacters || !validTemporalStructureTimes(assessment.Role.DecisiveAtMS, durationMS, assessment.Role.Kind == fillereval.TemporalRoleUnclear)) {
		return fmt.Errorf("derived direct-video standalone role claim is invalid")
	}
	return nil
}

func temporalStructureAssessmentFromWire(alias string, wire temporalStructureOpenRouterWire, assessedAt time.Time, call fillereval.TemporalInferenceCall) TemporalStructureAssessment {
	assessment := TemporalStructureAssessment{
		Alias:     alias,
		Inference: temporalInferenceFromCalls(assessedAt, []fillereval.TemporalInferenceCall{call}),
	}
	for _, segment := range wire.Segments {
		assessment.Segments = append(assessment.Segments, TemporalStructureSegmentClaim{
			StartMS: segment.StartMS, EndMS: segment.EndMS, Role: fillereval.TemporalSegmentRole(segment.Role),
			DecisiveAtMS: slices.Clone(segment.DecisiveAtMS), Reason: strings.TrimSpace(segment.Reason),
		})
	}
	assessment.Unit, assessment.Role = deriveTemporalStructureWholeFileClaim(assessment.Segments)
	return assessment
}

func deriveTemporalStructureWholeFileClaim(segments []TemporalStructureSegmentClaim) (*TemporalStructureUnitClaim, *TemporalStructureRoleClaim) {
	if len(segments) == 0 {
		return &TemporalStructureUnitClaim{Kind: fillereval.UnitUnclear, Reason: "no complete segment timeline was returned"}, nil
	}
	if len(segments) == 1 {
		segment := segments[0]
		evidence := slices.Clone(segment.DecisiveAtMS)
		switch {
		case temporalStructureFillerSegmentRole(segment.Role):
			role := fillereval.TemporalRole(segment.Role)
			return &TemporalStructureUnitClaim{Kind: fillereval.UnitStandalone, DecisiveAtMS: evidence, Reason: "one independently bounded filler interval covers the complete video"}, &TemporalStructureRoleClaim{Kind: role, DecisiveAtMS: slices.Clone(evidence), Reason: segment.Reason}
		case segment.Role == fillereval.TemporalSegmentProgrammeFragment:
			return &TemporalStructureUnitClaim{Kind: fillereval.UnitProgrammeExcerpt, DecisiveAtMS: evidence, Reason: "one programme fragment covers the complete video"}, nil
		case segment.Role == fillereval.TemporalSegmentUnusable:
			return &TemporalStructureUnitClaim{Kind: fillereval.UnitUnusable, DecisiveAtMS: evidence, Reason: segment.Reason}, nil
		default:
			return &TemporalStructureUnitClaim{Kind: fillereval.UnitUnclear, DecisiveAtMS: evidence, Reason: "one whole-video interval does not establish a filler or programme unit"}, nil
		}
	}
	boundaries := make([]int64, 0, min(len(segments)-1, temporalStructureMaximumDecisiveTimes))
	for index := 1; index < len(segments) && len(boundaries) < temporalStructureMaximumDecisiveTimes; index++ {
		boundaries = append(boundaries, segments[index].StartMS)
	}
	programmeSegments, fillerSegments, unsupportedSegments := 0, 0, 0
	for _, segment := range segments {
		switch {
		case segment.Role == fillereval.TemporalSegmentProgrammeFragment:
			programmeSegments++
		case temporalStructureFillerSegmentRole(segment.Role):
			fillerSegments++
		default:
			unsupportedSegments++
		}
	}
	if programmeSegments >= 2 && fillerSegments >= 1 && unsupportedSegments == 0 && segments[0].Role == fillereval.TemporalSegmentProgrammeFragment && segments[len(segments)-1].Role == fillereval.TemporalSegmentProgrammeFragment {
		return &TemporalStructureUnitClaim{Kind: fillereval.UnitProgrammeSpots, DecisiveAtMS: boundaries, Reason: "programme fragments surround independently bounded filler intervals"}, nil
	}
	if programmeSegments == 0 {
		return &TemporalStructureUnitClaim{Kind: fillereval.UnitCompilation, DecisiveAtMS: boundaries, Reason: "multiple independently bounded non-programme intervals cover the complete video"}, nil
	}
	return &TemporalStructureUnitClaim{Kind: fillereval.UnitUnclear, DecisiveAtMS: boundaries, Reason: "the mixed segment timeline does not establish programme surrounding inserted filler"}, nil
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
