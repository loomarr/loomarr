package prepared_test

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/prepared"
)

// Tune-time lookup must never wait for the readiness control plane. A publication is deliberately
// invisible until its atomic rename; while that build is running Peek reports a clean miss so
// Origin can use live fallback immediately. Blocking here creates an intermittent tune delay equal
// to however much of the background encode remains.
func TestLibraryPeekDoesNotWaitForAnInProgressPublication(t *testing.T) {
	t.Parallel()
	lib, err := prepared.NewLibrary(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	spec := baseline("source-being-prepared")
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	t.Cleanup(unblock)
	published := make(chan error, 1)
	go func() {
		_, publishErr := lib.Publish(t.Context(), spec, func(_ context.Context, workspace string) (prepared.Output, error) {
			close(started)
			<-release
			return writeOne("segment.m4s", "media")(context.Background(), workspace)
		})
		published <- publishErr
	}()
	<-started

	peeked := make(chan struct {
		ok  bool
		err error
	}, 1)
	go func() {
		_, ok, peekErr := lib.Peek(spec)
		peeked <- struct {
			ok  bool
			err error
		}{ok: ok, err: peekErr}
	}()

	select {
	case result := <-peeked:
		if result.err != nil || result.ok {
			t.Fatalf("Peek during publication = (_, %v, %v), want immediate clean miss", result.ok, result.err)
		}
	case <-time.After(250 * time.Millisecond):
		unblock()
		<-published
		t.Fatal("Peek waited behind the background publication — tune would stall until encoding finished")
	}

	unblock()
	if err := <-published; err != nil {
		t.Fatal(err)
	}
	if _, ok, err := lib.Peek(spec); err != nil || !ok {
		t.Fatalf("Peek after publication = (_, %v, %v), want hit", ok, err)
	}
}

func baseline(source string) prepared.Specification {
	return prepared.Specification{
		SourceFingerprint: source,
		Rendition: prepared.RenditionContract{
			VideoCodec: "h264", AudioCodec: "aac", Width: 1920, Height: 1080,
			FrameRate: 25, VideoBitrateKbps: 5000, AudioBitrateKbps: 160,
			SegmentDurationMS: 2000, PackagingVersion: 1,
		},
	}
}

func TestLibraryPublishMakesOneImmutablePublicationReusable(t *testing.T) {
	t.Parallel()
	lib, err := prepared.NewLibrary(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	builds := 0
	build := func(_ context.Context, workspace string) (prepared.Output, error) {
		builds++
		if err := os.WriteFile(filepath.Join(workspace, "init.mp4"), []byte("init"), 0o600); err != nil {
			return prepared.Output{}, err
		}
		if err := os.WriteFile(filepath.Join(workspace, "segment-0001.m4s"), []byte("media"), 0o600); err != nil {
			return prepared.Output{}, err
		}
		return prepared.Output{Files: []string{"init.mp4", "segment-0001.m4s"}}, nil
	}

	first, err := lib.Publish(context.Background(), baseline("source-a"), build)
	if err != nil {
		t.Fatal(err)
	}
	second, err := lib.Publish(context.Background(), baseline("source-a"), build)
	if err != nil {
		t.Fatal(err)
	}
	if first.Key != second.Key || first.Directory != second.Directory {
		t.Fatalf("same source/rendition produced different publications: %#v %#v", first, second)
	}
	if builds != 1 {
		t.Fatalf("builds = %d, want one shared preparation", builds)
	}
}

func TestLibraryFailedPublishIsInvisibleAndKeepsPreviousPublication(t *testing.T) {
	t.Parallel()
	lib, err := prepared.NewLibrary(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	oldSpec := baseline("source-old")
	old, err := lib.Publish(context.Background(), oldSpec, writeOne("segment.m4s", "old"))
	if err != nil {
		t.Fatal(err)
	}
	newSpec := baseline("source-new")
	wantErr := errors.New("encoder stopped")
	_, err = lib.Publish(context.Background(), newSpec, func(_ context.Context, workspace string) (prepared.Output, error) {
		if werr := os.WriteFile(filepath.Join(workspace, "partial.m4s"), []byte("partial"), 0o600); werr != nil {
			return prepared.Output{}, werr
		}
		return prepared.Output{}, wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Publish error = %v, want %v", err, wantErr)
	}
	if _, ok, err := lib.Lookup(newSpec); err != nil || ok {
		t.Fatalf("failed publication lookup = (_, %v, %v), want absent", ok, err)
	}
	gotOld, ok, err := lib.Lookup(oldSpec)
	if err != nil || !ok || gotOld.Key != old.Key {
		t.Fatalf("previous publication = (%#v, %v, %v), want %#v", gotOld, ok, err, old)
	}
}

func TestLibraryRejectsIncompleteOutput(t *testing.T) {
	t.Parallel()
	lib, err := prepared.NewLibrary(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	spec := baseline("source-a")
	_, err = lib.Publish(context.Background(), spec, func(context.Context, string) (prepared.Output, error) {
		return prepared.Output{Files: []string{"missing.m4s"}}, nil
	})
	if !errors.Is(err, prepared.ErrIncomplete) {
		t.Fatalf("Publish error = %v, want ErrIncomplete", err)
	}
	if _, ok, err := lib.Lookup(spec); err != nil || ok {
		t.Fatalf("incomplete publication lookup = (_, %v, %v), want absent", ok, err)
	}
}

func TestLibraryRejectsFileOutsideWorkspace(t *testing.T) {
	t.Parallel()
	lib, err := prepared.NewLibrary(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.m4s")
	if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
		t.Fatal(err)
	}
	spec := baseline("source-a")
	_, err = lib.Publish(context.Background(), spec, func(_ context.Context, workspace string) (prepared.Output, error) {
		if err := os.Symlink(outside, filepath.Join(workspace, "segment.m4s")); err != nil {
			return prepared.Output{}, err
		}
		return prepared.Output{Files: []string{"segment.m4s"}}, nil
	})
	if !errors.Is(err, prepared.ErrIncomplete) {
		t.Fatalf("Publish error = %v, want ErrIncomplete", err)
	}
}

func TestLibrarySourceFingerprintChangesIdentity(t *testing.T) {
	t.Parallel()
	lib, err := prepared.NewLibrary(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	one, err := lib.Publish(context.Background(), baseline("source-a"), writeOne("segment.m4s", "a"))
	if err != nil {
		t.Fatal(err)
	}
	two, err := lib.Publish(context.Background(), baseline("source-b"), writeOne("segment.m4s", "b"))
	if err != nil {
		t.Fatal(err)
	}
	if one.Key == two.Key {
		t.Fatal("changed source fingerprint reused stale publication identity")
	}
}

func TestLibraryOpensOnlyDeclaredAssetsFromACompletePublication(t *testing.T) {
	t.Parallel()
	lib, err := prepared.NewLibrary(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	pub, err := lib.Publish(context.Background(), baseline("source-a"), writeOne("segment.m4s", "media"))
	if err != nil {
		t.Fatal(err)
	}

	asset, ok, err := lib.Open(pub.Key, "segment.m4s")
	if err != nil || !ok {
		t.Fatalf("Open declared asset = (_, %v, %v), want hit", ok, err)
	}
	defer func() { _ = asset.Content.Close() }()
	body, err := io.ReadAll(asset.Content)
	if err != nil || string(body) != "media" || asset.Modified.IsZero() {
		t.Fatalf("opened asset = (%q, %v, %v)", body, asset.Modified, err)
	}

	for _, name := range []string{".publication.json", "missing.m4s", "../segment.m4s"} {
		if _, ok, err := lib.Open(pub.Key, name); err != nil || ok {
			t.Errorf("Open(%q) = (_, %v, %v), want absent", name, ok, err)
		}
	}
	if _, ok, err := lib.Open("not-a-publication-key", "segment.m4s"); err != nil || ok {
		t.Fatalf("Open invalid key = (_, %v, %v), want absent", ok, err)
	}
}

func writeOne(name, body string) prepared.Builder {
	return func(_ context.Context, workspace string) (prepared.Output, error) {
		if err := os.WriteFile(filepath.Join(workspace, name), []byte(body), 0o600); err != nil {
			return prepared.Output{}, err
		}
		return prepared.Output{Files: []string{name}}, nil
	}
}
