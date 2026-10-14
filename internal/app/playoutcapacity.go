package app

import (
	"context"
	"path/filepath"
	"strconv"
	"time"

	"github.com/loomarr/loomarr/internal/diagnostics"
	"github.com/loomarr/loomarr/internal/playout"
)

// probeCapacity measures this host's per-class stream costs and encoder session limit once at boot
// (#1512 G5) and runs the tone-map self-check (G11). A stored table for the same FFmpeg, GPU and
// encoder is reused, but the HDR class is measured at every start: the self-check is a startup
// check, and a driver update can break a tone-mapper without changing the fingerprint.
func (r *playoutResolver) probeCapacity(ctx context.Context, foreground playout.Foreground) {
	enc := playout.Encoder(r.encoder())
	if enc == "" {
		enc = r.detectedEncoder(ctx)
	}
	bin, gpu, root := r.capabilityInputs()
	started := time.Now()
	fingerprint, fpErr := playout.HostFingerprint(ctx, bin, gpu)
	cfg := playout.ClassProbeConfig{
		FFmpeg: bin, ClipDir: filepath.Join(root, "probe-clips"), Encoder: enc,
		CPUTonemap: playout.TonemapperFor(bin)(), GPU: playout.GPUFiltersFor(bin)(),
		Outputs: playout.ProbeOutputs(enc), Manager: r.processDiagnostics, Foreground: foreground,
		Curve: r.ToneCurve(),
	}
	stored, reused := playout.MeasuredCosts{}, false
	if fpErr == nil {
		stored, reused = playout.LoadClassCosts(root, fingerprint, enc, started)
	}
	if reused {
		r.measuredCosts.Store(&stored)
		cfg.Classes, cfg.Outputs = []playout.StreamClass{playout.ClassHDR4K}, cfg.Outputs[:1]
		// A table stored before the probe measured premium (#1512 G10) measures it now, not in a week.
		premium := false
		for k := range stored.Costs {
			premium = premium || k.Class == playout.ClassPremium4K
		}
		if !premium {
			cfg.Classes = append(cfg.Classes, playout.ClassPremium4K)
		}
	}
	res := playout.ProbeClassCosts(ctx, cfg)
	if ctx.Err() != nil {
		return
	}
	if r.onTonemap != nil {
		r.onTonemap(res.Tonemap)
	}

	m := playout.MeasuredCosts{Encoder: enc, Costs: map[playout.CostKey]playout.ClassCost{}, ObservedAt: started}
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
			if k.Class == playout.ClassHDR4K {
				delete(m.Costs, k)
			}
		}
	}
	if !reused {
		bound := playout.SessionProbeBound
		if sdr, ok := res.Costs[playout.CostKey{Class: playout.ClassSDR, Height: cfg.Outputs[0].Height}]; ok {
			bound = min(bound, max(1, int(sdr.Speed/1.2)+1)) // one past the throughput ceiling
		}
		limit, err := playout.ProbeSessionLimit(ctx, playout.SessionProbeConfig{
			FFmpeg: bin, Encoder: enc, Bound: bound, Manager: r.processDiagnostics,
			InUse: playout.EncoderSessionsInUse(enc), Foreground: foreground,
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
		if err := playout.StoreClassCosts(root, fingerprint, m); err != nil && r.log != nil {
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
func (r *playoutResolver) CostsFor(enc playout.Encoder) *playout.MeasuredCosts {
	m := r.measuredCosts.Load()
	if m == nil || m.Encoder != enc {
		return nil
	}
	return m
}

// ToneCurve is the curve the live chain maps HDR with (playout.ToneCurveSource): the probe keys its
// HDR measurement by it and the budget picks the HDR cells by it. It is the live playout.tone_curve
// setting, the same one the spawner maps with. Nil-safe.
func (r *playoutResolver) ToneCurve() playout.ToneCurve {
	if r == nil || r.toneCurve == nil {
		return playout.DefaultToneCurve
	}
	return playout.ParseToneCurve(r.toneCurve())
}

func costsForLog(costs map[playout.CostKey]playout.ClassCost) map[string]playout.ClassCost {
	out := make(map[string]playout.ClassCost, len(costs))
	for k, c := range costs {
		out[string(k.Class)+"@"+strconv.Itoa(k.Height)] = c
	}
	return out
}

// tonemapObservation is the Diagnostics health of the tone-map self-check: a failure is red and
// immediate (G11: never silent); a tone-map too slow for a live channel is a warning.
func tonemapObservation(check playout.TonemapCheck) (diagnostics.HealthObservation, bool) {
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
