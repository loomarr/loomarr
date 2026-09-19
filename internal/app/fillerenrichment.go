package app

import (
	"context"
	"io/fs"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/fillerenrichment"
	"github.com/loomarr/loomarr/internal/metrics"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/taxonomy"
)

type fillerEnrichmentRepository struct{ st store.Store }

func activeFillerTextSelection(set resolved, recorder *metrics.Recorder) fillerenrichment.TextSelection {
	selection := resolveSelection(set)
	if selection.URL == "" || selection.Model == "" {
		return fillerenrichment.TextSelection{}
	}
	return fillerenrichment.TextSelection{Provider: buildProviderFor(selection, recorder),
		ProviderName: selection.Provider, Model: selection.Model}
}

func (r fillerEnrichmentRepository) ListCandidates(ctx context.Context, producer, producerVersion, taxonomyVersion string, limit int) ([]fillerenrichment.Candidate, error) {
	clips, err := r.st.ListFillerEnrichmentCandidates(ctx, producer, producerVersion, taxonomyVersion, limit)
	if err != nil {
		return nil, err
	}
	out := make([]fillerenrichment.Candidate, len(clips))
	for i, clip := range clips {
		out[i] = fillerenrichment.Candidate{ClipHash: clip.Hash, Path: clip.Path, Name: clip.Name,
			Kind: string(clip.Kind), Transcript: clip.Transcript, VisibleText: clip.VisibleText}
	}
	return out, nil
}

func (r fillerEnrichmentRepository) ListStates(ctx context.Context, clipHash string) ([]fillerenrichment.State, error) {
	return r.st.ListFillerEnrichment(ctx, clipHash)
}

func (r fillerEnrichmentRepository) ListTaxa(ctx context.Context) ([]taxonomy.Taxon, error) {
	return r.st.ListTaxa(ctx)
}

func (r fillerEnrichmentRepository) ApplyPass(ctx context.Context, pass fillerenrichment.Pass) (int, error) {
	return r.st.ApplyFillerEnrichmentPass(ctx, pass)
}

type fillerEnrichmentSignals struct {
	store store.FillerSourceStore
	files fs.FS
}

func (l fillerEnrichmentSignals) Load(ctx context.Context, candidate fillerenrichment.Candidate, observedAt time.Time) (fillerenrichment.Signals, error) {
	signals := fillerenrichment.Signals{ClipHash: candidate.ClipHash, Kind: candidate.Kind,
		Title: candidate.Name, OriginalName: candidate.Name, Transcript: candidate.Transcript,
		VisibleText: candidate.VisibleText, ObservedAt: observedAt}
	metadata, ok := filler.ReadSourceMetadataFS(l.files, candidate.Path)
	if ok {
		signals.Title = metadata.Title
		signals.Description = metadata.Description
		signals.OriginalName = metadata.OriginalName
		signals.UploadDate = metadata.UploadDate
		signals.SourceID = metadata.SourceID
	}
	if signals.SourceID == "" || l.store == nil {
		return signals, nil
	}
	sources, err := l.store.ListFillerSources(ctx)
	if err != nil {
		return fillerenrichment.Signals{}, err
	}
	for _, source := range sources {
		if source.ID != signals.SourceID {
			continue
		}
		geography := source.Geography.Normalize()
		scope := ""
		if geography.Market != "" {
			scope = string(filler.GeographicLocal)
		} else if geography.Country != "" {
			scope = string(filler.GeographicNational)
		}
		signals.Source = fillerenrichment.Geography{Scope: scope, Country: geography.Country, Market: geography.Market}
		break
	}
	return signals, nil
}
