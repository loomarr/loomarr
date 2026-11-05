package fillerstructurewindow

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/loomarr/loomarr/internal/fillerstructure"
)

// directVideoResponse is the strict wire shape of one window's structured output.
type directVideoResponse struct {
	Segments []directVideoResponseSegment `json:"segments"`
}

type directVideoResponseSegment struct {
	EndMS        int64   `json:"endMs"`
	Role         string  `json:"role"`
	DecisiveAtMS []int64 `json:"decisiveAtMs"`
	Reason       string  `json:"reason"`
}

// ParseDirectVideoResponse strictly decodes window-local provider output and projects its complete
// timeline onto the planned source-relative media interval. Planned geometry, not encoder drift,
// remains the coverage authority.
func ParseDirectVideoResponse(set MediaSet, ordinal int, raw string) ([]fillerstructure.Segment, error) {
	if err := ValidateMediaSet(set); err != nil {
		return nil, err
	}
	if ordinal < 0 || ordinal >= len(set.Plan.Windows) {
		return nil, fmt.Errorf("structure window direct-video ordinal is invalid")
	}
	window := set.Plan.Windows[ordinal]
	durationMS := window.MediaEndMS - window.MediaStartMS
	local, err := decodeDirectVideoResponse(raw, durationMS)
	if err != nil {
		return nil, err
	}
	segments := make([]fillerstructure.Segment, 0, len(local))
	for _, segment := range local {
		segments = append(segments, fillerstructure.Segment{
			StartMS: window.MediaStartMS + segment.StartMS,
			EndMS:   window.MediaStartMS + segment.EndMS,
			Role:    segment.Role,
		})
	}
	return segments, nil
}

// decodeDirectVideoResponse decodes, normalizes and validates one window-local response into a
// complete, contiguous segment timeline over [0, durationMS).
func decodeDirectVideoResponse(raw string, durationMS int64) ([]fillerstructure.Segment, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	var response directVideoResponse
	if err := decoder.Decode(&response); err != nil {
		return nil, fmt.Errorf("decode direct-video response: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, errors.New("decode direct-video response: trailing JSON value")
	}
	for index := range response.Segments {
		slices.Sort(response.Segments[index].DecisiveAtMS)
	}
	response.Segments = coalesceDirectVideoProgramme(response.Segments)
	if durationMS <= 0 || len(response.Segments) == 0 || len(response.Segments) > DirectVideoMaximumSegments {
		return nil, errors.New("direct-video segment plan must cover the complete source")
	}
	segments := make([]fillerstructure.Segment, 0, len(response.Segments))
	startMS := int64(0)
	for index, segment := range response.Segments {
		role := fillerstructure.Role(segment.Role)
		mayBeEmpty := role == fillerstructure.RoleAmbiguous || role == fillerstructure.RoleUnusable
		if segment.EndMS <= startMS || !directVideoRole(role) || strings.TrimSpace(segment.Reason) == "" ||
			!validDirectVideoTimes(segment.DecisiveAtMS, startMS, segment.EndMS, mayBeEmpty) {
			return nil, fmt.Errorf("direct-video segment %d is invalid", index)
		}
		segments = append(segments, fillerstructure.Segment{StartMS: startMS, EndMS: segment.EndMS, Role: role})
		startMS = segment.EndMS
	}
	if startMS != durationMS {
		return nil, errors.New("direct-video segment plan must cover the complete source")
	}
	return segments, nil
}

// coalesceDirectVideoProgramme merges adjacent programme_fragment segments into one interval.
func coalesceDirectVideoProgramme(segments []directVideoResponseSegment) []directVideoResponseSegment {
	result := make([]directVideoResponseSegment, 0, len(segments))
	for _, segment := range segments {
		if len(result) == 0 {
			result = append(result, segment)
			continue
		}
		previous := &result[len(result)-1]
		if previous.Role != string(fillerstructure.RoleProgrammeFragment) || segment.Role != previous.Role || segment.EndMS <= previous.EndMS {
			result = append(result, segment)
			continue
		}
		previous.EndMS = segment.EndMS
		previous.DecisiveAtMS = boundedDirectVideoTimes(append(slices.Clone(previous.DecisiveAtMS), segment.DecisiveAtMS...))
		previous.Reason = "adjacent programme observations form one continuous programme interval"
	}
	return result
}

func boundedDirectVideoTimes(values []int64) []int64 {
	slices.Sort(values)
	values = slices.Compact(values)
	if len(values) <= DirectVideoMaximumDecisiveTime {
		return values
	}
	bounded := make([]int64, 0, DirectVideoMaximumDecisiveTime)
	for index := range DirectVideoMaximumDecisiveTime {
		at := index * (len(values) - 1) / (DirectVideoMaximumDecisiveTime - 1)
		bounded = append(bounded, values[at])
	}
	return bounded
}

func directVideoRole(role fillerstructure.Role) bool {
	return slices.Contains([]fillerstructure.Role{
		fillerstructure.RoleCommercial, fillerstructure.RolePromo, fillerstructure.RoleBumper, fillerstructure.RolePSA,
		fillerstructure.RoleStationID, fillerstructure.RoleTrailer, fillerstructure.RoleInterstitial,
		fillerstructure.RoleProgrammeFragment, fillerstructure.RoleNonFiller, fillerstructure.RoleAmbiguous,
		fillerstructure.RoleUnusable,
	}, role)
}

func validDirectVideoTimes(values []int64, startMS, endMS int64, mayBeEmpty bool) bool {
	if len(values) == 0 {
		return mayBeEmpty
	}
	if len(values) > DirectVideoMaximumDecisiveTime || !slices.IsSorted(values) {
		return false
	}
	previous := int64(-1)
	for _, value := range values {
		if value < startMS || value > endMS || value == previous {
			return false
		}
		previous = value
	}
	return true
}
