package playout

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/loomarr/loomarr/internal/media/encode"
	"github.com/loomarr/loomarr/internal/playout/packager"
)

// slateEncode is one (host, output) slate, encoding or encoded; done closes when s or err is set.
type slateEncode struct {
	done chan struct{}
	s    *packager.Slate
	err  error
}

// slateEncodeTimeout bounds one slate encode (0.8 s cold on NVENC live).
const slateEncodeTimeout = time.Minute

// slate returns the house slate for an encode as a SlateSource: the packager waits on it only when a
// slot needs slate (#1512 G2: never on the tune path; the first manifest waits for a real item
// anyway). The encode starts on that first need or, when prewarmed, once the channel is on air.
func (m *PackagerHLS) slate(host encode.HostProfile, out OutputProfile) packager.SlateSource {
	return func(ctx context.Context) (*packager.Slate, error) {
		e := m.slateEncodeFor(host, out)
		select {
		case <-e.done:
			return e.s, e.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

// slateEncodeFor returns the (host, output) slate encode, starting it in the background if none is
// running or done. Encodes are shared across channels and sessions; a failed one is forgotten, so
// the next need encodes it again.
func (m *PackagerHLS) slateEncodeFor(host encode.HostProfile, out OutputProfile) *slateEncode {
	key := fmt.Sprintf("%+v|%+v", host, out)
	m.slateMu.Lock()
	e := m.slates[key]
	if e == nil {
		e = &slateEncode{done: make(chan struct{})}
		m.slates[key] = e
		go func() {
			defer close(e.done)
			ctx, cancel := context.WithTimeout(m.life, slateEncodeTimeout)
			defer cancel()
			t0 := time.Now()
			if e.s, e.err = m.encodeSlate(ctx, host, out); e.err != nil {
				m.log.Error("packager hls: slate encode failed", "err", e.err)
				m.slateMu.Lock()
				if m.slates[key] == e {
					delete(m.slates, key)
				}
				m.slateMu.Unlock()
				return
			}
			m.log.Info("packager hls: slate encoded", "output", fmt.Sprintf("%dx%d@%d", out.Width, out.Height, out.FPS),
				"ms", time.Since(t0).Milliseconds())
		}()
	}
	m.slateMu.Unlock()
	return e
}

// encodeSlate encodes a house slate: a black, silent source run through the same builder and
// encoder as the items, so its sample descriptions match theirs.
func (m *PackagerHLS) encodeSlate(ctx context.Context, host encode.HostProfile, out OutputProfile) (*packager.Slate, error) {
	src := filepath.Join(m.root, fmt.Sprintf("slate-%dx%d-%d.mkv", out.Width, out.Height, out.FPS))
	if _, err := os.Stat(src); err != nil {
		// Two hosts with one output size may make the source at once: each writes its own temp.
		f, err := os.CreateTemp(m.root, "slate-*.tmp.mkv")
		if err != nil {
			return nil, err
		}
		tmp := f.Name()
		_ = f.Close()
		cmd := exec.CommandContext(ctx, m.ffmpeg, "-hide_banner", "-nostdin", "-loglevel", "error", "-y",
			"-f", "lavfi", "-i", fmt.Sprintf("color=c=black:s=%dx%d:r=%d", out.Width, out.Height, out.FPS),
			"-f", "lavfi", "-i", "anullsrc=r=48000:cl=stereo", "-t", "2",
			"-c:v", "libx264", "-preset", "ultrafast", "-pix_fmt", "yuv420p", "-c:a", "aac", tmp)
		if o, err := cmd.CombinedOutput(); err != nil {
			_ = os.Remove(tmp)
			return nil, fmt.Errorf("packager hls: slate source: %w: %s", err, bytes.TrimSpace(o))
		}
		if err := os.Rename(tmp, src); err != nil {
			return nil, err
		}
	}
	format := MediaFormat{VideoCodec: "h264", Width: out.Width, Height: out.Height, FrameRate: float64(out.FPS),
		PixelFormat: "yuv420p", AudioCodec: "aac", AudioChannels: 2, AudioSampleRate: 48000, Container: "matroska,webm"}
	pl, err := Build(host, format, out)
	if err != nil {
		return nil, fmt.Errorf("packager hls: slate pipeline: %w", err)
	}
	gop := int64(out.gop())
	args := pl.FragmentArgs(src, 0, 0, gop, gop*audioRateHz/int64(out.FPS)/1024+1, out.FPS, 0)
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, m.ffmpeg, args...)
	cmd.Stderr = &stderr
	encoded, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("packager hls: slate encode: %w: %s", err, bytes.TrimSpace(stderr.Bytes()))
	}
	return packager.NewSlate(encoded)
}
