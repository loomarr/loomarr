package playout

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/media/encode"
	"github.com/loomarr/loomarr/internal/platform/diagnostics"
)

// Image subtitles burned into the picture on the GPU (#2088 step 8). A channel can set a language
// whose image track (PGS) is always drawn into its programmes; a host whose GPU cannot blend it
// airs the programme without it, never on the CPU.
//
// ffmpeg's own route ([0:s:N] into an overlay) re-sends and re-converts the whole canvas on every
// video frame, about a quarter of a core per stream. Instead a track is prepared once: a
// SubtitleStillDecoder turns it into full-canvas stills, which one ffmpeg run converts to one-frame
// ffv1 files that are already yuva420p BT.709, so nothing is converted while airing. Each encoder
// then reads an ffconcat list written for its own seek: the still on screen at that moment first,
// then each change for exactly as long as it shows. Seeking a sparse stream with -ss loses the
// still already on screen, so the list starts from the seek instead (spike: exact on the GeForce
// and the Arc).

// SubtitleTrack is one extracted image subtitle track to decode (docs/design/images.md,
// decode-subtitles): Sup is the absolute path of the track's raw stream, copied out of its source
// by Go, and SHA256 its hex digest; Codec is the source stream's codec (hdmv_pgs_subtitle); Dir is
// the private staging directory the decoder writes its stills into. The preparer removes both.
type SubtitleTrack struct {
	Sup    string
	SHA256 string
	Codec  string
	Dir    string
}

// SubtitleStills is a decoded track: its canvas and its states in time order.
type SubtitleStills struct {
	Width, Height int
	States        []SubtitleState
}

// SubtitleState is the canvas from StartMs until EndMs (source time). PNG is an absolute path to a
// full-canvas still inside the track's Dir, or "" for a clear canvas. An EndMs at or before
// StartMs holds the state until the next one.
type SubtitleState struct {
	StartMs, EndMs int64
	PNG            string
}

// SubtitleStillDecoder decodes an image subtitle track into timed full-canvas stills. Pixel
// decoding belongs to the image worker (docs/design/images.md); playout keeps the timing, the
// lists and the GPU overlay.
type SubtitleStillDecoder interface {
	Decode(ctx context.Context, track SubtitleTrack) (SubtitleStills, error)
}

// ErrSubtitleDecoderUnavailable is a decoder's refusal when this build has no image subtitle
// decoder: programmes air without the track.
var ErrSubtitleDecoderUnavailable = errors.New("image subtitle decoder unavailable")

// UnavailableSubtitleDecoder refuses every track with ErrSubtitleDecoderUnavailable.
type UnavailableSubtitleDecoder struct{}

// Decode refuses.
func (UnavailableSubtitleDecoder) Decode(context.Context, SubtitleTrack) (SubtitleStills, error) {
	return SubtitleStills{}, ErrSubtitleDecoderUnavailable
}

// SubtitleDecoderAvailable reports whether d can decode a track at all: false for nil and for
// UnavailableSubtitleDecoder, so a build without a decoder looks nothing up for burn-in.
func SubtitleDecoderAvailable(d SubtitleStillDecoder) bool {
	_, none := d.(UnavailableSubtitleDecoder)
	return d != nil && !none
}

// BurnIn is one image subtitle track prepared for burning in.
type BurnIn struct {
	// Dir holds the stills and the encoders' lists.
	Dir string `json:"-"`
	// Width and Height are the canvas the track declares (its own frame, usually the disc's
	// 1920x1080 even when the encode is cropped).
	Width  int `json:"width"`
	Height int `json:"height"`
	// Cues are the canvas states in time order: Still is a file in Dir, burnInBlank when nothing shows.
	Cues []BurnCue `json:"cues"`
}

// BurnCue is the canvas from At (source time) until the next cue.
type BurnCue struct {
	At    time.Duration `json:"at"`
	Still string        `json:"still"`
}

const (
	burnInBlank = "blank.nut"
	burnInIndex = "burnin.json"
	// burnInLead starts every list a second before the item's first frame, and the graph shifts it
	// back (setpts): program_opencl drops main frames earlier than a side input's first frame, and a
	// seek can start the main a few frames below zero.
	burnInLead = time.Second
	// The decoder's ceilings (docs/design/images.md, decode-subtitles), held again by Go: the
	// extracted track's size, the canvas, and the number of states. Go checks them before it
	// allocates the blank canvas or reads a still.
	burnInTrackLimit = 256 << 20
	burnInMaxWidth   = 3840
	burnInMaxHeight  = 2160
	burnInMaxStates  = 50_000
)

// IsBurnableSubtitle reports whether a subtitle codec can be burned in. VOBSUB (dvd_subtitle) keeps
// its palette outside the stream and has no decoder yet; its track is dropped like any other image
// track a host cannot draw.
func IsBurnableSubtitle(codec string) bool { return codec == "hdmv_pgs_subtitle" }

// List is the ffconcat script for an encoder that starts at seek: the still showing then, held
// burnInLead longer, then every change. The last still has no duration: the overlay holds it.
func (b *BurnIn) List(seek time.Duration) string {
	var sb strings.Builder
	sb.WriteString("ffconcat version 1.0\n")
	i := sort.Search(len(b.Cues), func(i int) bool { return b.Cues[i].At > seek }) - 1
	still, from, lead := burnInBlank, seek, burnInLead
	if i >= 0 {
		still = b.Cues[i].Still
	}
	for _, next := range b.Cues[i+1:] {
		fmt.Fprintf(&sb, "file %s\nduration %s\n", still, seconds(next.At-from+lead))
		still, from, lead = next.Still, next.At, 0
	}
	fmt.Fprintf(&sb, "file %s\n", still)
	return sb.String()
}

// WriteList writes List(seek) beside the stills for one encoder; remove deletes it when the
// encoder is done.
func (b *BurnIn) WriteList(seek time.Duration) (path string, remove func(), err error) {
	f, err := os.CreateTemp(b.Dir, "list-*.ffconcat")
	if err != nil {
		return "", nil, err
	}
	_, err = f.WriteString(b.List(seek))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		_ = os.Remove(f.Name())
		return "", nil, err
	}
	return f.Name(), func() { _ = os.Remove(f.Name()) }, nil
}

// LoadBurnIn reads a track prepared earlier; ok is false when dir holds none.
func LoadBurnIn(dir string) (*BurnIn, bool) {
	raw, err := os.ReadFile(filepath.Join(dir, burnInIndex))
	if err != nil {
		return nil, false
	}
	var b BurnIn
	if json.Unmarshal(raw, &b) != nil || b.Width <= 0 || b.Height <= 0 {
		return nil, false
	}
	b.Dir = dir
	return &b, true
}

// BurnInPreparer prepares stream (the source's absolute stream index, `0:N`, as the Library lists
// it) of input, whose codec is codec, into dir.
type BurnInPreparer func(ctx context.Context, input string, stream int, codec, dir string) (*BurnIn, error)

// burnRunner runs the preparer's ffmpeg steps under the process observer.
type burnRunner struct {
	ffmpeg   string
	observer *diagnostics.ProcessManager
}

// run runs one ffmpeg step, its standard output into stdout (discarded when nil).
func (r burnRunner) run(ctx context.Context, purpose string, args []string, stdout io.Writer) error {
	cmd := exec.CommandContext(ctx, r.ffmpeg, args...)
	var stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = stdout, &stderr
	spec, _ := diagnostics.ProcessSpecFromContext(ctx)
	spec.Purpose, spec.Executable, spec.Args, spec.Target = purpose, r.ffmpeg, args, "subtitles"
	rec := r.observer.Begin(spec)
	err := cmd.Run()
	if rec != nil {
		if stderr.Len() > 0 {
			rec.RecordOutput(strings.TrimSpace(stderr.String()))
		}
		rec.Finish(diagnostics.ProcessResult{Err: err, Cancelled: ctx.Err() != nil})
	}
	if err != nil {
		return fmt.Errorf("%s: %w: %s", purpose, err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

// errTrackTooLarge is a cappedWriter's refusal past its limit.
var errTrackTooLarge = errors.New("track too large")

// cappedWriter passes at most left bytes to w, then refuses, which stops ffmpeg on a broken pipe.
type cappedWriter struct {
	w    io.Writer
	left int64
	over bool
}

func (c *cappedWriter) Write(p []byte) (int, error) {
	if int64(len(p)) > c.left {
		c.over = true
		return 0, errTrackTooLarge
	}
	c.left -= int64(len(p))
	return c.w.Write(p)
}

// extractTrack copies stream of input out as a raw .sup at path, refusing one larger than limit
// bytes, and returns its SHA-256 in hex.
func extractTrack(ctx context.Context, r burnRunner, input string, stream int, path string, limit int64) (string, error) {
	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	sum := sha256.New()
	capped := &cappedWriter{w: io.MultiWriter(f, sum), left: limit}
	err = r.run(ctx, "subtitle_burn_extract", []string{"-v", "error", "-nostdin", "-i", input,
		"-map", "0:" + strconv.Itoa(stream), "-c:s", "copy", "-f", "sup", "pipe:1"}, capped)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	switch {
	case capped.over:
		return "", fmt.Errorf("prepare burn-in: the track is over %d MiB", limit>>20)
	case err != nil:
		return "", err
	case capped.left == limit:
		return "", errors.New("prepare burn-in: the track is empty")
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}

// checkStills holds a decoded track to the decoder's ceilings before anything is allocated for
// it: an even canvas no larger than 3840x2160, at most burnInMaxStates states, and every still a
// path inside dir, the decoder's staging directory.
func checkStills(dir string, stills SubtitleStills) error {
	if w, h := stills.Width, stills.Height; w <= 0 || h <= 0 || w%2 != 0 || h%2 != 0 || w > burnInMaxWidth || h > burnInMaxHeight {
		return fmt.Errorf("prepare burn-in: canvas %dx%d", w, h)
	}
	if n := len(stills.States); n > burnInMaxStates {
		return fmt.Errorf("prepare burn-in: %d states, over %d", n, burnInMaxStates)
	}
	for _, s := range stills.States {
		if s.PNG == "" {
			continue
		}
		rel, err := filepath.Rel(dir, s.PNG)
		if !filepath.IsAbs(s.PNG) || err != nil || !filepath.IsLocal(rel) || rel == "." {
			return fmt.Errorf("prepare burn-in: still %q is outside the decoder's directory", s.PNG)
		}
	}
	return nil
}

// FFmpegBurnInWith prepares with decoder and ffmpeg: the track copied out as its raw stream
// (reading the whole source, so it runs once per track and the caller caches it), decoded into
// stills, and the stills converted to ffv1 in one run. It builds in a sibling directory and
// renames it into place, so dir is either complete or absent.
func FFmpegBurnInWith(ffmpegPath string, observer *diagnostics.ProcessManager, decoder SubtitleStillDecoder) BurnInPreparer {
	r := burnRunner{ffmpeg: ffmpegPath, observer: observer}
	return func(ctx context.Context, input string, stream int, codec, dir string) (*BurnIn, error) {
		build := dir + ".building"
		_ = os.RemoveAll(build)
		decoded := filepath.Join(build, "decoded")
		if err := os.MkdirAll(decoded, 0o755); err != nil {
			return nil, err
		}
		defer func() { _ = os.RemoveAll(build) }()
		sup := filepath.Join(build, "track.sup")
		sum, err := extractTrack(ctx, r, input, stream, sup, burnInTrackLimit)
		if err != nil {
			return nil, err
		}
		track := SubtitleTrack{Sup: sup, SHA256: sum, Codec: codec, Dir: decoded}
		stills, err := decoder.Decode(ctx, track)
		if err != nil {
			return nil, fmt.Errorf("prepare burn-in: %w", err)
		}
		if err := checkStills(decoded, stills); err != nil {
			return nil, err
		}
		b, err := writeStills(build, decoded, stills)
		if err != nil {
			return nil, err
		}
		_ = os.RemoveAll(decoded)
		_ = os.Remove(sup)
		// One run converts every still: the image2 sequence in, one intra frame per segment out.
		if err := r.run(ctx, "subtitle_burn_convert", []string{"-v", "error", "-nostdin", "-framerate", "1",
			"-i", filepath.Join(build, "s%05d.png"), "-vf", "scale=out_color_matrix=bt709:out_range=tv,format=yuva420p",
			"-c:v", "ffv1", "-g", "1", "-f", "segment", "-segment_time", "0.5", "-reset_timestamps", "1",
			"-segment_format", "nut", filepath.Join(build, "c%05d.nut")}, nil); err != nil {
			return nil, err
		}
		if err := finishStills(build, b); err != nil {
			return nil, err
		}
		_ = os.RemoveAll(dir)
		if err := os.Rename(build, dir); err != nil {
			return nil, err
		}
		b.Dir = dir
		return b, nil
	}
}

// writeStills copies the blank canvas as s00000.png and each distinct still after it, and returns
// the cues naming the ffv1 files the conversion will give them (cNNNNN.nut for sNNNNN.png). A
// state that ends before the next begins is followed by a blank cue at its end. Each still is
// opened inside decoded (os.Root), so a link out of the decoder's directory is refused; the caller
// has held stills to checkStills.
func writeStills(dir, decoded string, stills SubtitleStills) (*BurnIn, error) {
	from, err := os.OpenRoot(decoded)
	if err != nil {
		return nil, fmt.Errorf("prepare burn-in: %w", err)
	}
	defer func() { _ = from.Close() }()
	b := &BurnIn{Width: stills.Width, Height: stills.Height}
	states := append([]SubtitleState(nil), stills.States...)
	sort.SliceStable(states, func(i, j int) bool { return states[i].StartMs < states[j].StartMs })
	n := 0
	next := func() (string, *os.File, error) {
		f, err := os.Create(filepath.Join(dir, fmt.Sprintf("s%05d.png", n)))
		name := fmt.Sprintf("c%05d.nut", n)
		n++
		return name, f, err
	}
	_, blank, err := next()
	if err != nil {
		return nil, err
	}
	// The blank is the one canvas Go allocates, after checkStills bounded it.
	err = png.Encode(blank, image.NewNRGBA(image.Rect(0, 0, stills.Width, stills.Height)))
	if cerr := blank.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return nil, err
	}
	names := map[string]string{}
	cue := func(at int64, still string) {
		c := BurnCue{At: time.Duration(at) * time.Millisecond, Still: still}
		if k := len(b.Cues) - 1; k >= 0 && b.Cues[k].At == c.At {
			b.Cues[k] = c // two states at one instant: the later one shows
			return
		}
		b.Cues = append(b.Cues, c)
	}
	for i, s := range states {
		still := burnInBlank
		if s.PNG != "" {
			if still = names[s.PNG]; still == "" {
				if still, err = copyStill(from, decoded, s.PNG, stills.Width, stills.Height, next); err != nil {
					return nil, err
				}
				names[s.PNG] = still
			}
		}
		cue(s.StartMs, still)
		if still != burnInBlank && s.EndMs > s.StartMs && (i == len(states)-1 || states[i+1].StartMs > s.EndMs) {
			cue(s.EndMs, burnInBlank)
		}
	}
	return b, nil
}

// copyStill copies one decoded still, path inside decoded, into the image2 sequence, refusing one
// that resolves outside root or whose size is not the canvas.
func copyStill(root *os.Root, decoded, path string, width, height int, next func() (string, *os.File, error)) (string, error) {
	rel, err := filepath.Rel(decoded, path)
	if err != nil {
		return "", fmt.Errorf("prepare burn-in: %w", err)
	}
	src, err := root.Open(rel)
	if err != nil {
		return "", fmt.Errorf("prepare burn-in: %w", err)
	}
	defer func() { _ = src.Close() }()
	cfg, err := png.DecodeConfig(src)
	if err != nil {
		return "", fmt.Errorf("prepare burn-in: still %s: %w", filepath.Base(path), err)
	}
	if cfg.Width != width || cfg.Height != height {
		return "", fmt.Errorf("prepare burn-in: still %dx%d on a %dx%d canvas", cfg.Width, cfg.Height, width, height)
	}
	if _, err := src.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	name, dst, err := next()
	if err != nil {
		return "", err
	}
	_, err = io.Copy(dst, src)
	if cerr := dst.Close(); err == nil {
		err = cerr
	}
	return name, err
}

// finishStills names the blank, drops the PNGs, checks every still converted, and writes the index.
func finishStills(dir string, b *BurnIn) error {
	if err := os.Rename(filepath.Join(dir, "c00000.nut"), filepath.Join(dir, burnInBlank)); err != nil {
		return fmt.Errorf("prepare burn-in: no blank still: %w", err)
	}
	pngs, _ := filepath.Glob(filepath.Join(dir, "s*.png"))
	for _, p := range pngs {
		_ = os.Remove(p)
	}
	for _, c := range b.Cues {
		if _, err := os.Stat(filepath.Join(dir, c.Still)); err != nil {
			return fmt.Errorf("prepare burn-in: still %s missing: %w", c.Still, err)
		}
	}
	raw, err := json.Marshal(b)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, burnInIndex), raw, 0o644)
}

// SubtitleBurn is one encoder's subtitle overlay: the list written for its seek (BurnIn.WriteList),
// the track's canvas, and the VAAPI family's blend kernel (WriteBlendKernel).
type SubtitleBurn struct {
	List          string
	Width, Height int
	Kernel        string
}

// burning reports whether this item burns in its subtitles on this host, declaring why not when it
// was asked to. The caller passes a burn only when the host's GPU overlay passed its self-check
// (WatermarkCheck), as it does a bug.
func (b *builder) burning() bool {
	if b.burn == nil {
		return false
	}
	why := ""
	switch {
	case b.host.Family != encode.FamilyNVENC && b.host.Family != encode.FamilyVAAPI:
		why = "no GPU blend in the " + string(b.host.Family) + " family; never drawn on the CPU"
	case b.out.HDR:
		why = "no 10-bit GPU overlay for an HDR10 output yet"
	case !b.host.Overlay:
		why = "the GPU overlay failed this host's self-check"
	case b.burn.Width <= 0 || b.burn.Height <= 0:
		why = fmt.Sprintf("bad canvas %dx%d", b.burn.Width, b.burn.Height)
	case !safeGraphPath.MatchString(b.burn.List):
		why = "the list's path would need escaping in the filter graph"
	case b.host.Family == encode.FamilyVAAPI && (b.burn.Kernel == "" || !safeGraphPath.MatchString(b.burn.Kernel)):
		why = "no usable blend kernel file for the VAAPI overlay"
	}
	if why != "" {
		b.fallback("subtitles", "dropped: "+why)
		return false
	}
	return true
}

// subtitleSource is the stills: the list from the item's seek, shifted back by its lead, and the
// track's canvas fitted into the output as its own frame and centred. The canvas is the disc's
// frame, so a cropped encode's subtitles land in the letterbox, where the disc shows them, instead
// of off the picture. This chain (movie, scale, pad, and the VAAPI kernel's alphaextract) runs on
// the CPU once per still change, never per video frame: each still is one frame the overlay holds
// on the GPU until the next.
func (b *builder) subtitleSource() string {
	s := fmt.Sprintf("movie=filename=%s:format_name=concat:format_opts=safe=0,setpts=PTS-%s/TB", b.burn.List, seconds(burnInLead))
	w, h, _ := fitSize(b.burn.Width, b.burn.Height, b.out.Width, b.out.Height)
	if w != b.burn.Width || h != b.burn.Height {
		s += fmt.Sprintf(",scale=w=%d:h=%d", w, h)
	}
	if w != b.out.Width || h != b.out.Height {
		s += fmt.Sprintf(",pad=w=%d:h=%d:x=%d:y=%d:color=black@0", b.out.Width, b.out.Height, even((b.out.Width-w)/2), even((b.out.Height-h)/2))
	}
	return s + "," + conformColour
}

// kernelSubtitles is the VAAPI kernel's subtitle inputs, built like kernelBug's from each still:
// [sw] its colour in NV12 and [sa] its alpha NV12-shaped, each uploaded to OpenCL once per change.
func (b *builder) kernelSubtitles() string {
	upload := conformColour + ",hwupload=derive_device=opencl,settb=AVTB"
	return b.subtitleSource() + ",split[ssc][ssa];[ssc]format=nv12," + upload + "[sw];" +
		"[ssa]alphaextract,format=gray,split[ssay][ssah];[ssah]scale=w=iw/2:h=ih/2:flags=area,split[ssau][ssav];" +
		"[ssay][ssau][ssav]mergeplanes=map0s=0:map0p=0:map1s=1:map1p=0:map2s=2:map2p=0:format=yuv420p,format=nv12," + upload + "[sa]"
}
