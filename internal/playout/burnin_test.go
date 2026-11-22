package playout

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/media/encode"
	"github.com/loomarr/loomarr/internal/testkit/execfixture"
)

func testBurnTrack() *BurnIn {
	return &BurnIn{Width: 1920, Height: 1080, Cues: []BurnCue{
		{At: 1 * time.Second, Still: "c00001.nut"},
		{At: 4 * time.Second, Still: burnInBlank},
		{At: 5 * time.Second, Still: "c00002.nut"},
		{At: 9 * time.Second, Still: burnInBlank},
	}}
}

func testBurn() *SubtitleBurn {
	return &SubtitleBurn{List: "/data/subtitle-burn/abc/list-1.ffconcat", Width: 1920, Height: 1080, Kernel: "/data/watermarks/blend-bug-0123abcd.cl"}
}

// The list starts with the still ON SCREEN at the seek, held a lead longer (the graph shifts it
// back), then each change for exactly as long as it shows; the last still is held.
func TestBurnInList(t *testing.T) {
	for _, c := range []struct {
		name string
		seek time.Duration
		want string
	}{
		{"mid-cue", 2500 * time.Millisecond,
			"file c00001.nut\nduration 2.500\nfile blank.nut\nduration 1.000\nfile c00002.nut\nduration 4.000\nfile blank.nut\n"},
		{"before the first cue", 0,
			"file blank.nut\nduration 2.000\nfile c00001.nut\nduration 3.000\nfile blank.nut\nduration 1.000\nfile c00002.nut\nduration 4.000\nfile blank.nut\n"},
		{"exactly on a change", 5 * time.Second,
			"file c00002.nut\nduration 5.000\nfile blank.nut\n"},
		{"after the last cue", time.Minute, "file blank.nut\n"},
	} {
		got := strings.TrimPrefix(testBurnTrack().List(c.seek), "ffconcat version 1.0\n")
		if got != c.want {
			t.Errorf("%s: list\n%s\nwant\n%s", c.name, got, c.want)
		}
	}
}

func TestBurnInWriteListAndLoad(t *testing.T) {
	dir := t.TempDir()
	b := testBurnTrack()
	b.Dir = dir
	if err := finishIndex(dir, b); err != nil {
		t.Fatal(err)
	}
	loaded, ok := LoadBurnIn(dir)
	if !ok || loaded.Width != 1920 || len(loaded.Cues) != 4 || loaded.Cues[2].At != 5*time.Second || loaded.Dir != dir {
		t.Fatalf("LoadBurnIn = %+v, %v", loaded, ok)
	}
	path, remove, err := loaded.WriteList(2500 * time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(path) != dir || !safeGraphPath.MatchString(path) {
		t.Errorf("list at %q: beside the stills, and safe in a filter graph", path)
	}
	remove()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("list not removed: %v", err)
	}
	if _, ok := LoadBurnIn(t.TempDir()); ok {
		t.Error("an empty directory loaded as a track")
	}
}

// finishIndex writes only the index, as finishStills does once the stills exist.
func finishIndex(dir string, b *BurnIn) error {
	for _, c := range b.Cues {
		if err := os.WriteFile(filepath.Join(dir, c.Still), nil, 0o644); err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "c00000.nut"), nil, 0o644); err != nil {
		return err
	}
	return finishStills(dir, b)
}

// Both GPU families blend the subtitles where the bug is blended, under it, from the list shifted
// back by its lead. NVENC crops overlay_cuda's aligned rows before the passthrough scale.
func TestBuildItemBurning_IsAGPUOverlay(t *testing.T) {
	src := testSources()["h264-1080p-sdr-25"]
	for hostName, host := range testHosts() {
		if host.Family != encode.FamilyVAAPI && host.Family != encode.FamilyNVENC {
			continue
		}
		for _, bug := range []*Watermark{nil, testBug()} {
			name := hostName
			if bug != nil {
				name += "+bug"
			}
			p, err := BuildItemBurning(overlayHost(host), src, testOutput, bug, testBurn())
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if !p.Subtitles || p.Watermark != (bug != nil) {
				t.Errorf("%s: subtitles %v watermark %v; fallbacks %v", name, p.Subtitles, p.Watermark, p.Fallbacks)
				continue
			}
			g := p.VideoFilter
			source := "movie=filename=/data/subtitle-burn/abc/list-1.ffconcat:format_name=concat:format_opts=safe=0,setpts=PTS-1.000/TB," + conformColour
			switch host.Family {
			case encode.FamilyNVENC:
				wantAll(t, name, g, source+",hwupload_cuda[sb];[main][sb]overlay_cuda=eof_action=repeat,crop=w=1920:h=1080:x=0:y=0",
					",scale_cuda=w=1920:h=1080:format=yuv420p:passthrough=0,fps=25,")
				if bug != nil {
					wantAll(t, name, g, "[sub][wm]overlay_cuda=")
				}
			case encode.FamilyVAAPI:
				wantAll(t, name, g, source+",split[ssc][ssa];[ssc]format=nv12,",
					"[main][sw][sa]program_opencl=source=/data/watermarks/blend-bug-0123abcd.cl:kernel=blend_bug:inputs=3",
					",hwmap=derive_device=vaapi:reverse=1,format=vaapi,fps=25,")
				if bug != nil {
					wantAll(t, name, g, "inputs=3[sub];", "[sub][wm][wa]program_opencl=")
				}
			}
		}
	}
}

// The canvas is fitted into the output as its own frame: a 1080p track on a 720p output is scaled,
// and a 4:3 canvas on 16:9 is pillarboxed with transparent bars.
func TestBuildItemBurning_FitsTheCanvas(t *testing.T) {
	nv := overlayHost(testHosts()["nvenc-opencl"])
	src := testSources()["h264-1080p-sdr-25"]
	out := testOutput
	out.Width, out.Height = 1280, 720
	p, err := BuildItemBurning(nv, src, out, nil, testBurn())
	if err != nil || !strings.Contains(p.VideoFilter, "setpts=PTS-1.000/TB,scale=w=1280:h=720,"+conformColour) {
		t.Errorf("720p output: %v\n%s", err, p.VideoFilter)
	}
	burn := testBurn()
	burn.Width, burn.Height = 720, 576
	p, err = BuildItemBurning(nv, src, testOutput, nil, burn)
	if err != nil || !strings.Contains(p.VideoFilter, "scale=w=1350:h=1080,pad=w=1920:h=1080:x=284:y=0:color=black@0,") {
		t.Errorf("4:3 canvas: %v\n%s", err, p.VideoFilter)
	}
}

// A host that cannot blend drops the track and says why; it never refuses the programme.
func TestBuildItemBurning_DropsWhereTheGPUCannot(t *testing.T) {
	src := testSources()["h264-1080p-sdr-25"]
	hdr := testOutput
	hdr.HDR, hdr.HEVC = true, true
	noKernel := testBurn()
	noKernel.Kernel = ""
	for _, c := range []struct {
		name string
		host encode.HostProfile
		out  OutputProfile
		burn *SubtitleBurn
	}{
		{"software", overlayHost(testHosts()["software"]), testOutput, testBurn()},
		{"videotoolbox", overlayHost(testHosts()["videotoolbox"]), testOutput, testBurn()},
		{"self-check failed", testHosts()["nvenc-opencl"], testOutput, testBurn()},
		{"vaapi without a kernel", overlayHost(testHosts()["vaapi-intel"]), testOutput, noKernel},
		{"hdr10 output", overlayHost(testHosts()["nvenc-opencl"]), hdr, testBurn()},
	} {
		p, err := BuildItemBurning(c.host, src, c.out, nil, c.burn)
		if err != nil {
			t.Fatalf("%s: refused: %v", c.name, err)
		}
		if p.Subtitles || strings.Contains(p.VideoFilter, "movie=") || !hasFallback(p, "subtitles") {
			t.Errorf("%s: subtitles %v, fallbacks %v\n%s", c.name, p.Subtitles, p.Fallbacks, p.VideoFilter)
		}
	}
}

func TestIsBurnableSubtitle(t *testing.T) {
	if !IsBurnableSubtitle("hdmv_pgs_subtitle") || IsBurnableSubtitle("dvd_subtitle") || IsBurnableSubtitle("subrip") {
		t.Error("only PGS burns in")
	}
}

// The decoder's states become cues: a state that ends before the next begins is cleared at its
// end, one shown again reuses its still, a clear state is the blank, and a still that is not the
// canvas is refused.
func TestWriteStills_StatesBecomeCues(t *testing.T) {
	dir := t.TempDir()
	still := func(name string, w, h int) string {
		path := filepath.Join(dir, name)
		f, err := os.Create(path)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = f.Close() }()
		if err := png.Encode(f, image.NewNRGBA(image.Rect(0, 0, w, h))); err != nil {
			t.Fatal(err)
		}
		return path
	}
	a, b := still("a.png", 64, 36), still("b.png", 64, 36)
	build := t.TempDir()
	got, err := writeStills(build, dir, SubtitleStills{Width: 64, Height: 36, States: []SubtitleState{
		{StartMs: 5000, EndMs: 9000, PNG: b},
		{StartMs: 1000, EndMs: 4000, PNG: a},
		{StartMs: 9000, EndMs: 9500, PNG: a},
		{StartMs: 9500, PNG: ""},
	}})
	if err != nil {
		t.Fatal(err)
	}
	want := []BurnCue{
		{At: time.Second, Still: "c00001.nut"},
		{At: 4 * time.Second, Still: burnInBlank},
		{At: 5 * time.Second, Still: "c00002.nut"},
		{At: 9 * time.Second, Still: "c00001.nut"},
		{At: 9500 * time.Millisecond, Still: burnInBlank},
	}
	if fmt.Sprint(got.Cues) != fmt.Sprint(want) || got.Width != 64 || got.Height != 36 {
		t.Errorf("cues %+v\nwant %+v", got.Cues, want)
	}
	// Slot 0 is the blank canvas, which ffmpeg draws; Go writes only the two distinct stills.
	pngs, _ := filepath.Glob(filepath.Join(build, "s*.png"))
	if len(pngs) != 2 || filepath.Base(pngs[0]) != "s00001.png" {
		t.Errorf("sequence %v: want s00001.png and s00002.png, with s00000.png left for ffmpeg", pngs)
	}
	if _, err := writeStills(t.TempDir(), dir, SubtitleStills{Width: 64, Height: 36, States: []SubtitleState{
		{StartMs: 0, EndMs: 10, PNG: still("small.png", 32, 18)},
	}}); err == nil {
		t.Error("a still smaller than the canvas was accepted")
	}
	// A link inside the decoder's directory that resolves outside it is refused.
	outside := t.TempDir()
	if err := os.Rename(still("escape.png", 64, 36), filepath.Join(outside, "escape.png")); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.png")
	if err := os.Symlink(filepath.Join(outside, "escape.png"), link); err != nil {
		t.Fatal(err)
	}
	if _, err := writeStills(t.TempDir(), dir, SubtitleStills{Width: 64, Height: 36, States: []SubtitleState{
		{StartMs: 0, EndMs: 10, PNG: link},
	}}); err == nil {
		t.Error("a still linked from outside the decoder's directory was read")
	}
}

// The decoder's result is held to its ceilings before anything is allocated for it.
func TestCheckStills_HoldsTheDecodersCeilings(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "a.png")
	ok := func(s SubtitleStills) SubtitleStills {
		if s.Width == 0 && s.Height == 0 {
			s.Width, s.Height = 1920, 1080
		}
		return s
	}
	if err := checkStills(dir, ok(SubtitleStills{Width: 3840, Height: 2160, States: []SubtitleState{{PNG: in}, {PNG: ""}}})); err != nil {
		t.Errorf("a 4K canvas with a still in the directory: %v", err)
	}
	if err := checkStills(dir, ok(SubtitleStills{States: make([]SubtitleState, burnInMaxStates)})); err != nil {
		t.Errorf("exactly %d states: %v", burnInMaxStates, err)
	}
	for name, s := range map[string]SubtitleStills{
		"odd width":     {Width: 1921, Height: 1080},
		"odd height":    {Width: 1920, Height: 1081},
		"zero":          {Width: 0, Height: 1080},
		"negative":      {Width: -2, Height: 1080},
		"too wide":      {Width: 3842, Height: 1080},
		"too tall":      {Width: 1920, Height: 2162},
		"too many":      ok(SubtitleStills{States: make([]SubtitleState, burnInMaxStates+1)}),
		"sibling":       ok(SubtitleStills{States: []SubtitleState{{PNG: filepath.Join(filepath.Dir(dir), "x.png")}}}),
		"dot-dot":       ok(SubtitleStills{States: []SubtitleState{{PNG: dir + "/../x.png"}}}),
		"relative":      ok(SubtitleStills{States: []SubtitleState{{PNG: "a.png"}}}),
		"the dir":       ok(SubtitleStills{States: []SubtitleState{{PNG: dir}}}),
		"prefix-shared": ok(SubtitleStills{States: []SubtitleState{{PNG: dir + "x/a.png"}}}),
	} {
		if err := checkStills(dir, s); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// The track is copied out of its source by stream index, hashed as it is written, and refused
// past the limit or when empty.
func TestExtractTrack_CopiesHashesAndCaps(t *testing.T) {
	dir := t.TempDir()
	argv := filepath.Join(dir, "argv")
	r := burnRunner{ffmpeg: execfixture.POSIX(t, "ffmpeg", `echo "$@" > `+argv+`
head -c "${BYTES:-4096}" /dev/zero`)}
	out := filepath.Join(dir, "track.sup")
	sum, err := extractTrack(t.Context(), r, "/media/film.mkv", 3, out, 4096)
	if err != nil {
		t.Fatal(err)
	}
	want := sha256.Sum256(make([]byte, 4096))
	if sum != hex.EncodeToString(want[:]) {
		t.Errorf("sha256 %s, want the written bytes' %x", sum, want)
	}
	if fi, err := os.Stat(out); err != nil || fi.Size() != 4096 {
		t.Errorf("track file %v %v, want 4096 bytes", fi, err)
	}
	if raw, _ := os.ReadFile(argv); !strings.Contains(string(raw), "-i /media/film.mkv -map 0:3 -c:s copy -f sup pipe:1") {
		t.Errorf("ffmpeg ran with %q, want stream 3 copied as sup", raw)
	}
	if _, err := extractTrack(t.Context(), r, "/media/film.mkv", 3, out, 1024); err == nil || !strings.Contains(err.Error(), "over") {
		t.Errorf("a track over the limit: %v, want refused", err)
	}
	t.Setenv("BYTES", "0")
	if _, err := extractTrack(t.Context(), r, "/media/film.mkv", 3, out, 4096); err == nil {
		t.Error("an empty track was accepted")
	}
}

// A decoder that cannot decode (none, a nil pointer, the refusing stand-in) reads as unavailable,
// so a build without one looks nothing up for burn-in.
func TestSubtitleDecoderAvailable(t *testing.T) {
	var nilPtr *availabilityDecoder
	for _, c := range []struct {
		name string
		d    SubtitleStillDecoder
		want bool
	}{
		{"nil", nil, false},
		{"refusing stand-in", UnavailableSubtitleDecoder{}, false},
		{"refusing stand-in pointer", &UnavailableSubtitleDecoder{}, false},
		{"typed nil", nilPtr, false},
		{"a decoder", &availabilityDecoder{}, true},
	} {
		if got := SubtitleDecoderAvailable(c.d); got != c.want {
			t.Errorf("%s: available = %v, want %v", c.name, got, c.want)
		}
	}
}

type availabilityDecoder struct{}

func (*availabilityDecoder) Decode(context.Context, SubtitleTrack) (SubtitleStills, error) {
	return SubtitleStills{}, nil
}
