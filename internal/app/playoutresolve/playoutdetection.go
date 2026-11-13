package playoutresolve

import (
	"context"

	"github.com/loomarr/loomarr/internal/media/encode"
	"github.com/loomarr/loomarr/internal/playout"
)

// Profile is the encode profile at the ladder rung the ResourceBudget admitted a session at,
// resolved against the measured encoder and the operator's quality tier (§9.1: "best picture the
// hardware sustains, then adapt"). A new session takes the best rung that fits and keeps it.
func (r *Resolver) Profile(ctx context.Context, rung int) playout.Profile {
	enc := encode.Encoder(r.encoder())
	if enc == "" {
		// A matching persisted result is available to this first tune after only its host fingerprint
		// matches. Its real encoder validation runs asynchronously; a miss likewise starts the full
		// benchmark in the background and uses software until a safe result is ready.
		//
		// Falling straight through to libx264 would leave a box with a working GPU silently
		// encoding in software forever. Detect trial-encodes each candidate, so its answer is
		// measured rather than inferred from `ffmpeg -encoders` (which lists encoders the
		// hardware cannot actually run — the exact trap that took a live channel down: the
		// host listed h264_vulkan, the container had no /dev/dri).
		r.reuseEncoderEvidence(ctx)
		enc = encode.EncoderSoftware
		if r.detectReady.Load() {
			enc = r.PublishedEncoder()
		}
		r.warmEncoderDetection(ctx)
	}
	return playout.Resolve(playout.TierFor(r.tier()), enc, rung)
}

// WarmProfile loads the host's persisted encoder evidence ahead of the first tune (#1512 G2: its
// host fingerprint was 0.2 s of a cold first tune). It is cheap (one `ffmpeg -version`); the full
// encoder measurement still starts only on the first demand.
func (r *Resolver) WarmProfile(ctx context.Context) { r.reuseEncoderEvidence(ctx) }

func (r *Resolver) reuseEncoderEvidence(ctx context.Context) {
	r.detectEvidence.Do(func() {
		var capacity playout.Capacity
		var ok bool
		if r.loadCapabilityEvidence != nil {
			capacity, ok = r.loadCapabilityEvidence(ctx)
		} else {
			bin, gpu, root := r.capabilityInputs()
			capacity, ok = playout.LoadMatchingObservedCapabilityEvidence(
				ctx, bin, playout.DefaultProfile(), gpu, root,
			)
		}
		if ok {
			r.publishDetectedCapacity(capacity, true)
		}
	})
}

// warmEncoderDetection starts the expensive host probe once without putting it on a viewer's
// critical path. It runs as the owner's work, not the request's, so one TV disconnecting cannot
// discard the result every later tune needs, and shutdown waits for it.
func (r *Resolver) warmEncoderDetection(fallback context.Context) {
	r.detectStart.Do(func() {
		detect := func(ctx context.Context) { r.detectedEncoder(ctx) }
		if r.goDetect != nil {
			r.goDetect(detect)
			return
		}
		go detect(context.WithoutCancel(fallback))
	})
}

// detectedEncoder returns the best encoder that actually WORKS here, probing once. Control-plane
// callers may wait for it; Profile deliberately uses warmEncoderDetection + detectReady instead.
//
// Lazily and exactly once per process, which is the only workable timing: a full Detect trials
// every candidate and then measures the winner warm. The first demand starts it asynchronously;
// everything after completion reads the result. A matching persisted hardware result substitutes
// one short real TS encode, while an FFmpeg/GPU/profile mismatch or failed validation performs the
// complete measurement and atomically replaces that evidence.
func (r *Resolver) detectedEncoder(ctx context.Context) encode.Encoder {
	r.detectOnce.Do(func() {
		bin, gpu, root := r.capabilityInputs()
		cap, reused := playout.DetectObservedWithEvidence(
			ctx, bin, playout.DefaultProfile(), gpu, root, r.processDiagnostics,
		)
		r.publishDetectedCapacity(cap, reused)
	})
	return r.PublishedEncoder()
}

func (r *Resolver) capabilityInputs() (bin, gpu, root string) {
	r.inputsOnce.Do(func() {
		if r.ffmpegPath != nil {
			r.capabilityBin = r.ffmpegPath()
		}
		if r.capabilityBin == "" {
			r.capabilityBin = "ffmpeg"
		}
		if r.gpuName != nil {
			r.capabilityGPU = r.gpuName()
		}
		if r.capabilityRoot != nil {
			r.capabilityPath = r.capabilityRoot()
		}
	})
	return r.capabilityBin, r.capabilityGPU, r.capabilityPath
}

func (r *Resolver) publishDetectedCapacity(cap playout.Capacity, reused bool) {
	r.detectedMu.Lock()
	r.detected = cap.Chosen
	r.detectedMu.Unlock()
	r.maxChannels.Store(int64(cap.MaxChannels))
	if cap.EncodeHostBytes > 0 {
		// Only a real measurement replaces the previous one; a cheap evidence reload carries none.
		r.encodeHostBytes.Store(cap.EncodeHostBytes)
	}
	if r.log != nil {
		skipped := make([]string, 0, len(cap.All))
		for _, c := range cap.All {
			if !c.Works {
				skipped = append(skipped, string(c.Encoder)+": "+c.Err)
			}
		}
		r.log.Info("playout: encoder probed",
			"chosen", cap.Chosen, "measured_max_channels", cap.MaxChannels,
			"reused_verified_evidence", reused, "skipped", skipped)
	}
	r.detectReady.Store(true)
}

func (r *Resolver) PublishedEncoder() encode.Encoder {
	r.detectedMu.RLock()
	defer r.detectedMu.RUnlock()
	return r.detected
}

// FileEncodeHost is the hardware filler derivatives encode on (#2121), the channels' own: the
// operator's pinned encoder, else the one detection published, and software until detection
// settles. encode.FileFamily then keeps a family without a measured level in software.
func (r *Resolver) FileEncodeHost(pinned encode.Encoder) encode.HostProfile {
	enc := pinned
	if enc == "" && r != nil && r.detectReady.Load() {
		enc = r.PublishedEncoder()
	}
	if enc == "" {
		enc = encode.EncoderSoftware
	}
	return encode.HostFor(enc, false, encode.GPUFilters{})
}

// PublishedCapability returns only the encoder result already published by the asynchronous
// capability probe. Dashboard polling must never start or wait for a benchmark.
func (r *Resolver) PublishedCapability() playout.Capacity {
	if !r.detectReady.Load() {
		return playout.Capacity{}
	}
	return playout.Capacity{Chosen: r.PublishedEncoder(), MaxChannels: int(r.maxChannels.Load())}
}

// EncodeHostBytes is the measured host memory one hardware encode holds, 0 until a trial encode
// has measured it on this host.
func (r *Resolver) EncodeHostBytes() int64 {
	return r.encodeHostBytes.Load()
}
