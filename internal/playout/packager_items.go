package playout

import (
	"log/slog"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/loomarr/loomarr/internal/media/encode"
	"github.com/loomarr/loomarr/internal/playout/packager"
)

// tuneInSeek starts a tune-in's encoder at the source's indexed keyframe at or before its offset
// (#1595), and returns how much earlier that is than the offset (negative: later, by at most
// demuxSeekBackoff). From the exact offset, ffmpeg's accurate seek decodes and discards everything
// back to that keyframe before the first frame: up to a whole source GOP on the tune path. From just
// past the keyframe it discards a few frames, and audio is still cut at the same instant
// (-noaccurate_seek would not promise that: MP4 seeks each track on its own). The slot is unchanged,
// so the item airs from the keyframe and its slot ends that much short of the airing's end. Only
// the tune-in: a later lookup (a resume after a break or a slate) must not air anything twice.
func tuneInSeek(it PackagerItem) (PackagerItem, time.Duration) {
	back := it.Seek - it.Keyframe
	if !it.KeyframeIndexed || back < 0 || back > tuneInRewindMax {
		return it, 0
	}
	seek := it.Keyframe + demuxSeekBackoff
	rewind := it.Seek - seek
	it.Seek = seek
	return it, rewind
}

// demuxSeekBackoff is how far past a keyframe a seek must aim to land on it. ffmpeg moves an input
// seek 3/23 s earlier for a demuxer that seeks by DTS (Matroska, not MP4) when a stream has B-frames
// (ffmpeg_demux.c), so a seek at the keyframe lands on the one before it: a whole GOP decoded and
// discarded again (measured: 637 ms to the first fragment at +112 ms, 303-315 ms at +131 ms).
const demuxSeekBackoff = 131 * time.Millisecond

// tuneInRewindMax bounds how far before the airing's offset a tune-in may start (tuneInSeek): past
// one long source GOP (x264's default 250 frames is 10.4 s at 23.976 fps), the airing's end it would
// cut costs more than the seek it saves.
const tuneInRewindMax = 12 * time.Second

// ladderResumeWait is how long an airing resumed on a new rung gets to produce, as long as a tune-in's
// first item (FirstItemWait): the encoder it replaces was too slow, so the timeline has no lead left,
// and a hold beats a slate.
const ladderResumeWait = 10 * time.Second

// itemLadder is one channel packager's software ladder (#1517): the RungMonitor of the airing on
// air, which outlives its encoder restarts (a new airing starts a new monitor). The schedule reads
// it; the item encoder's progress goroutine feeds it.
type itemLadder struct {
	// speed is where every item encoder's reported realtime multiple lands (float64 bits), whether
	// or not the host has a monitor; PackagerHLS.Stats reads it.
	speed   *atomic.Uint64
	mu      sync.Mutex
	airing  string
	monitor *RungMonitor
	stepped bool // a step ended the airing's encoder: its next open resumes on the monitor's rung
}

func (l *itemLadder) begin(airing string, m *RungMonitor) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.airing, l.monitor, l.stepped = airing, m, false
}

func (l *itemLadder) current() *RungMonitor {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.monitor
}

// resume reports whether airing is the one on air (a restart of it), on which rung, and whether a
// step caused the restart.
func (l *itemLadder) resume(airing string) (rung SoftwareRung, resumed, stepped bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.monitor == nil || l.airing != airing {
		return RungFull, false, false
	}
	stepped, l.stepped = l.stepped, false
	return l.monitor.Rung(), true, stepped
}

// watch records an item encoder's speed, feeds its samples to m and ends that encoder on a step.
// Samples from an encoder of an airing no longer on air are ignored by the monitor. m is nil on a
// host with no monitor (GPU hosts): the encoder's progress is still read for its speed.
func (l *itemLadder) watch(m *RungMonitor, log *slog.Logger, label string) func(SpeedSample) bool {
	return func(s SpeedSample) bool {
		if l.speed != nil && s.Speed > 0 {
			l.speed.Store(math.Float64bits(s.Speed))
		}
		if m == nil {
			return false
		}
		l.mu.Lock()
		defer l.mu.Unlock()
		if l.monitor != m || l.stepped {
			return false
		}
		d := m.Observe(s)
		if d.Step {
			l.stepped = true
			log.Info("packager hls: software rung step", "item", label, "rung", d.Rung.String(), "reason", d.Reason,
				"encoder_media", s.OutTime.Round(time.Millisecond))
		}
		return d.Step
	}
}

// prefetchedItem is the item the channel start resolved for admission, handed to the schedule's
// first lookup so it is not resolved twice. The schedule runs on one goroutine.
type prefetchedItem struct {
	at   time.Time
	item PackagerItem
	used bool
}

// take is the prefetched item as it airs at at, advanced by the time since it was resolved; false
// once taken, or when at is outside that airing.
func (p *prefetchedItem) take(at time.Time) (PackagerItem, bool) {
	if p == nil || p.used {
		return PackagerItem{}, false
	}
	p.used = true
	d := at.Sub(p.at)
	if d < 0 || d >= p.item.Remaining {
		return PackagerItem{}, false
	}
	it := p.item
	it.Remaining -= d
	if it.Input != "" {
		it.Seek += d
	}
	return it, true
}

// itemFault is what a failed encode of one source proved about this host. After a failed item the
// packager asks the schedule again, so the next attempt at the same source builds without the stage
// that failed (§9.1 V47). The encoder never changes: the channel's init must still match.
type itemFault struct {
	// cpuDecode: the GPU decoder faulted on this source (IsHardwareDecodeFault); retrying the same
	// -hwaccel path fails identically.
	cpuDecode bool
	// noOpenCL, noLibplacebo: that GPU tone-mapper failed this source, so the next one for the
	// curve is taken, ending at the CPU (the maintainer order, #1512).
	noOpenCL, noLibplacebo bool
}

func (f itemFault) apply(h encode.HostProfile) encode.HostProfile {
	if f.cpuDecode {
		h.DecodeCodecs = nil
	}
	h.TonemapOpenCL = h.TonemapOpenCL && !f.noOpenCL
	h.Libplacebo = h.Libplacebo && !f.noLibplacebo
	return h
}

// leftTheGPU reports a demotion that moved a stage onto the CPU, the live hardware→software fallback
// the metrics count: the decode, or the tone-map once no GPU tone-mapper is left on this host.
// OpenCL → libplacebo stays on the GPU and is not one.
func leftTheGPU(before, after itemFault, host encode.HostProfile) bool {
	if after.cpuDecode && !before.cpuDecode {
		return true
	}
	h := after.apply(host) // any other change demoted a GPU tone-mapper
	return after != before && !h.TonemapOpenCL && !h.Libplacebo
}

// itemFaults holds one channel packager's faults by source, for its life. Only a failure adds one.
type itemFaults struct {
	mu sync.Mutex
	by map[string]itemFault
}

func (f *itemFaults) get(input string) itemFault {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.by[input]
}

// record demotes one stage for an encode that failed before its first output: the decode on a GPU
// decode fault, otherwise the GPU tone-mapper the pipeline used. It reports false when there is
// nothing left to demote, so the item keeps failing into slate rather than retrying blindly.
func (f *itemFaults) record(input string, pl Pipeline, decodeFault bool) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	cur := f.by[input]
	switch {
	case decodeFault && !cur.cpuDecode:
		cur.cpuDecode = true
	case pl.Tonemapper == TonemapperOpenCL:
		cur.noOpenCL = true
	case pl.Tonemapper == TonemapperLibplacebo:
		cur.noLibplacebo = true
	default:
		return false
	}
	f.by[input] = cur
	return true
}

// packagerItemArgs is one item's encoder command for its slot: the pipeline builder's output on
// the host less any stage this source faulted, the channel's bug when wm is non-nil (a programme on
// a host whose overlay passed its self-check), the filler gain in its audio stage, fMP4 out.
func packagerItemArgs(host encode.HostProfile, out OutputProfile, it PackagerItem, wm *Watermark, slot packager.Slot, fault itemFault) (Pipeline, []string, error) {
	h := fault.apply(host)
	h.Overlay = wm != nil
	pl, err := BuildItem(h, it.Format, out, wm)
	if err != nil {
		return Pipeline{}, nil, err
	}
	pl = pl.WithGain(it.GainDB)
	return pl, pl.FragmentArgs(it.Input, it.Seek, slot.Offset, slot.Frames, slot.AudioFrames, out.FPS, it.AudioTrack), nil
}
