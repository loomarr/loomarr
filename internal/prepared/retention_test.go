package prepared

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPruneEvictsWholeColdPublicationsOldestFirst(t *testing.T) {
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	lib, err := newLibrary(t.TempDir(), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}

	oldest := publishSized(t, lib, "oldest", 600<<10)
	now = now.Add(time.Hour)
	protected := publishSized(t, lib, "protected", 600<<10)
	now = now.Add(time.Hour)
	newest := publishSized(t, lib, "newest", 600<<10)
	now = now.Add(time.Hour)
	if _, ok, err := lib.Lookup(baselineSpec("protected")); err != nil || !ok {
		t.Fatalf("touch protected publication = (_, %v, %v)", ok, err)
	}

	result, err := lib.Prune(context.Background(), 1<<20, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.PublicationsEvicted != 2 || result.BytesEvicted == 0 || result.RemainingBytes > result.BudgetBytes {
		t.Fatalf("Prune = %+v, want two whole publications evicted under budget", result)
	}
	assertPublicationMissing(t, oldest)
	assertPublicationPresent(t, protected)
	assertPublicationMissing(t, newest)
}

func TestPruneLeavesRecentPublicationsOverTheSoftCap(t *testing.T) {
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	lib, err := newLibrary(t.TempDir(), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	one := publishSized(t, lib, "one", 600<<10)
	two := publishSized(t, lib, "two", 600<<10)
	now = now.Add(31 * time.Minute)
	asset, ok, err := lib.Open(one.Key, "segment.m4s")
	if err != nil || !ok {
		t.Fatalf("Open = (_, %v, %v)", ok, err)
	}
	if err := asset.Content.Close(); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := lib.Lookup(baselineSpec("two")); err != nil || !ok {
		t.Fatalf("Lookup = (_, %v, %v)", ok, err)
	}

	result, err := lib.Prune(context.Background(), 1<<20, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.PublicationsEvicted != 0 || result.RemainingBytes <= result.BudgetBytes || result.ProtectedBytes == 0 {
		t.Fatalf("Prune = %+v, want protected overage", result)
	}
	assertPublicationPresent(t, one)
	assertPublicationPresent(t, two)
}

func TestPruneProtectsTheWholeStoreDuringStartupGrace(t *testing.T) {
	root := t.TempDir()
	seedNow := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	seed, err := newLibrary(root, func() time.Time { return seedNow })
	if err != nil {
		t.Fatal(err)
	}
	publication := publishSized(t, seed, "old", 600<<10)
	if err := os.Chtimes(publication.Directory, seedNow, seedNow); err != nil {
		t.Fatal(err)
	}

	now := seedNow.Add(14 * 24 * time.Hour)
	reopened, err := newLibrary(root, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	result, err := reopened.Prune(context.Background(), 1, nil)
	if err != nil || result.PublicationsEvicted != 0 {
		t.Fatalf("startup Prune = (%+v, %v), want protected", result, err)
	}
	assertPublicationPresent(t, publication)

	now = now.Add(preparedStartupGrace + time.Second)
	result, err = reopened.Prune(context.Background(), 1, nil)
	if err != nil || result.PublicationsEvicted != 1 {
		t.Fatalf("post-grace Prune = (%+v, %v), want eviction", result, err)
	}
	assertPublicationMissing(t, publication)
}

func TestPruneRemovesOnlyAbandonedOwnedStagingDirectories(t *testing.T) {
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	root := t.TempDir()
	lib, err := newLibrary(root, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(root, ".staging-"+validTestKey()+"-stale")
	fresh := filepath.Join(root, ".staging-"+validTestKey()+"-fresh")
	unknown := filepath.Join(root, "operator-data")
	for _, dir := range []string{stale, fresh, unknown} {
		if err := os.Mkdir(dir, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chtimes(stale, now.Add(-preparedStagingGrace-time.Second), now.Add(-preparedStagingGrace-time.Second)); err != nil {
		t.Fatal(err)
	}

	result, err := lib.Prune(context.Background(), 1<<20, nil)
	if err == nil || !errors.Is(err, ErrUnknownEntry) {
		t.Fatalf("Prune error = %v, want ErrUnknownEntry", err)
	}
	if result.StagingRemoved != 1 {
		t.Fatalf("Prune = %+v, want one abandoned staging directory removed", result)
	}
	if _, err := os.Stat(stale); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("stale staging still exists: %v", err)
	}
	for _, dir := range []string{fresh, unknown} {
		if _, err := os.Stat(dir); err != nil {
			t.Errorf("Prune removed %s: %v", dir, err)
		}
	}
}

func TestPruneProtectsTheAcceptedScheduleWithoutTreatingAProbeAsPlayback(t *testing.T) {
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	lib, err := newLibrary(t.TempDir(), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	publication := publishSized(t, lib, "scheduled", 600<<10)
	if err := os.Chtimes(publication.Directory, now, now); err != nil {
		t.Fatal(err)
	}
	specification := baselineSpec("scheduled")
	now = now.Add(preparedStartupGrace + preparedUseGrace + time.Hour)
	if _, ok, err := lib.Peek(specification); err != nil || !ok {
		t.Fatalf("Peek = (_, %v, %v), want hit", ok, err)
	}

	result, err := lib.Prune(context.Background(), 1, []Specification{specification})
	if err != nil || result.PublicationsEvicted != 0 || result.ProtectedBytes == 0 {
		t.Fatalf("scheduled Prune = (%+v, %v), want protected", result, err)
	}
	assertPublicationPresent(t, publication)

	result, err = lib.Prune(context.Background(), 1, nil)
	if err != nil || result.PublicationsEvicted != 1 {
		t.Fatalf("unscheduled Prune = (%+v, %v), want eviction", result, err)
	}
	assertPublicationMissing(t, publication)
}

func TestPruneOwnsReadinessControlFiles(t *testing.T) {
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	lib, err := newLibrary(t.TempDir(), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	readiness, err := OpenReadiness(lib)
	if err != nil {
		t.Fatal(err)
	}
	request := Request{Source: Source{Path: filepath.Join(t.TempDir(), "source.mkv")}, Rendition: baselineRendition()}
	if err := readiness.RememberBinding(
		BindingKey{ChannelID: "ch", LibraryItemID: "item"}, Binding{Policy: "policy", Request: request},
	); err != nil {
		t.Fatal(err)
	}
	stale := filepath.Join(lib.root, ".readiness-abandoned")
	if err := os.WriteFile(stale, []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := now.Add(-preparedStagingGrace - time.Hour)
	if err := os.Chtimes(stale, old, old); err != nil {
		t.Fatal(err)
	}

	result, err := lib.Prune(context.Background(), 1, nil)
	if err != nil {
		t.Fatalf("Prune with readiness control files: %v", err)
	}
	if result.StagingRemoved != 1 {
		t.Fatalf("Prune = %+v, want abandoned readiness workspace removed", result)
	}
	if _, err := os.Stat(filepath.Join(lib.root, readinessMetadata)); err != nil {
		t.Fatalf("readiness index was removed: %v", err)
	}
}

func publishSized(t *testing.T, lib *Library, source string, size int) Publication {
	t.Helper()
	pub, err := lib.Publish(context.Background(), baselineSpec(source), func(_ context.Context, workspace string) (Output, error) {
		return Output{Files: []string{"segment.m4s"}}, os.WriteFile(
			filepath.Join(workspace, "segment.m4s"), make([]byte, size), 0o600,
		)
	})
	if err != nil {
		t.Fatal(err)
	}
	return pub
}

func baselineSpec(source string) Specification {
	return Specification{SourceFingerprint: source, Rendition: RenditionContract{
		VideoCodec: "h264", AudioCodec: "aac", Width: 1920, Height: 1080,
		FrameRate: 25, VideoBitrateKbps: 5000, AudioBitrateKbps: 160,
		SegmentDurationMS: 2000, PackagingVersion: 1,
	}}
}

func assertPublicationPresent(t *testing.T, publication Publication) {
	t.Helper()
	if _, err := os.Stat(publication.Directory); err != nil {
		t.Fatalf("publication %s is missing: %v", publication.Key, err)
	}
}

func assertPublicationMissing(t *testing.T, publication Publication) {
	t.Helper()
	if _, err := os.Stat(publication.Directory); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("publication %s still exists: %v", publication.Key, err)
	}
}

func validTestKey() string { return "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" }
