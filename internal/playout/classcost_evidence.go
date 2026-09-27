package playout

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// MeasuredCosts is the class probe's persisted answer for one encoder on one host.
type MeasuredCosts struct {
	Encoder Encoder
	Costs   map[CostKey]ClassCost
	// SessionLimit is how many encoder sessions opened at once (ProbeSessionLimit); 0 = none known.
	SessionLimit int
	ObservedAt   time.Time
}

const (
	classCostEvidenceName    = ".host-costs-v1.json"
	classCostEvidenceVersion = 1
)

type classCostEvidence struct {
	Version      int             `json:"version"`
	Fingerprint  string          `json:"fingerprint"`
	Encoder      Encoder         `json:"encoder"`
	SessionLimit int             `json:"sessionLimit"`
	Costs        []classCostCell `json:"costs"`
	ObservedAt   time.Time       `json:"observedAt"`
}

type classCostCell struct {
	Class    StreamClass `json:"class"`
	Height   int         `json:"height"`
	Curve    ToneCurve   `json:"curve,omitempty"` // HDR only; absent = Hable
	Speed    float64     `json:"speed"`
	CPUCores float64     `json:"cpuCores"`
}

// HostFingerprint identifies this FFmpeg build, GPU and default profile, the same key the encoder
// evidence uses: any change re-measures.
func HostFingerprint(ctx context.Context, ffmpegPath, gpu string) (string, error) {
	return hostCapabilityFingerprint(ctx, ffmpegPath, gpu, DefaultProfile())
}

// LoadClassCosts returns the stored costs when they were measured for this fingerprint and encoder
// within capabilityEvidenceMaxAge.
func LoadClassCosts(root, fingerprint string, enc Encoder, now time.Time) (MeasuredCosts, bool) {
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
		m.Costs[CostKey{Class: c.Class, Height: c.Height, Curve: c.Curve}] = ClassCost{Speed: c.Speed, CPUCores: c.CPUCores}
	}
	return m, true
}

// StoreClassCosts persists m atomically for fingerprint.
func StoreClassCosts(root, fingerprint string, m MeasuredCosts) error {
	e := classCostEvidence{Version: classCostEvidenceVersion, Fingerprint: fingerprint, Encoder: m.Encoder,
		SessionLimit: m.SessionLimit, ObservedAt: m.ObservedAt.UTC()}
	for _, class := range TranscodeClasses {
		for k, c := range m.Costs {
			if k.Class == class {
				e.Costs = append(e.Costs, classCostCell{Class: k.Class, Height: k.Height, Curve: k.Curve, Speed: c.Speed, CPUCores: c.CPUCores})
			}
		}
	}
	return writeEvidence(root, classCostEvidenceName, e)
}

// MigrateCapabilityEvidence moves the encoder evidence from its old home in the prepared library
// to playout.state_dir, once: a file already in the state directory wins and the old one is left.
// It reports whether a file moved.
func MigrateCapabilityEvidence(fromDir, toDir string) (bool, error) {
	fromDir, toDir = strings.TrimSpace(fromDir), strings.TrimSpace(toDir)
	if fromDir == "" || toDir == "" || filepath.Clean(fromDir) == filepath.Clean(toDir) {
		return false, nil
	}
	from := filepath.Join(fromDir, capabilityEvidenceName)
	to := filepath.Join(toDir, capabilityEvidenceName)
	if _, err := os.Stat(to); err == nil {
		return false, nil
	}
	src, err := os.Open(from)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	defer func() { _ = src.Close() }()
	body, err := io.ReadAll(io.LimitReader(src, 1<<20))
	if err != nil {
		return false, err
	}
	var e capabilityEvidence
	if err := json.Unmarshal(body, &e); err != nil {
		return false, fmt.Errorf("playout: old capability evidence is unreadable: %w", err)
	}
	// writeEvidence, not rename: the prepared library and the state directory may be on different
	// volumes, and the copy must be atomic in its new home before the old one goes.
	if err := writeEvidence(toDir, capabilityEvidenceName, e); err != nil {
		return false, err
	}
	return true, os.Remove(from)
}
