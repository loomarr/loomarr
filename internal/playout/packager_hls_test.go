package playout

import (
	"errors"
	"testing"
)

type fakeHLSOrigin struct {
	name     string
	acquired []string
	stopped  int
	assets   map[string]string
}

var errFakeAcquire = errors.New("fake acquire")

func (f *fakeHLSOrigin) acquirePlaylist(ch string, _ EncodePlan, _ bool) (hlsPlaylistLease, error) {
	f.acquired = append(f.acquired, ch)
	return hlsPlaylistLease{}, errFakeAcquire
}
func (f *fakeHLSOrigin) AssetPath(_ string, _ EncodePlan, rel string) (string, bool) {
	p, ok := f.assets[rel]
	return p, ok
}
func (f *fakeHLSOrigin) StopChannel(string) { f.stopped++ }
func (f *fakeHLSOrigin) StopAll()           { f.stopped++ }

func TestSwitchedHLSChoosesPerTuneAndServesBothOrigins(t *testing.T) {
	remux := &fakeHLSOrigin{name: "remux", assets: map[string]string{"seg-1.ts": "/remux/seg-1.ts"}}
	pk := &fakeHLSOrigin{name: "packager", assets: map[string]string{"init.mp4": "/pk/init.mp4"}}
	on := false
	s := switchedHLS{remux: remux, packaged: pk, usePackager: func() bool { return on }}

	_, _ = s.acquirePlaylist("a", PlanBaseline, false)
	on = true
	_, _ = s.acquirePlaylist("b", PlanBaseline, false)
	if len(remux.acquired) != 1 || remux.acquired[0] != "a" || len(pk.acquired) != 1 || pk.acquired[0] != "b" {
		t.Fatalf("remux tuned %v, packager tuned %v", remux.acquired, pk.acquired)
	}
	// A channel keeps serving from the origin it started on after the setting flips.
	on = false
	if p, ok := s.AssetPath("b", PlanBaseline, "init.mp4"); !ok || p != "/pk/init.mp4" {
		t.Fatalf("packager asset after flip: %q %v", p, ok)
	}
	if p, ok := s.AssetPath("a", PlanBaseline, "seg-1.ts"); !ok || p != "/remux/seg-1.ts" {
		t.Fatalf("remux asset: %q %v", p, ok)
	}
	s.StopAll()
	s.StopChannel("a")
	if remux.stopped != 2 || pk.stopped != 2 {
		t.Fatalf("stops: remux %d packager %d", remux.stopped, pk.stopped)
	}
}

func TestPackagerHLSAssetPathServesOnlyItsOwnFiles(t *testing.T) {
	m := &PackagerHLS{channels: map[remuxKey]*packagedChannel{
		{channel: "ch", plan: PlanBaseline}: {dir: "/scratch/ch-1"},
	}}
	for rel, want := range map[string]bool{
		"init.mp4": true, "seg00000007.m4s": true,
		"../secret": false, "seg/../../x.m4s": false, "live.m3u8": false, "seg-1.ts": false,
	} {
		if _, ok := m.AssetPath("ch", PlanBaseline, rel); ok != want {
			t.Errorf("AssetPath(%q) = %v, want %v", rel, ok, want)
		}
	}
	if _, ok := m.AssetPath("other", PlanBaseline, "init.mp4"); ok {
		t.Error("an asset of a channel with no packager resolved")
	}
}
