package playout

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/encode"
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
