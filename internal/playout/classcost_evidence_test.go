package playout

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestClassCosts_ProgressiveOnlyEvidenceIsRemeasured(t *testing.T) {
	root := t.TempDir()
	now := time.Now().UTC()
	evidence := classCostEvidence{
		Version: 1, Fingerprint: "fp", Encoder: EncoderVideoToolbox, ObservedAt: now,
		Costs: []classCostCell{{Class: ClassSDR, Height: 1080, Speed: 20, CPUCores: 0.02}},
	}
	raw, err := json.Marshal(evidence)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, classCostEvidenceName), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := LoadClassCosts(root, "fp", EncoderVideoToolbox, now); ok {
		t.Fatal("progressive-only costs were reused for the CPU-deinterlace graph")
	}
}

func TestClassCosts_RoundTripKeyedToFingerprintAndEncoder(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	m := MeasuredCosts{Encoder: EncoderNVENC, SessionLimit: 12, ObservedAt: now, Costs: map[CostKey]ClassCost{
		{Class: ClassSDR, Height: 1080}: {Speed: 19.18, CPUCores: 0.026}, {Class: ClassHDR4K, Height: 720}: {Speed: 11, CPUCores: 0.14},
		{Class: ClassPremium4K, Height: 2160}: {Speed: 3.1, CPUCores: 0.19, ConservativeCPU: true}, // admitted only on this cell
	}}
	if err := StoreClassCosts(root, "fp", m); err != nil {
		t.Fatal(err)
	}
	got, ok := LoadClassCosts(root, "fp", EncoderNVENC, now.Add(time.Hour))
	if !ok || got.SessionLimit != 12 || got.Costs[CostKey{Class: ClassSDR, Height: 1080}] != m.Costs[CostKey{Class: ClassSDR, Height: 1080}] ||
		got.Costs[CostKey{Class: ClassPremium4K, Height: 2160}] != m.Costs[CostKey{Class: ClassPremium4K, Height: 2160}] ||
		len(got.Costs) != 3 {
		t.Fatalf("round trip = %+v, %v", got, ok)
	}
	for name, load := range map[string]func() bool{
		"other fingerprint": func() bool { _, ok := LoadClassCosts(root, "fp2", EncoderNVENC, now); return ok },
		"other encoder":     func() bool { _, ok := LoadClassCosts(root, "fp", EncoderVAAPI, now); return ok },
		"stale":             func() bool { _, ok := LoadClassCosts(root, "fp", EncoderNVENC, now.Add(8*24*time.Hour)); return ok },
	} {
		if load() {
			t.Errorf("%s: stored costs were reused", name)
		}
	}
}

func TestMigrateCapabilityEvidence_MovesOnceFromThePreparedDir(t *testing.T) {
	prepared, state := t.TempDir(), filepath.Join(t.TempDir(), "playout-state")
	old := capabilityEvidence{Version: 1, Fingerprint: "fp", Encoder: EncoderNVENC, MaxChannels: 12, ObservedAt: time.Now().UTC()}
	if err := storeCapabilityEvidence(prepared, old); err != nil {
		t.Fatal(err)
	}
	moved, err := MigrateCapabilityEvidence(prepared, state)
	if err != nil || !moved {
		t.Fatalf("migrate = %v, %v", moved, err)
	}
	if _, err := os.Stat(filepath.Join(prepared, capabilityEvidenceName)); !os.IsNotExist(err) {
		t.Fatal("old evidence left in the prepared dir")
	}
	if got, ok := loadCapabilityEvidence(state, "fp", time.Now()); !ok || got.MaxChannels != 12 {
		t.Fatalf("migrated evidence = %+v, %v", got, ok)
	}
	// A second run, or a newer file already in the state dir, is left alone.
	if err := storeCapabilityEvidence(prepared, old); err != nil {
		t.Fatal(err)
	}
	if moved, err := MigrateCapabilityEvidence(prepared, state); moved || err != nil {
		t.Fatalf("second migrate = %v, %v; want a no-op", moved, err)
	}
}
