package storeadapter

import (
	"context"

	"github.com/loomarr/loomarr/internal/filler/enrichment"
	"github.com/loomarr/loomarr/internal/filler/records"
	"github.com/loomarr/loomarr/internal/filler/research"
	"github.com/loomarr/loomarr/internal/filler/taxonomy"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
)

// Enrichment bridges the store → the enrichment package's repository seams.
type Enrichment struct{ st records.Store }

// NewEnrichment returns the enrichment repository over st.
func NewEnrichment(st records.Store) Enrichment { return Enrichment{st: st} }

func (r Enrichment) ListCandidates(ctx context.Context, producer, producerVersion, taxonomyVersion string, limit int) ([]enrichment.Candidate, error) {
	clips, err := r.st.ListFillerEnrichmentCandidates(ctx, producer, producerVersion, taxonomyVersion, limit)
	return enrichmentCandidates(clips, err)
}

func (r Enrichment) ListCapabilityCandidates(ctx context.Context, producer, producerVersion, taxonomyVersion string, limit int) ([]enrichment.Candidate, error) {
	clips, err := r.st.ListFillerEnrichmentCapabilityCandidates(ctx, producer, producerVersion, taxonomyVersion, limit)
	return enrichmentCandidates(clips, err)
}

func enrichmentCandidates(clips []clipstore.Clip, err error) ([]enrichment.Candidate, error) {
	if err != nil {
		return nil, err
	}
	out := make([]enrichment.Candidate, len(clips))
	for i, clip := range clips {
		out[i] = enrichment.Candidate{ClipHash: clip.Hash, Path: clip.Path, Name: clip.Name,
			Kind: string(clip.Role), Transcript: clip.Transcript, VisibleText: clip.VisibleText}
	}
	return out, nil
}

func (r Enrichment) ListStates(ctx context.Context, clipHash string) ([]enrichment.State, error) {
	return r.st.ListFillerEnrichment(ctx, clipHash)
}

func (r Enrichment) ListTaxa(ctx context.Context) ([]taxonomy.Taxon, error) {
	return r.st.ListTaxa(ctx)
}

func (r Enrichment) ApplyPass(ctx context.Context, pass enrichment.Pass) (int, error) {
	return r.st.ApplyFillerEnrichmentPass(ctx, pass)
}

// Research bridges the store → the research package's repository seam.
type Research struct {
	st records.FillerResearchStore
}

// NewResearch returns the research repository over st.
func NewResearch(st records.FillerResearchStore) Research { return Research{st: st} }

func (r Research) ListCandidates(ctx context.Context, producer, producerVersion,
	adapter, adapterVersion string, limit int) ([]research.Candidate, error) {
	return r.st.ListFillerResearchCandidates(ctx, producer, producerVersion, adapter, adapterVersion, limit)
}

func (r Research) SaveReport(ctx context.Context, report research.Report) error {
	return r.st.SaveFillerResearchReport(ctx, report)
}

func (r Research) PromoteStoredCountries(ctx context.Context, limit int) (int, error) {
	return r.st.PromoteStoredFillerResearchCountries(ctx, limit)
}
