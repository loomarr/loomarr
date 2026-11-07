package proposals

import "encoding/json"

// Scores describes source-backed evidence, not a calibrated probability or ranking.
type Scores struct {
	Version           int           `json:"version" doc:"Assessment contract version; zero means unassessed historical data"`
	ThemeFit          *float64      `json:"themeFit" doc:"Mean distinct qualifier coverage; null when evidence is insufficient"`
	AvailabilityRatio float64       `json:"availabilityRatio" doc:"In-library title share, not playback readiness"`
	EraBalance        *float64      `json:"eraBalance" doc:"Requested date adherence; null when unknown or not requested"`
	Theme             ThemeEvidence `json:"theme"`
	Era               EraEvidence   `json:"era"`
}

// ThemeEvidence keeps the supported interpretation inspectable without treating
// an unmatched word as proof that a grounded title is unsuitable.
type ThemeEvidence struct {
	Status        string              `json:"status" enum:"unassessed,partial,supported"`
	Basis         string              `json:"basis" enum:"qualifiers,named_membership,none"`
	AssessedItems int                 `json:"assessedItems"`
	UnknownItems  int                 `json:"unknownItems"`
	Qualifiers    []QualifierEvidence `json:"qualifiers"`
}

type QualifierEvidence struct {
	Term           string `json:"term"`
	SupportedItems int    `json:"supportedItems"`
}

// EraEvidence separates a missing request from missing catalog/episode dates.
type EraEvidence struct {
	Status        string `json:"status" enum:"not_requested,unassessed,partial,supported"`
	AssessedItems int    `json:"assessedItems"`
	MatchingItems int    `json:"matchingItems"`
	UnknownItems  int    `json:"unknownItems"`
}

// UnmarshalJSON applies the assessment contract at the persisted-domain boundary,
// shared by Proposal lists and Journey responses. Old percentages cannot acquire
// the new meaning merely by being decoded into fields with the same wire names.
func (s *Scores) UnmarshalJSON(data []byte) error {
	type wire Scores
	var decoded wire
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*s = Scores(decoded)
	if s.Version != 1 {
		s.ThemeFit, s.EraBalance = nil, nil
		s.Theme = ThemeEvidence{Status: "unassessed", Basis: "none", Qualifiers: []QualifierEvidence{}}
		s.Era = EraEvidence{Status: "unassessed"}
	}
	return nil
}
