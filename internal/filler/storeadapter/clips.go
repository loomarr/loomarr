// Package storeadapter bridges the store's filler rows to the ports the filler packages declare.
//
// Each adapter translates a store type into the domain type a filler seam speaks (a query row into
// a `filler.SweepableProposal`; a clip row is `clipstore.Clip` on both sides) and keeps the few rules
// that belong to the translation itself, such as which clips a work list includes and which store
// error means which domain error. A method the seam takes exactly as the store has it is the
// store's own, promoted through an embedded role. The composition root builds one adapter per seam.
package storeadapter

import (
	"context"
	"errors"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/filler/records"
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
func listWorkClips(ctx context.Context, st clipLister, f filler.ClipQuery) ([]clipstore.Clip, error) {
	return st.ListClips(ctx, clipstore.ClipFilter{IncludeHeld: f.IncludeHeld})
}

type clipReader interface {
	GetClip(ctx context.Context, id string) (clipstore.Clip, error)
}

// findClip turns a missing clip into the found=false the filler seams speak.
func findClip(ctx context.Context, st clipReader, id string) (clipstore.Clip, bool, error) {
	c, err := st.GetClip(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return clipstore.Clip{}, false, nil
	}
	if err != nil {
		return clipstore.Clip{}, false, err
	}
	return c, true, nil
}

// Sync bridges the store's clip methods → filler.Store (the sync).
type Sync struct {
	syncRows
	st clipReader
}

// NewSync returns the sync's clip store over st.
func NewSync(st records.Store) Sync { return Sync{syncRows: st, st: st} }

// GetClip reports an absent clip as not found rather than as an error.
func (a Sync) GetClip(ctx context.Context, id string) (clipstore.Clip, bool, error) {
	return findClip(ctx, a.st, id)
}

// PipelineClips bridges the store → the CLIP-facing seams the ingest pipeline needs
// (§10 V51b): `filler.ClipStore` for the runner, plus the probe and transcode rungs' writers.
//
// ⚠ The PIPELINE table itself needs no adapter — `store.Store` satisfies `filler.PipelineStore`
// directly, because those methods already speak `pipelinestate.ClipPipeline`.
type PipelineClips struct {
	pipelineClipRows
	st clipReader
}

// NewPipelineClips returns the pipeline's clip store over st.
func NewPipelineClips(st records.Store) PipelineClips {
	return PipelineClips{pipelineClipRows: st, st: st}
}

// GetClip reports an absent clip as not found rather than as an error.
func (a PipelineClips) GetClip(ctx context.Context, id string) (clipstore.Clip, bool, error) {
	return findClip(ctx, a.st, id)
}

// AiringClips lists the clips on air for the library re-encode (#2121): the default
// filter leaves out held, removed and composite clips.
type AiringClips struct{ st clipLister }

// NewAiringClips returns the re-encode's clip lister over st.
func NewAiringClips(st records.Store) AiringClips { return AiringClips{st: st} }

// ListClips is the on-air list unless the query asks for held clips too.
func (a AiringClips) ListClips(ctx context.Context, f filler.ClipQuery) ([]clipstore.Clip, error) {
	return listWorkClips(ctx, a.st, f)
}

// Language bridges the store → filler.LanguageClipStore (the language gate, V40).
type Language struct {
	languageRows
	st clipLister
}

// NewLanguage returns the language gate's clip store over st.
func NewLanguage(st languageStore) Language { return Language{languageRows: st, st: st} }

// ListClips is the job's work list: held candidates in, removed clips out (listWorkClips).
func (a Language) ListClips(ctx context.Context, f filler.ClipQuery) ([]clipstore.Clip, error) {
	return listWorkClips(ctx, a.st, f)
}

// Transcribe bridges the store → filler.TranscribeClipStore (§10 V44). Its work list
// has the language gate's polarity (listWorkClips).
type Transcribe struct {
	transcribeRows
	st clipLister
}

// NewTranscribe returns the transcribe rung's clip store over st.
func NewTranscribe(st transcribeStore) Transcribe { return Transcribe{transcribeRows: st, st: st} }

// ListClips is the rung's work list (listWorkClips).
func (a Transcribe) ListClips(ctx context.Context, f filler.ClipQuery) ([]clipstore.Clip, error) {
	return listWorkClips(ctx, a.st, f)
}

// Vision bridges the store → filler.VisionClipStore (§10 V44). Same list polarity as
// the transcribe adapter — held candidates in, removed clips out.
type Vision struct {
	visionRows
	st clipLister
}

// NewVision returns the vision rung's clip store over st.
func NewVision(st records.Store) Vision { return Vision{visionRows: st, st: st} }

// ListClips is the rung's work list (listWorkClips).
func (a Vision) ListClips(ctx context.Context, f filler.ClipQuery) ([]clipstore.Clip, error) {
	return listWorkClips(ctx, a.st, f)
}
