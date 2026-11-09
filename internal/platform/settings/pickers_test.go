package settings

import (
	"archive/zip"
	"context"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
	_ "time/tzdata" // every zone loads here whatever the host has installed
)

// A stored or pinned `eng` from before the picker resolves as the picker's `en`, so the select
// shows it chosen instead of an unknown value. Reading it back never loses the language.
func TestAudioLanguageResolvesToThePickersCode(t *testing.T) {
	for raw, want := range map[string]string{"eng": "en", "fre": "fr", "fra": "fr", "EN": "en", "": ""} {
		s, err := New(context.Background(), NewRegistry(), fakeLoader{m: map[string]string{"playout.audio_language": raw}}, nil)
		if err != nil {
			t.Fatal(err)
		}
		s.env = func(string) (string, bool) { return "", false }
		r := s.Resolve("playout.audio_language")
		if r.Value != want || r.Caution {
			t.Errorf("stored %q resolved %v (caution %v), want %q", raw, r.Value, r.Caution, want)
		}
	}
}

// The audio picker offers the first-track choice and the server's language catalog, and its
// default is one of them.
func TestAudioLanguageOffersTheLanguageCatalog(t *testing.T) {
	s, _ := NewRegistry().Get("playout.audio_language")
	var values []string
	for _, p := range s.Presets {
		values = append(values, p.Value)
	}
	if values[0] != "" || !slices.Contains(values, "en") || !slices.Contains(values, "ja") {
		t.Fatalf("presets = %v, want first track then the language catalog", values)
	}
	if !slices.Contains(values, ValueString(s.Default)) {
		t.Errorf("default %v is not one of the presets", s.Default)
	}
}

// Every timezone choice is a zone the server can load, so picking one never leaves the guide on
// the container's clock.
func TestTimezonePresetsAllLoad(t *testing.T) {
	s, _ := NewRegistry().Get("guide.timezone")
	if s.Presentation != PresentationTimezone {
		t.Fatalf("presentation = %q, want timezone", s.Presentation)
	}
	if len(s.Presets) < 300 || s.Presets[0].Value != "" {
		t.Fatalf("got %d presets starting %v, want the device choice then every city zone", len(s.Presets), s.Presets[0])
	}
	for _, p := range s.Presets[1:] {
		if _, err := time.LoadLocation(p.Value); err != nil {
			t.Errorf("%s does not load: %v", p.Value, err)
		}
	}
	for _, want := range []string{"America/New_York", "Europe/London", "Australia/Sydney", "UTC"} {
		if !slices.ContainsFunc(s.Presets, func(p Preset) bool { return p.Value == want }) {
			t.Errorf("missing %s", want)
		}
	}
}

// timezones_gen.go is the toolchain's tz data. A Go bump that adds a zone fails here until
// `make settings-keys` regenerates it.
func TestTimezonesGenIsCurrent(t *testing.T) {
	goroot, err := exec.Command("go", "env", "GOROOT").Output()
	if err != nil {
		t.Skipf("no go toolchain: %v", err)
	}
	r, err := zip.OpenReader(filepath.Join(strings.TrimSpace(string(goroot)), "lib", "time", "zoneinfo.zip"))
	if err != nil {
		t.Skipf("no Go tz data: %v", err)
	}
	defer func() { _ = r.Close() }()
	var names []string
	for _, f := range r.File {
		if !f.FileInfo().IsDir() {
			names = append(names, f.Name)
		}
	}
	slices.Sort(names)
	if !slices.Equal(names, goZoneNames) {
		t.Errorf("timezones_gen.go has %d zones, Go's tz data %d: run make settings-keys", len(goZoneNames), len(names))
	}
}

// Units is the wire's closed set, and every declared unit is in it.
func TestUnitsCoverTheRegistry(t *testing.T) {
	units := Units()
	for _, s := range NewRegistry().All() {
		if s.Unit != UnitNone && !slices.Contains(units, s.Unit) {
			t.Errorf("%s: unit %q is missing from Units()", s.Key, s.Unit)
		}
		if (s.Unit == UnitSeconds || s.Unit == UnitHours) && s.Kind != KindInt {
			t.Errorf("%s: a time unit names what a whole number counts, but kind is %q", s.Key, s.Kind)
		}
	}
}
