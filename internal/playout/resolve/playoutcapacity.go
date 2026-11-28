package resolve

import (
	"context"
	"path/filepath"
	"strconv"
	"time"

	"github.com/loomarr/loomarr/internal/media/encode"
	"github.com/loomarr/loomarr/internal/platform/diagnostics"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/playout/admission"
	"github.com/loomarr/loomarr/internal/playout/classprobe"
	"github.com/loomarr/loomarr/internal/playout/media"
)

// ProbeCapacity measures this host's per-class stream costs and encoder session limit once at boot
// (#1512 G5) and runs the tone-map self-check (G11). A stored table for the same FFmpeg, GPU and
// encoder is reused, but the HDR class is measured at every start: the self-check is a startup
// check, and a driver update can break a tone-mapper without changing the fingerprint.
func (r *Resolver) ProbeCapacity(ctx context.Context, foreground classprobe.Foreground) {
	enc := encode.Encoder(r.encoder())
	if enc == "" {
		enc = r.detectedEncoder(ctx)
	}
	bin, gpu, root := r.capabilityInputs()
	started := time.Now()
	fingerprint, fpErr := classprobe.HostFingerprint(ctx, bin, gpu)
	cfg := classprobe.ClassProbeConfig{
		FFmpeg: bin, ClipDir: filepath.Join(root, "probe-clips"), Encoder: enc,
		CPUTonemap: playout.TonemapperFor(bin)(), GPU: playout.GPUFiltersFor(bin)(),
		Outputs: playout.ProbeOutputs(enc), Manager: r.processDiagnostics, Foreground: foreground,
		Curve: r.ToneCurve(),
	}
	stored, reused := classprobe.MeasuredCosts{}, false
	if fpErr == nil {
		stored, reused = classprobe.LoadClassCosts(root, fingerprint, enc, started)
	}
	if reused {
		r.measuredCosts.Store(&stored)
		cfg.Classes, cfg.Outputs = []media.StreamClass{media.ClassHDR4K}, cfg.Outputs[:1]
		// A table stored before the probe measured premium (#1512 G10) measures it now, not in a week.
		premium := false
		for k := range stored.Costs {
			premium = premium || k.Class == media.ClassPremium4K
		}
		if !premium {
			cfg.Classes = append(cfg.Classes, media.ClassPremium4K)
		}
	}
	res := classprobe.ProbeClassCosts(ctx, cfg)
	if ctx.Err() != nil {
		return
	}
	if r.onTonemap != nil {
		r.onTonemap(res.Tonemap)
	}

	m := classprobe.MeasuredCosts{Encoder: enc, Costs: map[admission.CostKey]admission.ClassCost{}, ObservedAt: started}
	if reused {
		m.SessionLimit, m.ObservedAt = stored.SessionLimit, stored.ObservedAt // the full table re-measures weekly
		for k, c := range stored.Costs {
			m.Costs[k] = c
		}
	}
	for k, c := range res.Costs {
		m.Costs[k] = c
	}
	if res.Tonemap.Ran && !res.Tonemap.OK {
		// A tone-map that fails now must not be admitted on an older measurement: HDR shows the card.
		for k := range m.Costs {
			if k.Class == media.ClassHDR4K {
				delete(m.Costs, k)
			}
		}
	}
	if !reused {
		bound := classprobe.SessionProbeBound
		if sdr, ok := res.Costs[admission.CostKey{Class: media.ClassSDR, Height: cfg.Outputs[0].Height}]; ok {
			bound = min(bound, max(1, int(sdr.Speed/1.2)+1)) // one past the throughput ceiling
		}
		limit, err := classprobe.ProbeSessionLimit(ctx, classprobe.SessionProbeConfig{
			FFmpeg: bin, Encoder: enc, Bound: bound, Manager: r.processDiagnostics,
			InUse: classprobe.EncoderSessionsInUse(enc), Foreground: foreground,
		})
		if err != nil && r.log != nil {
			r.log.Warn("playout: encoder session probe opened nothing; no session limit applied", "encoder", enc, "err", err)
		}
		m.SessionLimit = limit
	}
	if ctx.Err() != nil {
		return
	}
	r.measuredCosts.Store(&m)
	if fpErr == nil && len(m.Costs) > 0 {
		if err := classprobe.StoreClassCosts(root, fingerprint, m); err != nil && r.log != nil {
			r.log.Warn("playout: could not store the measured stream costs", "err", err)
		}
	}
	if r.log != nil {
		r.log.Info("playout: stream costs measured", "encoder", enc, "reused_stored_table", reused,
			"costs", costsForLog(m.Costs), "session_limit", m.SessionLimit, "tonemap", res.Tonemap,
			"failures", res.Failures, "took", time.Since(started).Round(time.Millisecond))
	}
}

// CostsFor is the measured cost table for enc, nil until the probe has published one for it.
func (r *Resolver) CostsFor(enc encode.Encoder) *classprobe.MeasuredCosts {
	m := r.measuredCosts.Load()
	if m == nil || m.Encoder != enc {
		return nil
	}
	return m
}

// ToneCurve is the curve the live chain maps HDR with (playout.ToneCurveSource): the probe keys its
// HDR measurement by it and the budget picks the HDR cells by it. It is the live playout.tone_curve
// setting, the same one the spawner maps with. Nil-safe.
func (r *Resolver) ToneCurve() media.ToneCurve {
	if r == nil || r.toneCurve == nil {
		return media.DefaultToneCurve
	}
	return media.ParseToneCurve(r.toneCurve())
}

func costsForLog(costs map[admission.CostKey]admission.ClassCost) map[string]admission.ClassCost {
	out := make(map[string]admission.ClassCost, len(costs))
	for k, c := range costs {
		out[string(k.Class)+"@"+strconv.Itoa(k.Height)] = c
	}
	return out
}

// TonemapObservation is the Diagnostics health of the tone-map self-check: a failure is red and
// immediate (G11: never silent); a tone-map too slow for a live channel is a warning.
func TonemapObservation(check classprobe.TonemapCheck) (diagnostics.HealthObservation, bool) {
	const route = "/settings/system/playback"
	switch {
	case !check.Ran:
		return diagnostics.HealthObservation{}, false
	case !check.OK:
		return diagnostics.HealthObservation{Status: diagnostics.HealthFailed, Immediate: true, RemediationRoute: route,
			Detail: "HDR programmes can't be converted for SDR screens on this server: " + check.Detail}, true
	case check.Detail != "":
		return diagnostics.HealthObservation{Status: diagnostics.HealthWarning, RemediationRoute: route,
			Detail: "4K HDR programmes will show a card instead: " + check.Detail}, true
	default:
		return diagnostics.HealthObservation{Status: diagnostics.HealthPassed,
			Detail: "HDR is converted for SDR screens (" + check.Stage + ")"}, true
	}
}
