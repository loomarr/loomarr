package playout

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/loomarr/loomarr/internal/media/encode"
)

// MeasuredCosts is the class probe's persisted answer for one encoder on one host.
type MeasuredCosts struct {
	Encoder encode.Encoder
	Costs   map[CostKey]ClassCost
	// SessionLimit is how many encoder sessions opened at once (ProbeSessionLimit); 0 = none known.
	SessionLimit int
	ObservedAt   time.Time
}

const (
	classCostEvidenceName = ".host-costs-v1.json"
	// Version 2 measures VideoToolbox's CPU deinterlace fallback as part of every class.
	// Progressive-only measurements must be replaced before pricing the repaired graph.
	classCostEvidenceVersion = 2
)

type classCostEvidence struct {
	Version      int             `json:"version"`
	Fingerprint  string          `json:"fingerprint"`
	Encoder      encode.Encoder  `json:"encoder"`
	SessionLimit int             `json:"sessionLimit"`
	Costs        []classCostCell `json:"costs"`
	ObservedAt   time.Time       `json:"observedAt"`
}

type classCostCell struct {
	Class           StreamClass `json:"class"`
	Height          int         `json:"height"`
	Curve           ToneCurve   `json:"curve,omitempty"` // HDR only; absent = Hable
	Speed           float64     `json:"speed"`
	CPUCores        float64     `json:"cpuCores"`
	ConservativeCPU bool        `json:"conservativeCPU,omitempty"`
}

// HostFingerprint identifies this FFmpeg build, GPU and default profile, the same key the encoder
// evidence uses: any change re-measures.
func HostFingerprint(ctx context.Context, ffmpegPath, gpu string) (string, error) {
	return hostCapabilityFingerprint(ctx, ffmpegPath, gpu, DefaultProfile())
}

// LoadClassCosts returns the stored costs when they were measured for this fingerprint and encoder
// within capabilityEvidenceMaxAge.
func LoadClassCosts(root, fingerprint string, enc encode.Encoder, now time.Time) (MeasuredCosts, bool) {
	if strings.TrimSpace(root) == "" || fingerprint == "" {
		return MeasuredCosts{}, false
	}
	body, err := os.ReadFile(filepath.Join(root, classCostEvidenceName))
	if err != nil {
		return MeasuredCosts{}, false
	}
	var e classCostEvidence
	if json.Unmarshal(body, &e) != nil || e.Version != classCostEvidenceVersion || e.Fingerprint != fingerprint ||
		e.Encoder != enc || e.ObservedAt.IsZero() || now.Before(e.ObservedAt) ||
		now.Sub(e.ObservedAt) > capabilityEvidenceMaxAge || len(e.Costs) == 0 {
		return MeasuredCosts{}, false
	}
	m := MeasuredCosts{Encoder: e.Encoder, SessionLimit: e.SessionLimit, ObservedAt: e.ObservedAt, Costs: map[CostKey]ClassCost{}}
	for _, c := range e.Costs {
		m.Costs[CostKey{Class: c.Class, Height: c.Height, Curve: c.Curve}] = ClassCost{Speed: c.Speed, CPUCores: c.CPUCores, ConservativeCPU: c.ConservativeCPU}
	}
	return m, true
}

// StoreClassCosts persists m atomically for fingerprint.
func StoreClassCosts(root, fingerprint string, m MeasuredCosts) error {
	e := classCostEvidence{Version: classCostEvidenceVersion, Fingerprint: fingerprint, Encoder: m.Encoder,
		SessionLimit: m.SessionLimit, ObservedAt: m.ObservedAt.UTC()}
	for _, class := range append(TranscodeClasses, ClassPremium4K) {
		for k, c := range m.Costs {
			if k.Class == class {
				e.Costs = append(e.Costs, classCostCell{Class: k.Class, Height: k.Height, Curve: k.Curve, Speed: c.Speed, CPUCores: c.CPUCores, ConservativeCPU: c.ConservativeCPU})
			}
		}
	}
	return writeEvidence(root, classCostEvidenceName, e)
}
