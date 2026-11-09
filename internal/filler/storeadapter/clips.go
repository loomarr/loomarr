// Package storeadapter bridges the store's filler rows to the ports the filler packages declare.
//
// Each adapter translates a store type into the domain type a filler seam speaks (a query row into
// a `filler.SweepableProposal`; a clip row is `clipstore.Clip` on both sides) and keeps the few rules
// that belong to the translation itself, such as which clips a work list includes and which store
// error means which domain error. The composition root builds one per seam.
package storeadapter

import (
	"context"
	"errors"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/filler/asset"
	"github.com/loomarr/loomarr/internal/filler/records"
	"github.com/loomarr/loomarr/internal/filler/research"
	"github.com/loomarr/loomarr/internal/filler/taxonomy"
	"github.com/loomarr/loomarr/internal/media/defects"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
)

// listWorkClips lists the clips a per-clip work pass (language, transcribe, vision) considers.
//
// ⚠ `IncludeHeld` is honoured but `IncludeRemoved` is deliberately NOT set. A held clip is a fine
// candidate — knowing what it says before a human files it is strictly more useful — but a clip
// the gate already tombstoned must not come back round as work: it would be re-detected forever,
// and on the local backend that is ~341s of QEMU per clip per pass, spent to re-learn an answer
// already recorded.
func listWorkClips(ctx context.Context, st store.Store, f filler.ClipQuery) ([]clipstore.Clip, error) {
	return st.ListClips(ctx, clipstore.ClipFilter{IncludeHeld: f.IncludeHeld})
}

// Sync bridges the store's clip methods → filler.Store (the sync).
type Sync struct{ st records.Store }

// NewSync returns the sync's clip store over st.
func NewSync(st records.Store) Sync { return Sync{st: st} }

func (a Sync) UpsertClip(ctx context.Context, c clipstore.Clip) error {
	return a.st.UpsertClip(ctx, c)
}
func (a Sync) GetClip(ctx context.Context, id string) (clipstore.Clip, bool, error) {
	c, err := a.st.GetClip(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return clipstore.Clip{}, false, nil
	}
	if err != nil {
		return clipstore.Clip{}, false, err
	}
	return c, true, nil
}
func (a Sync) LiveClipPaths(ctx context.Context) (map[string]string, error) {
	return a.st.LiveClipPaths(ctx)
}
func (a Sync) DeleteClipsNotIn(ctx context.Context, keep []string) (int, error) {
	return a.st.DeleteClipsNotIn(ctx, keep)
}

// PipelineClips bridges the store → the CLIP-facing seams the ingest pipeline needs
// (§10 V51b): `filler.ClipStore` for the runner, plus the probe and transcode rungs' writers.
//
// ⚠ The PIPELINE table itself needs no adapter — `store.Store` satisfies `filler.PipelineStore`
// directly, because those five methods already speak `pipelinestate.ClipPipeline`. This adapter
// narrows the store to the clip seams the pipeline declares and turns a missing clip into the
// `found=false` the seam speaks.
type PipelineClips struct{ st records.Store }

// NewPipelineClips returns the pipeline's clip store over st.
func NewPipelineClips(st records.Store) PipelineClips { return PipelineClips{st: st} }

func (a PipelineClips) GetClip(ctx context.Context, id string) (clipstore.Clip, bool, error) {
	return Sync(a).GetClip(ctx, id)
}
func (a PipelineClips) UpsertClip(ctx context.Context, c clipstore.Clip) error {
	return Sync(a).UpsertClip(ctx, c)
}
func (a PipelineClips) ReplaceClipIdentity(ctx context.Context, oldHash string, c clipstore.Clip) error {
	return a.st.ReplaceClipIdentity(ctx, oldHash, c)
}
func (a PipelineClips) ReplaceAiringClip(ctx context.Context, oldHash string, c clipstore.Clip, rec defects.Record) error {
	return a.st.ReplaceAiringClip(ctx, oldHash, c, rec)
}
func (a PipelineClips) CommitConditioningPublication(ctx context.Context, publication asset.ConditioningPublication, c clipstore.Clip) error {
	return a.st.CommitConditioningPublication(ctx, publication, c)
}
func (a PipelineClips) SetClipsRemoved(ctx context.Context, paths []string, at time.Time) (int, error) {
	return a.st.SetClipsRemoved(ctx, paths, at)
}
func (a PipelineClips) HoldClips(ctx context.Context, paths []string, at time.Time) (int, error) {
	return a.st.HoldClips(ctx, paths, at)
}
func (a PipelineClips) SetClipComposite(ctx context.Context, hash string, composite bool, at time.Time) error {
	return a.st.SetClipComposite(ctx, hash, composite, at)
}

// AiringClips lists the clips on air for the library re-encode (#2121): the default
// filter leaves out held, removed and composite clips.
type AiringClips struct{ st records.Store }

// NewAiringClips returns the re-encode's clip lister over st.
func NewAiringClips(st records.Store) AiringClips { return AiringClips{st: st} }

func (a AiringClips) ListClips(ctx context.Context, f filler.ClipQuery) ([]clipstore.Clip, error) {
	return listWorkClips(ctx, a.st, f)
}

// Language bridges the store → filler.LanguageClipStore (the language gate, V40).
type Language struct{ st store.Store }

// NewLanguage returns the language gate's clip store over st.
func NewLanguage(st store.Store) Language { return Language{st: st} }

// ListClips is the job's work list: held candidates in, removed clips out (listWorkClips).
func (a Language) ListClips(ctx context.Context, f filler.ClipQuery) ([]clipstore.Clip, error) {
	return listWorkClips(ctx, a.st, f)
}

func (a Language) SetClipLanguage(ctx context.Context, path, language string, at time.Time) error {
	return a.st.SetClipLanguage(ctx, path, language, at)
}

func (a Language) SetClipsRemoved(ctx context.Context, paths []string, at time.Time) (int, error) {
	return a.st.SetClipsRemoved(ctx, paths, at)
}

// Transcribe bridges the store → filler.TranscribeClipStore (§10 V44). Its work list
// has the language gate's polarity (listWorkClips).
type Transcribe struct{ st store.Store }

// NewTranscribe returns the transcribe rung's clip store over st.
func NewTranscribe(st store.Store) Transcribe { return Transcribe{st: st} }

func (a Transcribe) ListClips(ctx context.Context, f filler.ClipQuery) ([]clipstore.Clip, error) {
	return listWorkClips(ctx, a.st, f)
}

func (a Transcribe) SetClipTranscript(ctx context.Context, path, transcript string, at time.Time) error {
	return a.st.SetClipTranscript(ctx, path, transcript, at)
}

// Vision bridges the store → filler.VisionClipStore (§10 V44). Same list polarity as
// the transcribe adapter — held candidates in, removed clips out.
type Vision struct{ st records.Store }

// NewVision returns the vision rung's clip store over st.
func NewVision(st records.Store) Vision { return Vision{st: st} }

func (a Vision) ListClips(ctx context.Context, f filler.ClipQuery) ([]clipstore.Clip, error) {
	return listWorkClips(ctx, a.st, f)
}

func (a Vision) ApplyClipVision(ctx context.Context, hash, path, brand, visibleText string, era int, leaves []string, at time.Time) error {
	return a.st.ApplyClipVision(ctx, hash, path, brand, visibleText, era, leaves, at)
}

func (a Vision) RecordFillerEraObservation(ctx context.Context, clipHash string, era int, source research.ObservationSource, at time.Time) error {
	return a.st.RecordFillerEraObservation(ctx, clipHash, era, source, at)
}

// ListTaxa: the vision tier grounds its category against the taxonomy graph (§10 V45a).
func (a Vision) ListTaxa(ctx context.Context) ([]taxonomy.Taxon, error) {
	return a.st.ListTaxa(ctx)
}
