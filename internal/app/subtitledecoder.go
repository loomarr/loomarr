package app

import (
	"context"
	"path/filepath"
	"sync/atomic"

	"github.com/loomarr/loomarr/internal/media/images/rustgen"
	"github.com/loomarr/loomarr/internal/playout"
)

// subtitleWorkerDecoder decodes an image subtitle track through the image worker's
// decode-subtitles operation (docs/design/images.md). The playout build needs a decoder before the
// image worker is certified, so the worker is bound later with bind; unbound, it refuses every
// track with playout.ErrSubtitleDecoderUnavailable and programmes air without their image tracks.
type subtitleWorkerDecoder struct {
	worker atomic.Pointer[rustgen.Generator]
}

// bind gives the decoder the certified image worker. A nil worker (a store-less boot) leaves it
// refusing.
func (d *subtitleWorkerDecoder) bind(worker *rustgen.Generator) {
	if worker != nil {
		d.worker.Store(worker)
	}
}

// Decode asks the worker for the track's stills and returns them with absolute paths inside the
// track's staging directory.
func (d *subtitleWorkerDecoder) Decode(ctx context.Context, track playout.SubtitleTrack) (playout.SubtitleStills, error) {
	worker := d.worker.Load()
	if worker == nil {
		return playout.SubtitleStills{}, playout.ErrSubtitleDecoderUnavailable
	}
	got, err := worker.DecodeSubtitles(ctx, rustgen.SubtitleRequest{
		RequestID: filepath.Base(track.Dir), Codec: track.Codec,
		Path: track.Sup, ExpectedSHA256: track.SHA256, StagingDir: track.Dir,
	})
	if err != nil {
		return playout.SubtitleStills{}, err
	}
	out := playout.SubtitleStills{Width: got.Width, Height: got.Height, States: make([]playout.SubtitleState, len(got.States))}
	for i, state := range got.States {
		out.States[i] = playout.SubtitleState{StartMs: state.StartMS, EndMs: state.EndMS}
		if state.File != "" {
			out.States[i].PNG = filepath.Join(track.Dir, state.File)
		}
	}
	return out, nil
}
