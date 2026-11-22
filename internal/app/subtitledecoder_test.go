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
	if stills.Width != 1920 || stills.Height != 1080 || len(stills.States) != 2 {
		t.Fatalf("stills = %dx%d with %d states, want 1920x1080 with a still then a clear", stills.Width, stills.Height, len(stills.States))
	}
	shown, cleared := stills.States[0], stills.States[1]
	if shown.StartMs != 1000 || shown.EndMs != 4000 || cleared.StartMs != 4000 || cleared.PNG != "" {
		t.Fatalf("states = %+v, want a still from 1s to 4s, then a clear", stills.States)
	}
	if filepath.Dir(shown.PNG) != staging {
		t.Fatalf("still %q is not an absolute path inside the track's directory %q", shown.PNG, staging)
	}
	if info, err := os.Stat(shown.PNG); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("still %q: %v", shown.PNG, err)
	}
}
