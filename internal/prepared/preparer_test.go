package prepared_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/loomarr/loomarr/internal/prepared"
)

type countingPackager struct {
	mu     sync.Mutex
	builds int
	err    error
}

type packagerFunc func(context.Context, string, prepared.Source, prepared.RenditionContract) (prepared.Output, error)

func (f packagerFunc) Package(ctx context.Context, workspace string, source prepared.Source, rendition prepared.RenditionContract) (prepared.Output, error) {
	return f(ctx, workspace, source, rendition)
}

func (p *countingPackager) Package(_ context.Context, workspace string, _ prepared.Source, _ prepared.RenditionContract) (prepared.Output, error) {
	p.mu.Lock()
	p.builds++
	p.mu.Unlock()
	if p.err != nil {
		return prepared.Output{}, p.err
	}
	files := map[string]string{
		"media.m3u8":  "#EXTM3U\n#EXTINF:2,\nsegment.m4s\n#EXT-X-ENDLIST\n",
		"segment.m4s": "media",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(workspace, name), []byte(body), 0o600); err != nil {
			return prepared.Output{}, err
		}
	}
	return prepared.Output{Files: []string{"media.m3u8", "segment.m4s"}}, nil
}

func (p *countingPackager) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.builds
}

func preparedRequest(path string) prepared.Request {
	return prepared.Request{Source: prepared.Source{Path: path}, Rendition: baseline("unused").Rendition}
}

func newTestPreparer(t *testing.T, library *prepared.Library, packager prepared.Packager) *prepared.Preparer {
	t.Helper()
	readiness, err := prepared.OpenReadiness(library)
	if err != nil {
		t.Fatal(err)
	}
	return prepared.NewPreparer(prepared.PreparerDependencies{
		Library: library, Packager: packager, Readiness: readiness,
	})
}

func TestPreparerLookupNeverFingerprintsOrBuildsOnDemand(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "movie.mkv")
	if err := os.WriteFile(path, []byte("movie bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	lib, err := prepared.NewLibrary(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	packager := &countingPackager{}
	preparer := newTestPreparer(t, lib, packager)

	if _, ok, err := preparer.Lookup(preparedRequest(path)); err != nil || ok {
		t.Fatalf("cold Lookup = (_, %v, %v), want fast miss", ok, err)
	}
	if packager.count() != 0 {
		t.Fatal("Lookup started preparation")
	}
}

func TestPreparerSharesOnePublicationAcrossConcurrentRequests(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "movie.mkv")
	if err := os.WriteFile(path, []byte("movie bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	lib, err := prepared.NewLibrary(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	packager := &countingPackager{}
	preparer := newTestPreparer(t, lib, packager)
	req := preparedRequest(path)

	const callers = 8
	publications := make(chan prepared.Publication, callers)
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pub, err := preparer.Prepare(t.Context(), req)
			publications <- pub
			errs <- err
		}()
	}
	wg.Wait()
	close(publications)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	key := ""
	for pub := range publications {
		if key == "" {
			key = pub.Key
		}
		if pub.Key != key {
			t.Fatalf("publication key = %q, want shared %q", pub.Key, key)
		}
	}
	if packager.count() != 1 {
		t.Fatalf("package builds = %d, want 1", packager.count())
	}
	resolved, ok, err := preparer.Lookup(req)
	if err != nil || !ok || resolved.SourceFingerprint == "" {
		t.Fatalf("warm Lookup = (%#v, %v, %v), want ready specification", resolved, ok, err)
	}
}

func TestPreparerChangedSourceMakesOldPublicationUnreachable(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "movie.mkv")
	if err := os.WriteFile(path, []byte("old movie"), 0o600); err != nil {
		t.Fatal(err)
	}
	lib, err := prepared.NewLibrary(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	packager := &countingPackager{}
	preparer := newTestPreparer(t, lib, packager)
	req := preparedRequest(path)
	old, err := preparer.Prepare(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(path, []byte("new movie bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := preparer.Lookup(req); err != nil || ok {
		t.Fatalf("changed-source Lookup = (_, %v, %v), want miss", ok, err)
	}
	fresh, err := preparer.Prepare(t.Context(), req)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Key == old.Key || packager.count() != 2 {
		t.Fatalf("changed source reused old publication: old=%q fresh=%q builds=%d", old.Key, fresh.Key, packager.count())
	}
}

func TestPreparerSelectedAudioTrackIsPartOfSourceIdentity(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "movie.mkv")
	if err := os.WriteFile(path, []byte("movie bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	lib, err := prepared.NewLibrary(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	packager := &countingPackager{}
	preparer := newTestPreparer(t, lib, packager)
	one := preparedRequest(path)
	two := one
	two.Source.AudioTrack = 1

	first, err := preparer.Prepare(t.Context(), one)
	if err != nil {
		t.Fatal(err)
	}
	second, err := preparer.Prepare(t.Context(), two)
	if err != nil {
		t.Fatal(err)
	}
	if first.Key == second.Key {
		t.Fatal("different selected audio tracks shared a publication")
	}
}

func TestPreparerFailedPackagingRemainsUnready(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "movie.mkv")
	if err := os.WriteFile(path, []byte("movie bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	lib, err := prepared.NewLibrary(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	wantErr := errors.New("packager stopped")
	preparer := newTestPreparer(t, lib, &countingPackager{err: wantErr})
	req := preparedRequest(path)
	if _, err := preparer.Prepare(t.Context(), req); !errors.Is(err, wantErr) {
		t.Fatalf("Prepare error = %v, want %v", err, wantErr)
	}
	if _, ok, err := preparer.Lookup(req); err != nil || ok {
		t.Fatalf("failed-package Lookup = (_, %v, %v), want miss", ok, err)
	}
}

func TestPreparerRejectsSourceThatChangesWhilePackaging(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "movie.mkv")
	if err := os.WriteFile(path, []byte("old movie"), 0o600); err != nil {
		t.Fatal(err)
	}
	lib, err := prepared.NewLibrary(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	packager := packagerFunc(func(_ context.Context, workspace string, source prepared.Source, _ prepared.RenditionContract) (prepared.Output, error) {
		if err := os.WriteFile(source.Path, []byte("new movie bytes"), 0o600); err != nil {
			return prepared.Output{}, err
		}
		if err := os.WriteFile(filepath.Join(workspace, "media.m3u8"), []byte("#EXTM3U\n"), 0o600); err != nil {
			return prepared.Output{}, err
		}
		return prepared.Output{Files: []string{"media.m3u8"}}, nil
	})
	preparer := newTestPreparer(t, lib, packager)
	req := preparedRequest(path)

	if _, err := preparer.Prepare(t.Context(), req); !errors.Is(err, prepared.ErrSourceChanged) {
		t.Fatalf("Prepare error = %v, want ErrSourceChanged", err)
	}
	if _, ok, err := preparer.Lookup(req); err != nil || ok {
		t.Fatalf("changed-during-package Lookup = (_, %v, %v), want miss", ok, err)
	}
}

func TestPreparerLookupReusesACompletePublicationAfterRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "movie.mkv")
	if err := os.WriteFile(path, []byte("movie bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	library, err := prepared.NewLibrary(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	request := preparedRequest(path)
	first := newTestPreparer(t, library, &countingPackager{})
	if _, err := first.Prepare(t.Context(), request); err != nil {
		t.Fatal(err)
	}

	restarted := newTestPreparer(t, library, &countingPackager{err: errors.New("must not rebuild")})
	specification, ok, err := restarted.Lookup(request)
	if err != nil || !ok || specification.SourceFingerprint == "" {
		t.Fatalf("Lookup after restart = (%+v, %v, %v), want existing publication", specification, ok, err)
	}
}
