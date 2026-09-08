package playout

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestHLSAssetSourceStaysWithOpenedRemux(t *testing.T) {
	first, replacement := &Process{}, &Process{}
	firstDir, nextDir := t.TempDir(), t.TempDir()
	for dir, body := range map[string]string{firstDir: "original asset", nextDir: "replacement asset"} {
		if err := os.WriteFile(filepath.Join(dir, "seg-0.ts"), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	key := remuxKey{channel: "channel", plan: PlanBaseline}
	manager := &HLSManager{remuxes: map[remuxKey]*hlsRemux{key: {dir: firstDir, source: first}}}
	file, source, err := manager.OpenAssetSource("channel", PlanBaseline, "seg-0.ts")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	manager.remuxes[key] = &hlsRemux{dir: nextDir, source: replacement}
	data, err := io.ReadAll(file)
	if err != nil || source != first || string(data) != "original asset" {
		t.Fatalf("opened snapshot source=%p bytes=%q err=%v", source, data, err)
	}
	next, nextSource, err := manager.OpenAssetSource("channel", PlanBaseline, "seg-0.ts")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = next.Close() }()
	data, err = io.ReadAll(next)
	if err != nil || nextSource != replacement || string(data) != "replacement asset" {
		t.Fatalf("replacement snapshot source=%p bytes=%q err=%v", nextSource, data, err)
	}
	for _, name := range []string{"", ".", "..", "../seg-0.ts", "missing.ts"} {
		if file, _, err := manager.OpenAssetSource("channel", PlanBaseline, name); err == nil {
			_ = file.Close()
			t.Fatalf("invalid asset accepted: %q", name)
		}
	}
	delete(manager.remuxes, key)
	if file, _, err := manager.OpenAssetSource("channel", PlanBaseline, "seg-0.ts"); err == nil {
		_ = file.Close()
		t.Fatal("retired remux accepted")
	}
}

func TestSinkLeasePinsItsSourceProcess(t *testing.T) {
	first, replacement := &Process{}, &Process{}
	session := &Session{proc: first, viewers: make(map[int]sessionViewer)}
	lease, ok := session.addSink(newHLSRelay())
	if !ok || lease.source != first {
		t.Fatal("sink did not retain its supplying process")
	}
	session.proc = replacement
	if lease.source != first {
		t.Fatal("sink source followed a replacement process")
	}
}
