package filler

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

// The licence path end to end: a source declares one, the walk writes it to the sidecar, the
// scan reads it back. This tests the READING half against the shape the live API actually
// returns — see internal/testkit/fixtures/archive/FINDINGS.md for the 2026-07-31 capture.

func writeSidecar(t *testing.T, dir, base string, fields map[string]any) {
	t.Helper()
	raw, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, base+".info.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLicenseBeside(t *testing.T) {
	dir := t.TempDir()
	media := filepath.Join(dir, "clip.mp4")
	if err := os.WriteFile(media, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Run("reads a declared licence", func(t *testing.T) {
		writeSidecar(t, dir, "clip", map[string]any{
			"title":   "ちょっとだけ懐かしいCM 1993年 その４ 年末",
			"license": "https://creativecommons.org/licenses/by-nc-sa/4.0/",
		})
		if got, want := licenseBeside(media), "https://creativecommons.org/licenses/by-nc-sa/4.0/"; got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	// ⚠ The COMMON case: ~92% of archive.org items declare no licence (667 of 8362 measured in
	// classic_tv_commercials). A sidecar without the key must read as unknown, not as an error
	// and not as a default.
	t.Run("a sidecar without a licence is unknown", func(t *testing.T) {
		writeSidecar(t, dir, "clip", map[string]any{"title": "Frosted Flakes"})
		if got := licenseBeside(media); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})

	// A hand-copied clip never had a sidecar. That is normal, not a scan failure.
	t.Run("no sidecar at all is unknown", func(t *testing.T) {
		lone := filepath.Join(t.TempDir(), "lone.mp4")
		if err := os.WriteFile(lone, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if got := licenseBeside(lone); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})

	// A truncated download leaves malformed JSON. Degrade, never fail the scan.
	t.Run("malformed json is unknown", func(t *testing.T) {
		if err := os.WriteFile(filepath.Join(dir, "clip.info.json"), []byte("{not json"), 0o600); err != nil {
			t.Fatal(err)
		}
		if got := licenseBeside(media); got != "" {
			t.Errorf("got %q, want empty", got)
		}
	})
}

// SidecarLicense is the fs.FS twin, for the tagger's drop-folder view. Same rules.
func TestSidecarLicense(t *testing.T) {
	fsys := fstest.MapFS{
		"a.info.json": {Data: []byte(`{"license":"https://creativecommons.org/publicdomain/zero/1.0/"}`)},
		"b.info.json": {Data: []byte(`{"title":"no licence here"}`)},
	}

	if got, want := SidecarLicense(fsys, "a.mp4"), "https://creativecommons.org/publicdomain/zero/1.0/"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	if got := SidecarLicense(fsys, "b.mp4"); got != "" {
		t.Errorf("got %q, want empty for a sidecar with no licence", got)
	}
	if got := SidecarLicense(fsys, "missing.mp4"); got != "" {
		t.Errorf("got %q, want empty when there is no sidecar", got)
	}
}

// ⚠ A licence must NOT leak into the tagger's prompt. It says nothing about whether a clip is a
// cereal advert, so it would burn tokens — and it could not survive the trip anyway, because
// `isBoilerplate` drops any line starting with http(s)://. This pins the separation so nobody
// "helpfully" folds the licence into the text signals later.
func TestLicenseNeverReachesTheTaggerPrompt(t *testing.T) {
	fsys := fstest.MapFS{
		"a.info.json": {Data: []byte(`{
			"title": "Frosted Flakes",
			"description": "A tiger sells cereal.",
			"license": "https://creativecommons.org/licenses/by-nc-sa/4.0/"
		}`)},
	}

	text := SidecarText(fsys, "a.mp4")
	if text == "" {
		t.Fatal("expected the title/description to render")
	}
	if strings.Contains(text, "creativecommons") || strings.Contains(text, "license") {
		t.Errorf("licence leaked into the tagger prompt: %q", text)
	}
}
