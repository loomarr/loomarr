package rustgen_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
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
		{"vobsub without its index", w.b, rustgen.CodecVOBSUB, "invalid_request"},
		{"text codec", w.b, "subrip", "unsupported_input"},
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

// vobsubRequest names the worker's ffmpeg-made VOBSUB fixture pair, copied beside a staging
// directory.
func vobsubRequest(t *testing.T) rustgen.SubtitleRequest {
	t.Helper()
	fixture := filepath.Join("..", "..", "..", "..", "rust", "loomarr-image", "testdata", "vobsub")
	files := map[string][]byte{}
	for _, name := range []string{"track.idx", "track.sub"} {
		data, err := os.ReadFile(filepath.Join(fixture, name)) //nolint:gosec // a checked-in fixture
		if err != nil {
			t.Fatal(err)
		}
		files[name] = data
	}
	req := subtitleRequest(t, files["track.sub"], rustgen.CodecVOBSUB)
	idx := filepath.Join(filepath.Dir(req.Path), "track.idx")
	if err := os.WriteFile(idx, files["track.idx"], 0o600); err != nil {
		t.Fatal(err)
	}
	req.Index = &rustgen.SubtitleIndex{Path: idx, ExpectedSHA256: fmt.Sprintf("%x", sha256.Sum256(files["track.idx"]))}
	return req
}

func TestDecodeSubtitlesDecodesAVOBSUBPair(t *testing.T) {
	req := vobsubRequest(t)
	gen := openSubtitleWorker(t)
	stills, err := gen.DecodeSubtitles(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if stills.Width != 720 || stills.Height != 480 {
		t.Fatalf("canvas %dx%d, want 720x480", stills.Width, stills.Height)
	}
	want := []rustgen.SubtitleState{
		{StartMS: 1000, EndMS: 2490, File: "s00000.png"},
		{StartMS: 2490, EndMS: 3000},
		{StartMS: 3000, EndMS: 3990, File: "s00002.png"},
		{StartMS: 3990, EndMS: 3990},
	}
	if !slices.Equal(stills.States, want) {
		t.Fatalf("states = %+v, want %+v", stills.States, want)
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
	for _, c := range []struct {
		x, y int
		want color.NRGBA
	}{
		{100, 400, color.NRGBA{R: 255, G: 255, B: 255, A: 255}},
		{139, 409, color.NRGBA{A: 255}},
		{140, 400, color.NRGBA{}},
	} {
		if got := color.NRGBAModel.Convert(img.At(c.x, c.y)); got != c.want {
			t.Errorf("pixel (%d,%d) = %v, want %v", c.x, c.y, got, c.want)
		}
	}

	// The index is hash-checked like the track, and a PGS track takes none.
	stale := vobsubRequest(t)
	stale.Index.ExpectedSHA256 = fmt.Sprintf("%x", sha256.Sum256([]byte("other")))
	pgs := vobsubRequest(t)
	pgs.Codec = rustgen.CodecPGS
	for name, tt := range map[string]struct {
		req  rustgen.SubtitleRequest
		code string
	}{
		"stale index":       {stale, "source_changed"},
		"PGS with an index": {pgs, "invalid_request"},
	} {
		_, err := gen.DecodeSubtitles(context.Background(), tt.req)
		var refusal *rustgen.WorkerError
		if !errors.As(err, &refusal) || refusal.Code != tt.code {
			t.Errorf("%s: DecodeSubtitles error = %v, want typed %s", name, err, tt.code)
		}
	}
}

func encodePNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// stillJSON declares data as the still at name.
func stillJSON(name string, data []byte) map[string]any {
	return map[string]any{"relativePath": name, "bytes": len(data), "sha256": fmt.Sprintf("%x", sha256.Sum256(data))}
}

func stateJSON(start, end int64, still map[string]any) map[string]any {
	return map[string]any{"startMs": start, "endMs": end, "still": still}
}

// fakeSubtitleWorker answers every decode-subtitles request with result after running the shell
// command after, so a test controls exactly what the adapter must verify.
func fakeSubtitleWorker(t *testing.T, result any, after string) *rustgen.Generator {
	t.Helper()
	body, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	worker := testkit.Executable(t, "loomarr-image", fmt.Sprintf(`#!/bin/sh
if [ "$1" = capabilities ]; then
  printf '%%s\n' '{"protocol":1,"release":"test-release","recipe":"loomarr-rendition-v2","formats":["avif","jpeg","webp"],"animation":true,"operations":["decode-subtitles","generate"],"selfTest":true}'
  exit 0
fi
cat >/dev/null
%s
cat <<'EOF'
%s
EOF
`, after, body))
	gen, err := rustgen.Open(worker, rustgen.Contract{
		Protocol: 1, Release: "test-release", Recipe: "loomarr-rendition-v2",
		RequiredOperations: []string{"decode-subtitles"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return gen
}

func TestDecodeSubtitlesAcceptsOnlyAVerifiedResult(t *testing.T) {
	track := []byte("track bytes")
	// A transparent NRGBA encodes as 8-bit RGBA, the worker's still format.
	rgba := encodePNG(t, image.NewNRGBA(image.Rect(0, 0, 8, 4)))
	tampered := bytes.Clone(rgba)
	tampered[len(tampered)-1] ^= 0xff
	opaque := image.NewNRGBA(image.Rect(0, 0, 8, 4))
	for i := 3; i < len(opaque.Pix); i += 4 {
		opaque.Pix[i] = 0xff
	}
	rgb := encodePNG(t, opaque) // an opaque image encodes as 8-bit RGB
	deep := encodePNG(t, image.NewNRGBA64(image.Rect(0, 0, 8, 4)))
	small := encodePNG(t, image.NewNRGBA(image.Rect(0, 0, 4, 4)))
	shows := func(data []byte) []any {
		return []any{stateJSON(0, 1000, stillJSON("s0.png", data)), stateJSON(1000, 1000, nil)}
	}
	for _, tt := range []struct {
		name   string
		states []any
		// files are written relative to the request's directory, the parent of staging.
		files map[string][]byte
		after string
		want  string
	}{
		{name: "valid", states: shows(rgba), files: map[string][]byte{"staging/s0.png": rgba}},
		{
			// A real, correctly declared PNG sits at the escaped path, so only containment refuses it.
			name:   "outside staging",
			states: []any{stateJSON(0, 0, stillJSON("../escape.png", rgba))},
			files:  map[string][]byte{"escape.png": rgba},
			want:   "unsafe still path",
		},
		{
			name:   "out of order",
			states: []any{stateJSON(1000, 1000, stillJSON("s0.png", rgba)), stateJSON(1000, 1000, nil)},
			files:  map[string][]byte{"staging/s0.png": rgba},
			want:   "invalid timeline",
		},
		{
			name:   "unfolded clear",
			states: []any{stateJSON(0, 1000, nil), stateJSON(1000, 1000, stillJSON("s0.png", rgba))},
			files:  map[string][]byte{"staging/s0.png": rgba},
			want:   "unfolded clear",
		},
		{name: "hash mismatch", states: shows(rgba), files: map[string][]byte{"staging/s0.png": tampered}, want: "hash mismatch"},
		{name: "IHDR size", states: shows(small), files: map[string][]byte{"staging/s0.png": small}, want: "canvas-sized RGBA PNG"},
		{name: "colour type", states: shows(rgb), files: map[string][]byte{"staging/s0.png": rgb}, want: "canvas-sized RGBA PNG"},
		{name: "bit depth", states: shows(deep), files: map[string][]byte{"staging/s0.png": deep}, want: "canvas-sized RGBA PNG"},
		{
			name: "source changed", states: shows(rgba), files: map[string][]byte{"staging/s0.png": rgba},
			after: "printf 'TRACK BYTES' > \"$TRACK\"", want: "changed during decoding",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := subtitleRequest(t, track, rustgen.CodecPGS)
			dir := filepath.Dir(req.Path)
			for name, data := range tt.files {
				if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			result := map[string]any{
				"protocol": 1, "status": "ok", "requestId": req.RequestID,
				"source": map[string]any{"codec": rustgen.CodecPGS, "sha256": req.ExpectedSHA256, "bytes": len(track)},
				"width":  8, "height": 4, "states": tt.states,
			}
			gen := fakeSubtitleWorker(t, result, "TRACK='"+req.Path+"'\n"+tt.after)
			stills, err := gen.DecodeSubtitles(context.Background(), req)
			if tt.want == "" {
				if err != nil || len(stills.States) != 2 || stills.States[0].File != "s0.png" {
					t.Fatalf("DecodeSubtitles = %+v, %v; want the declared still and a clear", stills, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("DecodeSubtitles error = %v, want one containing %q", err, tt.want)
			}
		})
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
