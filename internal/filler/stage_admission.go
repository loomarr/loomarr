package filler

import (
	"context"
	"strconv"
	"strings"

	"github.com/loomarr/loomarr/internal/filleradmission"
	"github.com/loomarr/loomarr/internal/fillerdecision"
)

// AdmissionObserver owns the versioned evidence-to-durable-decision transaction. Production uses
// fillerdecision.Shadow; tests use an in-memory adapter at this same seam.
type AdmissionObserver interface {
	Observe(context.Context, fillerdecision.ShadowObservation) error
}

// AdmissionStage captures the facts the production pipeline can prove before the legacy score
// stage is allowed to file a clip. It never translates scalar confidence or inferred catalog
// fields back into evidence.
type AdmissionStage struct{ observer AdmissionObserver }

func NewAdmissionStage(observer AdmissionObserver) *AdmissionStage {
	return &AdmissionStage{observer: observer}
}

func (s *AdmissionStage) ID() StageID     { return StageAdmission }
func (s *AdmissionStage) Cost() StageCost { return CostCheap }
func (s *AdmissionStage) Applies(context.Context, StoreClip) (bool, string) {
	return true, ""
}

func (s *AdmissionStage) Run(ctx context.Context, clip StoreClip) (StageResult, error) {
	if s == nil || s.observer == nil {
		return StageResult{}, fillerdecision.ErrInvalid
	}
	evidence := []filleradmission.Evidence{{
		ID: "media-usability:probe", Claim: filleradmission.ClaimMediaUsability,
		Value: filleradmission.UsabilityUsable, Kind: filleradmission.KindDecoder,
		Source: "pipeline:probe",
	}}
	if year := EraFromName(clip.Name); year > 0 {
		evidence = append(evidence, filleradmission.Evidence{
			ID: "recording-date:filename", Claim: filleradmission.ClaimRecordingDate,
			Value: strconv.Itoa(year), Kind: filleradmission.KindFilename,
			Source: "clip:original-name", Location: "filename",
		})
	}
	if role := explicitContentRole(clip.Name); role != "" {
		evidence = append(evidence, filleradmission.Evidence{
			ID: "content-role:filename", Claim: filleradmission.ClaimContentRole,
			Value: role, Kind: filleradmission.KindFilename,
			Source: "clip:original-name", Location: "filename",
		})
	}
	if err := s.observer.Observe(ctx, fillerdecision.ShadowObservation{
		ClipHash: clip.Hash, ObservedAt: clip.UpdatedAt, Evidence: evidence,
	}); err != nil {
		return StageResult{}, err
	}
	reportProgress(ctx, StageAdmission, 100)
	return StageResult{Verdict: VerdictContinue}, nil
}

// explicitContentRole refuses KindFromName's useful catalog default. A default is placement policy,
// not evidence; only an explicit token in the original filename can support a V61 claim.
func explicitContentRole(name string) string {
	tokens := strings.FieldsFunc(strings.ToLower(name), func(r rune) bool {
		return r < 'a' || r > 'z'
	})
	for i, token := range tokens {
		switch token {
		case "bumper", "bumpers":
			return filleradmission.RoleBumper
		case "ident", "idents":
			return filleradmission.RoleStationID
		case "station":
			if i+1 < len(tokens) && (tokens[i+1] == "id" || tokens[i+1] == "ident") {
				return filleradmission.RoleStationID
			}
		case "psa", "psas":
			return filleradmission.RolePSA
		case "trailer", "trailers":
			return filleradmission.RoleTrailer
		case "interstitial", "interstitials":
			return filleradmission.RoleInterstitial
		case "commercial", "commercials", "advert", "adverts":
			return filleradmission.RoleCommercial
		}
	}
	return ""
}
