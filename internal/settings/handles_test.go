package settings

import (
	"bytes"
	"os"
	"reflect"
	"testing"
	"time"
)

// keys_gen.go is generated from the registry; a setting added, renamed or retyped without
// `go generate ./internal/settings` fails here.
func TestKeysGenIsCurrent(t *testing.T) {
	want, err := NewRegistry().KeysSource()
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("keys_gen.go")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("keys_gen.go is stale; run `go generate ./internal/settings`")
	}
}

type mapReader map[string]any

func (m mapReader) Value(key string) any { return m[key] }

func TestHandlesReadTheirKind(t *testing.T) {
	r := mapReader{
		LibraryURL.Key():                "http://media:8096",
		FillerSourceFolderEnabled.Key(): false,
	}
	if got := LibraryURL.Get(r); got != "http://media:8096" {
		t.Errorf("LibraryURL = %q", got)
	}
	// An answered false is false, even for a default-on switch.
	if FillerSourceFolderEnabled.Get(r) {
		t.Error("FillerSourceFolderEnabled = true, want the answered false")
	}
}

// A reader that cannot answer yields the declared default, so a default-on switch stays on.
func TestHandlesFallBackToTheDeclaredDefault(t *testing.T) {
	for _, r := range []Reader{nil, mapReader{}, (*Service)(nil)} {
		if !FillerSourceFolderEnabled.Get(r) {
			t.Errorf("%T: FillerSourceFolderEnabled = false, want its declared default true", r)
		}
		// A duration declared as "6h" is 6h, not zero.
		if got := FillerFetchEvery.Get(r); got != 6*time.Hour {
			t.Errorf("%T: FillerFetchEvery = %v, want 6h", r, got)
		}
	}
}

// The fallback is the value the service resolves for an unset key, for every setting: the same
// default, parsed by the same Kind rules.
func TestHandleFallbackMatchesTheResolvedDefault(t *testing.T) {
	svc := &Service{}
	for _, s := range NewRegistry().All() {
		if s.MigrationOnly || s.DataSubdir != "" {
			continue // a data subdirectory default depends on the data dir, which the handle cannot know
		}
		want := svc.defaultResolved(s, false, false).Value
		if got := fallback(s.Key); !reflect.DeepEqual(got, want) {
			t.Errorf("%s: fallback = %#v, resolved default = %#v", s.Key, got, want)
		}
	}
}

func TestHandleNames(t *testing.T) {
	for key, want := range map[string]string{
		"filler.vision.provider": "FillerVisionProvider",
		"llm.api_key":            "LLMAPIKey",
		"library.url":            "LibraryURL",
	} {
		if got := HandleName(key); got != want {
			t.Errorf("HandleName(%q) = %q, want %q", key, got, want)
		}
	}
}
