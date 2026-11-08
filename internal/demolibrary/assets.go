package demolibrary

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// AssetVersion stamps the generated tree. Bump it when a generator changes what it draws, so an
// existing demo directory is regenerated instead of mixing old and new files.
const AssetVersion = 5

// Test Card palette (web/packages/design-system/src/tokens/brand-contract.json). Duplicated
// rather than read at runtime because the backend binary does not ship the web tree; the guard
// test pins the two together.
var (
	chroma     = []string{"FFB020", "F5D90A", "3DD68C", "4CC9E8", "D6409F", "E5484D", "8B93A3"}
	ground     = "0B0C0E"
	foreground = "F7F8FA"
)

// segmentSeconds is the length actually encoded per format. The full-length file is this segment
// stream-copied end to end, so a 44-minute film costs a 30-second encode.
const segmentSeconds = 30

// Layout is where each generated file lives under the demo directory.
type Layout struct{ Dir string }

func (l Layout) Video(formatID string) string { return filepath.Join(l.Dir, "video", formatID+".mp4") }
func (l Layout) Filler(id string) string      { return filepath.Join(l.Dir, "filler", id+".mp4") }
func (l Layout) Poster(itemID string) string {
	return filepath.Join(l.Dir, "art", "poster", itemID+".jpg")
}
func (l Layout) Backdrop(itemID string) string {
	return filepath.Join(l.Dir, "art", "backdrop", itemID+".jpg")
}
func (l Layout) Icon(number int) string {
	return filepath.Join(l.Dir, "art", "icon", strconv.Itoa(number)+".png")
}
func (l Layout) Watermark() string { return filepath.Join(l.Dir, "art", "watermark.png") }

// PackProgramme and PackFiller are where the open pack's downloaded files live. They sit under
// media/, which a reset never removes, so switching content modes in one directory does not throw
// away a gigabyte of verified downloads.
func (l Layout) PackProgramme(itemID string) string {
	return filepath.Join(l.Dir, "media", "programmes", itemID+".mp4")
}
func (l Layout) PackFiller(id string) string {
	return filepath.Join(l.Dir, "media", "filler", id+".mp4")
}
func (l Layout) manifest() string { return filepath.Join(l.Dir, "manifest.json") }
func (l Layout) work() string     { return filepath.Join(l.Dir, ".work") }

// Manifest records how every generated file was made. Generated art carries the repository's
// licence; a future AI-art step would add its model and prompt per file here (#1587).
type Manifest struct {
	Version   int             `json:"version"`
	Content   Content         `json:"content"`
	Generator string          `json:"generator"`
	Licence   string          `json:"licence"`
	Files     []ManifestEntry `json:"files"`
}

// ManifestEntry is one generated file's provenance.
type ManifestEntry struct {
	Path   string `json:"path"` // relative to the demo directory
	Source string `json:"source"`
}

// Generate writes every asset lib needs under dir that is not already there. It is idempotent: a
// complete directory at the current AssetVersion and content is left untouched, and a partial one
// (an interrupted run) is finished, because each file is written to a temporary name and renamed.
//
// The invented library generates all of its media. The open pack downloads its pinned files and
// verifies each against its SHA-256 first, then cuts its posters and backdrops from those videos;
// its channel icons and watermark are generated like the invented library's.
func Generate(ctx context.Context, lib *Library, dir, ffmpeg string, log *slog.Logger) error {
	l := Layout{Dir: dir}
	if err := l.resetIfStale(lib.Content); err != nil {
		return err
	}
	for _, sub := range []string{"video", "filler", "art/poster", "art/backdrop", "art/icon", "media/programmes", "media/filler", ".work"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			return err
		}
	}
	m := Manifest{Version: AssetVersion, Content: lib.Content, Generator: "loomarr internal/demolibrary", Licence: "Same licence as the Loomarr repository; no third-party imagery"}
	g := generator{ffmpeg: ffmpeg, l: l, log: log}
	var jobs []job
	if lib.pack == nil {
		for _, f := range Formats {
			jobs = append(jobs, job{l.Video(f.ID), "ffmpeg lavfi SMPTE bars, " + f.Label, func(ctx context.Context, out string) error { return g.video(ctx, f, out) }})
		}
		for i, f := range lib.Fillers {
			jobs = append(jobs, job{l.Filler(f.ID), "ffmpeg lavfi colour card", func(ctx context.Context, out string) error { return g.filler(ctx, f, chroma[i%len(chroma)], out) }})
		}
		for _, t := range lib.Titles {
			jobs = append(jobs,
				job{l.Poster(t.ID()), "ffmpeg typographic poster", func(ctx context.Context, out string) error { return g.poster(ctx, t, out) }},
				job{l.Backdrop(t.ID()), "ffmpeg typographic backdrop", func(ctx context.Context, out string) error { return g.backdrop(ctx, t, out) }})
		}
	} else {
		m.Licence = "Generated files: same licence as the Loomarr repository. Pack media and the frames cut from it: each file's own licence, below"
		d := newDownloader(log, lib.pack.Mirror)
		var total int64
		for _, f := range lib.pack.files() {
			dest := l.PackProgramme(f.itemID)
			if f.filler {
				dest = l.PackFiller(f.itemID)
			}
			if err := d.fetch(ctx, f.media, dest); err != nil {
				return err
			}
			total += f.media.Size
			rel, _ := filepath.Rel(dir, dest)
			m.Files = append(m.Files, ManifestEntry{Path: rel, Source: f.media.URL + " sha256:" + f.media.SHA256 + " " + Licences[f.media.Licence]})
		}
		log.Info("demo pack media verified", "files", len(lib.pack.files()), "bytes", total)
		for _, pt := range lib.pack.Titles {
			t := pt.title()
			src := l.PackProgramme(t.ID())
			if pt.Kind == Series {
				src = l.PackProgramme(episodeID(t, 1))
			}
			art := pt.artMedia()
			source := "ffmpeg frame from " + art.URL + " " + Licences[art.Licence]
			jobs = append(jobs,
				job{l.Poster(t.ID()), source, func(ctx context.Context, out string) error {
					return g.frame(ctx, src, pt.PosterAt, 600, 900, out)
				}},
				job{l.Backdrop(t.ID()), source, func(ctx context.Context, out string) error {
					return g.frame(ctx, src, pt.BackdropAt, 1280, 720, out)
				}})
		}
	}
	for _, c := range lib.Channels {
		jobs = append(jobs, job{l.Icon(c.Number), "ffmpeg typographic channel icon", func(ctx context.Context, out string) error { return g.icon(ctx, c, out) }})
	}
	jobs = append(jobs, job{l.Watermark(), "ffmpeg test-card bars on transparent", g.watermark})

	made := 0
	for _, j := range jobs {
		rel, _ := filepath.Rel(dir, j.out)
		m.Files = append(m.Files, ManifestEntry{Path: rel, Source: j.source})
		if _, err := os.Stat(j.out); err == nil {
			continue
		}
		tmp := tempName(j.out)
		if err := j.run(ctx, tmp); err != nil {
			_ = os.Remove(tmp)
			return fmt.Errorf("generate %s: %w", rel, err)
		}
		if err := os.Rename(tmp, j.out); err != nil {
			return err
		}
		made++
		log.Info("demo asset generated", "file", rel)
	}
	if err := l.linkLibrary(lib); err != nil {
		return err
	}
	log.Info("demo assets ready", "dir", dir, "generated", made, "total", len(jobs))
	raw, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(l.manifest(), raw, 0o644)
}

// tempName keeps the extension last, because ffmpeg picks its muxer from it.
func tempName(out string) string {
	ext := filepath.Ext(out)
	return strings.TrimSuffix(out, ext) + ".partial" + ext
}

// Movies and Shows are the demo's media folders, laid out as a household's are. Each title's
// folder and file names carry its ids, so Loomarr's own scan grounds every title with no arr and
// no search, and plays the files it found (#2151 step 16: playout reads only the Library's
// files). A film names its TMDB id; a show names both, since the stand-in TMDB keys it on the
// same number as its TVDB id: the scan fetches it by TMDB and keys it by TVDB, as the media
// server did.
func (l Layout) Movies() string { return filepath.Join(l.Dir, "library", "movies") }
func (l Layout) Shows() string  { return filepath.Join(l.Dir, "library", "shows") }

// FilmFile and EpisodeFile are one playable item's file, which the stand-in server reports as its
// path too. Each is a hard link to the item's encoded file (an invented title's format file, or
// an open title's download): every item has its own path, as on a real server, at the cost of no
// disk, and the scan, which never follows a symlink, reads it as the file it is.
func (l Layout) FilmFile(t Title) string {
	name := fmt.Sprintf("%s (%d) {tmdb-%d}", fileSafe(t.Name), t.Year, t.ProviderID())
	return filepath.Join(l.Movies(), name, name+".mp4")
}
func (l Layout) EpisodeFile(t Title, e Episode) string {
	show := fmt.Sprintf("%s (%d) {tmdb-%d} {tvdb-%d}", fileSafe(t.Name), t.Year, t.ProviderID(), t.ProviderID())
	return filepath.Join(l.Shows(), show, fmt.Sprintf("Season %02d", e.Season),
		fmt.Sprintf("%s - S%02dE%02d.mp4", fileSafe(t.Name), e.Season, e.Number))
}

// fileSafe replaces the characters a file name can't hold on every platform the demo runs on.
func fileSafe(name string) string {
	return strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) {
			return '-'
		}
		return r
	}, name)
}

// linkLibrary (re)creates the media folders' hard links.
func (l Layout) linkLibrary(lib *Library) error {
	link := func(t Title, itemID, p string) error {
		target := l.PackProgramme(itemID)
		if !t.Open {
			target = l.Video(t.Format)
		}
		want, err := os.Stat(target)
		if err != nil {
			return err
		}
		if cur, err := os.Stat(p); err == nil && os.SameFile(cur, want) {
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		_ = os.Remove(p)
		return os.Link(target, p)
	}
	for _, t := range lib.Titles {
		if t.Kind == Movie {
			if err := link(t, t.ID(), l.FilmFile(t)); err != nil {
				return err
			}
			continue
		}
		for _, e := range lib.EpisodesOf(t) {
			if err := link(t, e.ID, l.EpisodeFile(t, e)); err != nil {
				return err
			}
		}
	}
	return nil
}

// resetIfStale removes generated files left by a different AssetVersion or content mode. The
// open pack's verified downloads under media/ stay: they are pinned, so they are never stale.
func (l Layout) resetIfStale(content Content) error {
	raw, err := os.ReadFile(l.manifest())
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	var m Manifest
	if json.Unmarshal(raw, &m) == nil && m.Version == AssetVersion && m.Content == content {
		return nil
	}
	for _, sub := range []string{"video", "filler", "art", "library", ".work"} {
		if err := os.RemoveAll(filepath.Join(l.Dir, sub)); err != nil {
			return err
		}
	}
	return os.Remove(l.manifest())
}

type job struct {
	out    string
	source string
	run    func(ctx context.Context, out string) error
}

type generator struct {
	ffmpeg string
	l      Layout
	log    *slog.Logger
}

// text writes s to a file drawtext reads with textfile=, which sidesteps filtergraph escaping for
// titles with colons, apostrophes and commas.
func (g generator) text(name, s string) (string, error) {
	p := filepath.Join(g.l.work(), name+".txt")
	return p, os.WriteFile(p, []byte(s), 0o644)
}

func (g generator) run(ctx context.Context, args ...string) error {
	cmd := exec.CommandContext(ctx, g.ffmpeg, append([]string{"-hide_banner", "-loglevel", "error", "-nostdin", "-y"}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w: %s", g.ffmpeg, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func drawtext(textfile string, size int, color, x, y string) string {
	return fmt.Sprintf("drawtext=textfile=%s:fontsize=%d:fontcolor=0x%s:x=%s:y=%s:line_spacing=%d", textfile, size, color, x, y, size/4)
}

// video encodes one format's segment and stream-copies it to the full runtime. The picture is
// SMPTE bars (the Test Card motif) over a band with the format label and a running clock, so a
// playing channel is visibly alive and a black-frame check has real luma to measure.
func (g generator) video(ctx context.Context, f Format, out string) error {
	label, err := g.text("label-"+f.ID, "LOOMARR DEMO\n"+f.Label)
	if err != nil {
		return err
	}
	bars := "smptehdbars"
	if f.Height < 720 {
		bars = "smptebars"
	}
	size := f.Height / 14
	vf := strings.Join([]string{
		fmt.Sprintf("%s=s=%dx%d:r=24:d=%d", bars, f.Width, f.Height, segmentSeconds),
		fmt.Sprintf("drawbox=x=0:y=ih*0.62:w=iw:h=ih*0.38:color=0x%s:t=fill", ground),
		drawtext(label, size, foreground, "(w-tw)/2", "h*0.66"),
		fmt.Sprintf("drawtext=text='%%{pts\\:hms}':fontsize=%d:fontcolor=0x%s:x=(w-tw)/2:y=h*0.88", size*2/3, chroma[0]),
	}, ",")
	args := []string{"-f", "lavfi", "-i", vf, "-f", "lavfi", "-t", strconv.Itoa(segmentSeconds), "-i", "anullsrc=r=48000:cl=stereo"}
	switch {
	case f.HDR10:
		// Convert the SDR bars to real PQ/BT.2020 code values rather than just tagging them, so
		// the tone-mapping path sees a genuine HDR10 signal. lavfi frames carry no colour tags, and
		// zscale finds no conversion path from "unspecified", hence the setparams first.
		args = append(args, "-vf", "setparams=color_primaries=bt709:color_trc=bt709:colorspace=bt709:range=tv,zscale=t=linear:npl=100,format=gbrpf32le,zscale=p=bt2020:t=smpte2084:m=bt2020nc:r=tv,format=yuv420p10le",
			"-c:v", "libx265", "-preset", "fast", "-crf", "30", "-tag:v", "hvc1",
			"-x265-params", "log-level=error:keyint=48:hdr10=1:repeat-headers=1:colorprim=bt2020:transfer=smpte2084:colormatrix=bt2020nc:master-display=G(13250,34500)B(7500,3000)R(34000,16000)WP(15635,16450)L(10000000,1):max-cll=1000,400",
			"-color_primaries", "bt2020", "-color_trc", "smpte2084", "-colorspace", "bt2020nc")
	case f.Codec == "hevc":
		args = append(args, "-c:v", "libx265", "-preset", "fast", "-crf", "30", "-pix_fmt", "yuv420p10le", "-tag:v", "hvc1",
			"-x265-params", "log-level=error:keyint=48")
	default:
		args = append(args, "-c:v", "libx264", "-preset", "veryfast", "-crf", "28", "-tune", "stillimage", "-pix_fmt", "yuv420p", "-g", "48")
	}
	seg := filepath.Join(g.l.work(), f.ID+"-segment.mp4")
	args = append(args, "-c:a", "aac", "-b:a", "96k", "-shortest", seg)
	if err := g.run(ctx, args...); err != nil {
		return err
	}
	defer func() { _ = os.Remove(seg) }()
	loops := (f.Duration+segmentSeconds-1)/segmentSeconds - 1
	return g.run(ctx, "-stream_loop", strconv.Itoa(loops), "-i", seg, "-c", "copy", "-t", strconv.Itoa(f.Duration), "-movflags", "+faststart", out)
}

// filler is a full-colour card with the interstitial's name.
func (g generator) filler(ctx context.Context, f Filler, bg, out string) error {
	name, err := g.text("filler-"+f.ID, strings.ToUpper(f.Name))
	if err != nil {
		return err
	}
	tag, err := g.text("filler-tag", "LOOMARR DEMO · always something on")
	if err != nil {
		return err
	}
	// A block sweeps along the bottom edge, 60 px a frame. The quality gate holds a clip whose
	// picture is unchanged for 2 s (freezedetect -60 dB) for review, so a static card never airs,
	// and a slow or small change stays under that threshold too. It is an overlay because
	// drawbox evaluates its position once, not per frame.
	sweep := fmt.Sprintf("color=c=0x%s:s=1920x1080:r=24:d=%d[bg];color=c=0x%s:s=480x96:r=24:d=%d[blk];"+
		"[bg][blk]overlay=x='mod(t*1440,W+480)-480':y=H-96", bg, f.Duration, ground, f.Duration)
	vf := strings.Join([]string{
		sweep,
		drawtext(name, 140, ground, "(w-tw)/2", "(h-th)/2"),
		drawtext(tag, 48, ground, "(w-tw)/2", "h*0.8"),
	}, ",")
	// A soft major chord, not silence: the filler quality gate rejects a clip whose audio is
	// silent (silent_content), so a silent card never reaches a break.
	chord := fmt.Sprintf("aevalsrc=0.12*sin(2*PI*261.63*t)+0.12*sin(2*PI*329.63*t)+0.12*sin(2*PI*392.00*t):s=48000:c=stereo:d=%d", f.Duration)
	return g.run(ctx, "-f", "lavfi", "-i", vf, "-f", "lavfi", "-i", chord,
		"-c:v", "libx264", "-preset", "veryfast", "-crf", "28", "-tune", "stillimage", "-pix_fmt", "yuv420p", "-g", "48",
		"-c:a", "aac", "-b:a", "96k", "-shortest", "-movflags", "+faststart", out)
}

// barsStrip draws the seven Test Card chroma bars across a band of the frame.
func barsStrip(w, y, h int) []string {
	bw := w / len(chroma)
	out := make([]string, 0, len(chroma))
	for i, c := range chroma {
		width := bw
		if i == len(chroma)-1 {
			width = w - bw*i
		}
		out = append(out, fmt.Sprintf("drawbox=x=%d:y=%d:w=%d:h=%d:color=0x%s:t=fill", bw*i, y, width, h, c))
	}
	return out
}

// accent picks a title's colour from the palette, deterministically.
func accent(t Title) string {
	n := t.N
	if t.Kind == Movie {
		n += 3
	}
	return chroma[n%(len(chroma)-1)] // skip the neutral grey
}

func (g generator) still(ctx context.Context, filters []string, out string) error {
	args := []string{"-f", "lavfi", "-i", strings.Join(filters, ","), "-frames:v", "1"}
	if strings.HasSuffix(out, ".jpg") {
		args = append(args, "-q:v", "4")
	}
	return g.run(ctx, append(args, out)...)
}

// frame cuts one still from a video at second `at`, scaled to cover w×h and centre-cropped: an
// open title's poster (2:3) and backdrop (16:9) are its own pictures.
func (g generator) frame(ctx context.Context, src string, at float64, w, h int, out string) error {
	vf := fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=increase,crop=%d:%d,setsar=1", w, h, w, h)
	return g.run(ctx, "-ss", strconv.FormatFloat(at, 'f', 2, 64), "-i", src, "-frames:v", "1", "-vf", vf, "-q:v", "3", out)
}

// poster is a 2:3 typographic cover: accent field, bars, wrapped title, year and genre.
func (g generator) poster(ctx context.Context, t Title, out string) error {
	title, err := g.text("poster-"+t.ID(), wrap(t.Name, 14))
	if err != nil {
		return err
	}
	meta, err := g.text("poster-meta-"+t.ID(), fmt.Sprintf("%d · %s", t.Year, strings.Join(t.Genres, ", ")))
	if err != nil {
		return err
	}
	f := []string{fmt.Sprintf("color=c=0x%s:s=600x900:d=1", accent(t))}
	f = append(f, barsStrip(600, 0, 60)...)
	f = append(f, drawtext(title, 58, ground, "44", "180"), drawtext(meta, 28, ground, "44", "h-100"))
	return g.still(ctx, f, out)
}

// backdrop is a 16:9 dark card with an accent block and the title.
func (g generator) backdrop(ctx context.Context, t Title, out string) error {
	title, err := g.text("backdrop-"+t.ID(), wrap(t.Name, 24))
	if err != nil {
		return err
	}
	f := []string{fmt.Sprintf("color=c=0x%s:s=1280x720:d=1", ground),
		fmt.Sprintf("drawbox=x=820:y=0:w=460:h=720:color=0x%s:t=fill", accent(t))}
	f = append(f, barsStrip(1280, 680, 40)...)
	f = append(f, drawtext(title, 64, foreground, "64", "(h-th)/2"))
	return g.still(ctx, f, out)
}

// icon is a square channel tile: the channel number over the bars.
func (g generator) icon(ctx context.Context, c Channel, out string) error {
	num, err := g.text("icon-"+strconv.Itoa(c.Number), strconv.Itoa(c.Number))
	if err != nil {
		return err
	}
	f := []string{fmt.Sprintf("color=c=0x%s:s=512x512:d=1", ground)}
	f = append(f, barsStrip(512, 360, 96)...)
	f = append(f, drawtext(num, 200, foreground, "(w-tw)/2", "96"))
	return g.still(ctx, f, out)
}

// watermark is the seven bars with transparent gaps: the alpha is the bug's shape. drawbox does
// not write alpha, so the bars are drawn opaque on black and the black is keyed out afterwards;
// drawing onto a transparent canvas yields an invisible image the playout refuses to air.
func (g generator) watermark(ctx context.Context, out string) error {
	const w, h, gap = 224, 128, 6
	bw := (w + gap) / len(chroma)
	f := []string{fmt.Sprintf("color=c=black:s=%dx%d:d=1", w, h)}
	for i, c := range chroma {
		f = append(f, fmt.Sprintf("drawbox=x=%d:y=0:w=%d:h=%d:color=0x%s:t=fill", bw*i, bw-gap, h, c))
	}
	f = append(f, "format=rgba", "colorkey=color=black:similarity=0.01:blend=0")
	return g.still(ctx, f, out)
}

// wrap breaks a title into lines of at most width characters, on word boundaries.
func wrap(s string, width int) string {
	var lines []string
	line := ""
	for _, w := range strings.Fields(s) {
		switch {
		case line == "":
			line = w
		case len(line)+1+len(w) <= width:
			line += " " + w
		default:
			lines = append(lines, line)
			line = w
		}
	}
	if line != "" {
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
