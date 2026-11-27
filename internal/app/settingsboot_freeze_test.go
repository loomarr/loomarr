package app

import (
	"testing"

	"github.com/loomarr/loomarr/internal/platform/settings"
)

func TestResolvedFreezeKeepsAppliedValuesAcrossLiveWrites(t *testing.T) {
	set := visionSet(t, map[string]string{
		"filler.dir":                   "/clips/old",
		"filler.watch_dir":             "/watch/old",
		"diagnostics.dir":              "/diagnostics/old",
		"filler.sync_every":            "5m",
		"filler.weight":                "3",
		"filler.source.folder.enabled": "true",
	})

	frozen, applied := set.freeze(settings.NewRegistry().RestartKeys()...)
	if applied["filler.dir"] != "/clips/old" || applied["filler.watch_dir"] != "/watch/old" {
		t.Fatalf("applied = %v, want canonical generation values", applied)
	}
	if len(applied) != 3 || applied["security.trusted_proxies"] != "127.0.0.0/8,::1/128" {
		t.Fatalf("applied = %v, want only the filler folders and the trusted proxies frozen for the generation", applied)
	}
	if _, frozenLive := applied["diagnostics.dir"]; frozenLive {
		t.Fatal("diagnostics.dir applies live; it must not be frozen for the generation")
	}

	set.svc.SetDB(map[string]string{
		"filler.dir":                   "/clips/new",
		"filler.watch_dir":             "/watch/new",
		"diagnostics.dir":              "/diagnostics/new",
		"filler.sync_every":            "10m",
		"filler.weight":                "7",
		"filler.source.folder.enabled": "false",
	})

	if got := settings.FillerDir.Get(frozen); got != "/clips/old" {
		t.Errorf("frozen string = %q, want old value", got)
	}
	if got := settings.FillerWatchDir.Get(frozen); got != "/watch/old" {
		t.Errorf("frozen watch = %q, want old value", got)
	}
	if got := settings.DiagnosticsDir.Get(frozen); got != "/diagnostics/new" {
		t.Errorf("live string = %q, want new value", got)
	}
	if got := settings.FillerSyncEvery.Get(frozen).String(); got != "10m0s" {
		t.Errorf("live duration = %q, want 10m0s", got)
	}
	if got := settings.FillerWeight.Get(frozen); got != 7 {
		t.Errorf("live int = %d, want 7", got)
	}
	if got := settings.FillerSourceFolderEnabled.Get(frozen); got {
		t.Error("live bool did not move to the newly persisted false value")
	}
	if got := settings.FillerSourceFolderEnabled.Get(frozen); got {
		t.Error("live safe-on bool did not move to the newly persisted false value")
	}

	if got := settings.FillerDir.Get(set); got != "/clips/new" {
		t.Errorf("live facade = %q, want newly persisted value", got)
	}
}
