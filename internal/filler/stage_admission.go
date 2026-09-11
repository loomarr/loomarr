package filler

import (
	"context"
	"strconv"

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

// explicitContentRole translates only KindFromName's explicit concrete result into evidence.
// Unclassified is the absence of role authority, never a content-role claim.
func explicitContentRole(name string) string {
	switch KindFromName(name) {
	case Bumper:
		return filleradmission.RoleBumper
	case StationID:
		return filleradmission.RoleStationID
	case PSA:
		return filleradmission.RolePSA
	case Trailer:
		return filleradmission.RoleTrailer
	case Interstitial:
		return filleradmission.RoleInterstitial
	case Commercial:
		return filleradmission.RoleCommercial
	default:
		return ""
	}
}
