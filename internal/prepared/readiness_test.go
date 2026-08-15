package prepared

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadinessPersistsBindingsAndFingerprintsAcrossRestart(t *testing.T) {
	root := t.TempDir()
	library, err := NewLibrary(root)
	if err != nil {
		t.Fatal(err)
	}
	index, err := OpenReadiness(library)
	if err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(t.TempDir(), "movie.mkv")
	if err := os.WriteFile(sourcePath, []byte("movie"), 0o600); err != nil {
		t.Fatal(err)
	}
	request := Request{Source: Source{Path: sourcePath, AudioTrack: 2}, Rendition: baselineRendition()}
	key := BindingKey{ChannelID: "ch-1", LibraryItemID: "item-1"}
	if err := index.RememberBinding(key, Binding{
		Policy: "balanced", ChannelPolicy: "eng", Request: request,
	}); err != nil {
		t.Fatal(err)
	}
	_, version, err := sourceVersion(request.Source)
	if err != nil {
		t.Fatal(err)
	}
	if err := index.rememberFingerprint(version, "sha256:movie:audio:2"); err != nil {
		t.Fatal(err)
	}

	reopened, err := OpenReadiness(library)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := reopened.Binding(key, "balanced", "eng")
	if !ok || got != request {
		t.Fatalf("Binding after restart = (%+v, %v), want %+v", got, ok, request)
	}
	if fingerprint, ok := reopened.fingerprint(version); !ok || fingerprint != "sha256:movie:audio:2" {
		t.Fatalf("fingerprint after restart = (%q, %v)", fingerprint, ok)
	}
}

func TestReadinessReplacesStalePoliciesAndFileVersions(t *testing.T) {
	library, err := NewLibrary(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	index, err := OpenReadiness(library)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "episode.mkv")
	if err := os.WriteFile(path, []byte("one"), 0o600); err != nil {
		t.Fatal(err)
	}
	key := BindingKey{ChannelID: "ch", LibraryItemID: "episode"}
	one := Request{Source: Source{Path: path, AudioTrack: 0}, Rendition: baselineRendition()}
	two := one
	two.Source.AudioTrack = 1
	if err := index.RememberBinding(key, Binding{Policy: "one", Request: one}); err != nil {
		t.Fatal(err)
	}
	if err := index.RememberBinding(key, Binding{Policy: "two", ChannelPolicy: "jpn", Request: two}); err != nil {
		t.Fatal(err)
	}
	if _, ok := index.Binding(key, "one", ""); ok {
		t.Fatal("replaced source policy still resolves")
	}
	if got, ok := index.Binding(key, "two", "jpn"); !ok || got != two {
		t.Fatalf("replacement binding = (%+v, %v)", got, ok)
	}

	_, oldVersion, err := sourceVersion(one.Source)
	if err != nil {
		t.Fatal(err)
	}
	if err := index.rememberFingerprint(oldVersion, "old"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("a changed episode"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, newVersion, err := sourceVersion(one.Source)
	if err != nil {
		t.Fatal(err)
	}
	if err := index.rememberFingerprint(newVersion, "new"); err != nil {
		t.Fatal(err)
	}
	if _, ok := index.fingerprint(oldVersion); ok {
		t.Fatal("stale file version remained in the durable index")
	}
	if got, ok := index.fingerprint(newVersion); !ok || got != "new" {
		t.Fatalf("new fingerprint = (%q, %v)", got, ok)
	}
}

func TestReadinessCorruptionDegradesToAReplaceableEmptyIndex(t *testing.T) {
	library, err := NewLibrary(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(library.root, readinessMetadata), []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	index, err := OpenReadiness(library)
	if err == nil || index == nil {
		t.Fatalf("OpenReadiness = (%v, %v), want usable empty index plus warning", index, err)
	}
	key := BindingKey{ChannelID: "ch", LibraryItemID: "item"}
	request := Request{Source: Source{Path: "/media/item.mkv", AudioTrack: 0}, Rendition: baselineRendition()}
	if err := index.RememberBinding(key, Binding{Policy: "policy", Request: request}); err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenReadiness(library)
	if err != nil {
		t.Fatalf("replacement index remained corrupt: %v", err)
	}
	if got, ok := reopened.Binding(key, "policy", ""); !ok || got != request {
		t.Fatalf("replacement binding = (%+v, %v)", got, ok)
	}
}
