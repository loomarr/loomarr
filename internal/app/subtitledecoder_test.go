package app

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/loomarr/loomarr/internal/media/images/rustgen"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/testkit"
)

func TestSubtitleWorkerDecoder_RefusesUntilTheWorkerIsBound(t *testing.T) {
	var d subtitleWorkerDecoder
	d.bind(nil)
	_, err := d.Decode(context.Background(), playout.SubtitleTrack{Codec: rustgen.CodecPGS})
	if !errors.Is(err, playout.ErrSubtitleDecoderUnavailable) {
		t.Fatalf("unbound decode error = %v, want ErrSubtitleDecoderUnavailable", err)
	}
}

func TestSubtitleWorkerDecoder_DecodesATrackThroughTheImageWorker(t *testing.T) {
	path, err := testkit.RustImageWorker()
	if err != nil {
		t.Fatal(err)
	}
	worker, err := rustgen.Open(path, rustgen.Contract{
		Protocol: 1, Release: "dev", Recipe: "loomarr-rendition-v2",
		RequiredFormats: []string{"avif", "jpeg", "webp"}, Animation: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	var track testkit.PGSTrack
	track.Show(1000, 100, 900)
	track.Clear(4000)
	track.Show(6000, 200, 900) // the last still holds until the programme ends
	dir := t.TempDir()
	sup := filepath.Join(dir, "track.sup")
	if err := os.WriteFile(sup, track.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	staging := filepath.Join(dir, "stills")
	if err := os.Mkdir(staging, 0o700); err != nil {
		t.Fatal(err)
	}

	var d subtitleWorkerDecoder
	d.bind(worker)
	stills, err := d.Decode(context.Background(), playout.SubtitleTrack{
		Sup: sup, SHA256: fmt.Sprintf("%x", sha256.Sum256(track.Bytes())), Codec: rustgen.CodecPGS, Dir: staging,
	})
	if err != nil {
		t.Fatal(err)
	}
	if stills.Width != 1920 || stills.Height != 1080 || len(stills.States) != 3 {
		t.Fatalf("stills = %dx%d with %d states, want 1920x1080 with a still, a clear and a last still", stills.Width, stills.Height, len(stills.States))
	}
	shown, cleared, last := stills.States[0], stills.States[1], stills.States[2]
	if shown.StartMs != 1000 || shown.EndMs != 4000 || cleared.StartMs != 4000 || cleared.EndMs != 6000 || cleared.PNG != "" {
		t.Fatalf("states = %+v, want a still from 1s to 4s, then a clear until 6s", stills.States)
	}
	// The last state has no successor: its end equals its start, which burn-in holds to the end.
	if last.StartMs != 6000 || last.EndMs != 6000 || last.PNG == "" || last.PNG == shown.PNG {
		t.Fatalf("last state = %+v, want its own still at 6s held to the end", last)
	}
	for _, still := range []string{shown.PNG, last.PNG} {
		if filepath.Dir(still) != staging {
			t.Fatalf("still %q is not an absolute path inside the track's directory %q", still, staging)
		}
		if info, err := os.Stat(still); err != nil || !info.Mode().IsRegular() {
			t.Fatalf("still %q: %v", still, err)
		}
	}
}
