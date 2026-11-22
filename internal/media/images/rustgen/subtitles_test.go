package rustgen_test

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/loomarr/loomarr/internal/media/images/rustgen"
	"github.com/loomarr/loomarr/internal/testkit"
)

// supWriter builds a .sup stream the way a Blu-ray authoring tool lays it out.
type supWriter struct{ b []byte }

func (w *supWriter) seg(ms int, kind byte, data []byte) {
	h := []byte{'P', 'G', 0, 0, 0, 0, 0, 0, 0, 0, kind, 0, 0}
	binary.BigEndian.PutUint32(h[2:6], uint32(ms*90)) //nolint:gosec // fixture times are small
	binary.BigEndian.PutUint16(h[11:13], uint16(len(data)))
	w.b = append(append(w.b, h...), data...)
}

// show is a display set on a 1920x1080 canvas: palette 0 (index 1 opaque white, 2
// half-transparent black), one object placed at x,y, its rows run-length coded.
func (w *supWriter) show(ms, x, y int, rle []byte, ow, oh int) {
	pcs := []byte{0x07, 0x80, 0x04, 0x38, 0x10, 0, 1, 0x80, 0, 0, 1, 0, 1, 0, 0, 0, 0, 0, 0}
	binary.BigEndian.PutUint16(pcs[15:17], uint16(x)) //nolint:gosec // fixture coordinates
	binary.BigEndian.PutUint16(pcs[17:19], uint16(y)) //nolint:gosec // fixture coordinates
	w.seg(ms, 0x16, pcs)
	w.seg(ms, 0x17, []byte{1, 0, 0, 0, 0, 0, 0, 0, 0, 0})
	w.seg(ms, 0x14, []byte{0, 0, 1, 235, 128, 128, 255, 2, 16, 128, 128, 128})
	ods := []byte{0, 1, 0, 0xC0, 0, 0, 0, 0, 0, 0, 0}
	binary.BigEndian.PutUint16(ods[7:9], uint16(ow))  //nolint:gosec // fixture size
	binary.BigEndian.PutUint16(ods[9:11], uint16(oh)) //nolint:gosec // fixture size
	w.seg(ms, 0x15, append(ods, rle...))
	w.seg(ms, 0x80, nil)
}

func (w *supWriter) clear(ms int) {
	w.seg(ms, 0x16, []byte{0x07, 0x80, 0x04, 0x38, 0x10, 0, 2, 0, 0, 0, 0})
	w.seg(ms, 0x17, []byte{1, 0, 0, 0, 0, 0, 0, 0, 0, 0})
	w.seg(ms, 0x80, nil)
}

// A 4x2 object: row 0 is two transparent then two white; row 1 is a coded run of four half-black.
var twoRows = []byte{0, 0x02, 1, 1, 0, 0, 0, 0x84, 2, 0, 0}

func openSubtitleWorker(t *testing.T) *rustgen.Generator {
	t.Helper()
	worker, err := testkit.RustImageWorker()
	if err != nil {
		t.Fatal(err)
	}
	gen, err := rustgen.Open(worker, rustgen.Contract{
		Protocol: 1, Release: "dev", Recipe: "loomarr-rendition-v2",
		RequiredFormats: []string{"avif", "jpeg", "webp"}, Animation: true,
		RequiredOperations: []string{"decode-subtitles"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return gen
}

func subtitleRequest(t *testing.T, track []byte, codec string) rustgen.SubtitleRequest {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "track.sup")
	if err := os.WriteFile(path, track, 0o600); err != nil {
		t.Fatal(err)
	}
	staging := filepath.Join(dir, "staging")
	if err := os.Mkdir(staging, 0o700); err != nil {
		t.Fatal(err)
	}
	return rustgen.SubtitleRequest{
		RequestID: "track-1", Codec: codec, Path: path,
		ExpectedSHA256: fmt.Sprintf("%x", sha256.Sum256(track)), StagingDir: staging,
	}
}

func TestDecodeSubtitlesDrawsEachDisplaySetOnTheCanvas(t *testing.T) {
	var w supWriter
	w.clear(0) // a stream often opens with an empty epoch
	w.show(1000, 100, 900, twoRows, 4, 2)
	w.clear(4000)
	w.clear(5000)
	req := subtitleRequest(t, w.b, rustgen.CodecPGS)

	stills, err := openSubtitleWorker(t).DecodeSubtitles(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if stills.Width != 1920 || stills.Height != 1080 {
		t.Fatalf("canvas %dx%d, want 1920x1080", stills.Width, stills.Height)
	}
	if len(stills.States) != 2 || stills.States[0] != (rustgen.SubtitleState{StartMS: 1000, EndMS: 4000, File: stills.States[0].File}) ||
		stills.States[0].File == "" || stills.States[1] != (rustgen.SubtitleState{StartMS: 4000, EndMS: 4000}) {
		t.Fatalf("states = %+v, want a still from 1s to 4s, then a clear", stills.States)
	}
	file, err := os.Open(filepath.Join(req.StagingDir, stills.States[0].File))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	img, err := png.Decode(file)
	if err != nil {
		t.Fatal(err)
	}
	white := color.NRGBA{R: 255, G: 255, B: 255, A: 255}
	for _, c := range []struct {
		x, y int
		want color.NRGBA
	}{
		{100, 900, color.NRGBA{}},
		{102, 900, white},
		{103, 900, white},
		{100, 901, color.NRGBA{A: 128}},
		{103, 901, color.NRGBA{A: 128}},
		{104, 900, color.NRGBA{}},
	} {
		if got := color.NRGBAModel.Convert(img.At(c.x, c.y)); got != c.want {
			t.Errorf("pixel (%d,%d) = %v, want %v", c.x, c.y, got, c.want)
		}
	}
}

func TestDecodeSubtitlesReturnsTypedRefusals(t *testing.T) {
	var w supWriter
	w.show(0, 0, 0, twoRows, 4, 2)
	gen := openSubtitleWorker(t)
	for _, tt := range []struct {
		name  string
		track []byte
		codec string
		code  string
	}{
		{"truncated", w.b[:len(w.b)-5], rustgen.CodecPGS, "corrupt_input"},
		{"vobsub", w.b, "dvd_subtitle", "unsupported_input"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := gen.DecodeSubtitles(context.Background(), subtitleRequest(t, tt.track, tt.codec))
			var refusal *rustgen.WorkerError
			if !errors.As(err, &refusal) || refusal.Code != tt.code {
				t.Fatalf("DecodeSubtitles error = %v, want typed %s", err, tt.code)
			}
		})
	}
}

func TestDecodeSubtitlesRejectsAStillOutsideStaging(t *testing.T) {
	track := []byte("track")
	digest := fmt.Sprintf("%x", sha256.Sum256(track))
	worker := testkit.Executable(t, "loomarr-image", fmt.Sprintf(`#!/bin/sh
if [ "$1" = capabilities ]; then
  printf '%%s\n' '{"protocol":1,"release":"test-release","recipe":"loomarr-rendition-v2","formats":["avif","jpeg","webp"],"animation":true,"operations":["decode-subtitles","generate"],"selfTest":true}'
  exit 0
fi
cat >/dev/null
printf '%%s\n' '{"protocol":1,"status":"ok","requestId":"track-1","source":{"codec":"hdmv_pgs_subtitle","sha256":"%s","bytes":5},"width":8,"height":4,"states":[{"startMs":0,"endMs":0,"still":{"relativePath":"../escape.png","bytes":10,"sha256":"%s"}}]}'
`, digest, digest))
	gen, err := rustgen.Open(worker, rustgen.Contract{
		Protocol: 1, Release: "test-release", Recipe: "loomarr-rendition-v2",
		RequiredOperations: []string{"decode-subtitles"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gen.DecodeSubtitles(context.Background(), subtitleRequest(t, track, rustgen.CodecPGS)); err == nil {
		t.Fatal("DecodeSubtitles accepted a still outside staging")
	}
}

func TestAWorkerWithoutTheOperationIsRefused(t *testing.T) {
	worker := testkit.Executable(t, "loomarr-image", `#!/bin/sh
printf '%s\n' '{"protocol":1,"release":"test-release","recipe":"loomarr-rendition-v2","formats":["avif","jpeg","webp"],"animation":true,"operations":["generate"],"selfTest":true}'
`)
	contract := rustgen.Contract{Protocol: 1, Release: "test-release", Recipe: "loomarr-rendition-v2"}
	gen, err := rustgen.Open(worker, contract)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := gen.DecodeSubtitles(context.Background(), subtitleRequest(t, []byte("x"), rustgen.CodecPGS)); err == nil {
		t.Fatal("DecodeSubtitles ran on a worker that does not list the operation")
	}
	contract.RequiredOperations = []string{"decode-subtitles"}
	if _, err := rustgen.Open(worker, contract); err == nil {
		t.Fatal("Open accepted a worker without a required operation")
	}
}
