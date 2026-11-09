package playout

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/media/encode"
	"github.com/loomarr/loomarr/internal/playout"
)

// slowEncodeSource airs one never-ending movie on a fixed host.
type slowEncodeSource struct{ host encode.HostProfile }

func (slowEncodeSource) ItemAt(context.Context, string, time.Time) (playout.PackagerItem, error) {
	return playout.PackagerItem{Label: "prog", Remaining: time.Hour, Input: "movie.mkv", Format: playout.MediaFormat{
		VideoCodec: "h264", Width: 1280, Height: 720, FrameRate: 25, PixelFormat: "yuv420p",
		AudioCodec: "aac", AudioChannels: 2, AudioSampleRate: 48000, Container: "matroska,webm",
	}}, nil
}

func (slowEncodeSource) Premium(context.Context, string) playout.FormatClass { return "" }

func (s slowEncodeSource) Output(context.Context, string, playout.FormatClass, int) (encode.HostProfile, playout.OutputProfile) {
	return s.host, playout.OutputProfile{Width: 1280, Height: 720, FPS: 25, GOPSeconds: 2, Quality: 23, TargetKbps: 3000, MaxKbps: 4500, AudioKbps: 128}
}

// A channel whose encoder reports 0.6x realtime must read Stalled on the playout status, software or
// hardware: the verdict is only as good as the speed the packager's Stats carry.
func TestChannelHealth_SlowEncoderReadsStalledFromPackagerStats(t *testing.T) {
	hosts := map[string]encode.HostProfile{
		"software": encode.HostFor(encode.EncoderSoftware, false, encode.GPUFilters{}),
		"hardware": encode.HostFor(encode.EncoderNVENC, true, encode.GPUFilters{}),
	}
	for name, host := range hosts {
		t.Run(name, func(t *testing.T) {
			ffmpeg := filepath.Join(t.TempDir(), "ffmpeg")
			script := "#!/bin/sh\ncase \"$*\" in *pipe:3*) printf 'frame=10\\nout_time_ms=500000\\nspeed=0.60x\\nprogress=continue\\n' >&3;; esac\nexec sleep 30\n"
			if err := os.WriteFile(ffmpeg, []byte(script), 0o755); err != nil {
				t.Fatal(err)
			}
			m, err := playout.NewPackagerHLS(slowEncodeSource{host: host}, ffmpeg, t.TempDir(), time.Minute, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(m.Stop)

			// A viewer polls the playlist; no fragment ever lands, so the read waits until cancelled.
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			go func() { _, _, _ = m.MediaPlaylist(ctx, "ch", string(playout.FormatBaseline)+".m3u8", false) }()

			var row ChannelHealth
			deadline := time.Now().Add(10 * time.Second)
			for time.Now().Before(deadline) {
				if stats := m.Stats(time.Now()); len(stats) == 1 && stats[0].Speed > 0 {
					row = channelHealthFrom(stats[0])
					break
				}
				time.Sleep(20 * time.Millisecond)
			}
			if row.ChannelID == "" {
				t.Fatalf("Stats never reported the encoder's speed: %+v", m.Stats(time.Now()))
			}
			if row.Health != HealthStalled {
				t.Errorf("a 0.6x transcode read %q (speed %v, reason %q), want %q", row.Health, row.Speed, row.Reason, HealthStalled)
			}
		})
	}
}
