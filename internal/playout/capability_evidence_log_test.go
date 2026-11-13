package playout

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/media/encode"
)

// The host's measured encoder capacity is persisted so the next start can reuse it. When that cannot
// work (the file cannot be written, cannot be read, or the host cannot be identified) the host is
// measured again at every start, and nothing said so. Each failure is logged, once, with its cause;
// the ordinary cold cases (no file yet, an old or different host's file, a cancelled run) are not.
func TestCapabilityEvidenceFailuresAreLogged(t *testing.T) {
	now := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	measured := func(context.Context, string, Profile, string) Capacity {
		return Capacity{Chosen: encode.EncoderNVENC, MaxChannels: 7}
	}
	identified := func(context.Context, string, string, Profile) (string, error) { return "host-a", nil }
	rootThatIsAFile := func(t *testing.T) string {
		path := filepath.Join(t.TempDir(), "occupied")
		if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	corruptEvidence := func(t *testing.T) string {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, capabilityEvidenceName), []byte("{not json"), 0o600); err != nil {
			t.Fatal(err)
		}
		return root
	}
	unreadableEvidence := func(t *testing.T) string {
		root := t.TempDir()
		// A directory where the file should be: ReadFile fails with something other than not-exist.
		if err := os.Mkdir(filepath.Join(root, capabilityEvidenceName), 0o700); err != nil {
			t.Fatal(err)
		}
		return root
	}
	staleEvidence := func(t *testing.T) string {
		root := t.TempDir()
		if err := storeCapabilityEvidence(root, capabilityEvidence{
			Version: capabilityEvidenceVersion, Fingerprint: "host-a", Encoder: encode.EncoderNVENC,
			MaxChannels: 7, ObservedAt: now.Add(-2 * capabilityEvidenceMaxAge),
		}); err != nil {
			t.Fatal(err)
		}
		return root
	}
	otherHostEvidence := func(t *testing.T) string {
		root := t.TempDir()
		if err := storeCapabilityEvidence(root, capabilityEvidence{
			Version: capabilityEvidenceVersion, Fingerprint: "host-b", Encoder: encode.EncoderNVENC,
			MaxChannels: 7, ObservedAt: now,
		}); err != nil {
			t.Fatal(err)
		}
		return root
	}

	for name, tc := range map[string]struct {
		root        func(t *testing.T) string
		fingerprint func(context.Context, string, string, Profile) (string, error)
		cancelled   bool
		want        []string // substrings a WARN must carry; none = no WARN at all
		warns       int      // how many WARN lines; 0 means one
	}{
		// A root that is a file cannot be read from either, so the read and the store each say so.
		"the evidence cannot be stored":   {root: rootThatIsAFile, fingerprint: identified, want: []string{"could not store", "not a directory"}, warns: 2},
		"the evidence file is corrupt":    {root: corruptEvidence, fingerprint: identified, want: []string{"could not read", "invalid character"}},
		"the evidence path is unreadable": {root: unreadableEvidence, fingerprint: identified, want: []string{"could not read", "could not store"}, warns: 2},
		"the host cannot be identified": {root: func(t *testing.T) string { return t.TempDir() }, fingerprint: func(context.Context, string, string, Profile) (string, error) {
			return "", errors.New("ffmpeg not found")
		}, want: []string{"identify", "ffmpeg not found"}},
		"no evidence yet is an ordinary cold start":       {root: func(t *testing.T) string { return t.TempDir() }, fingerprint: identified},
		"stale evidence is ordinary":                      {root: staleEvidence, fingerprint: identified},
		"another host's evidence is ordinary":             {root: otherHostEvidence, fingerprint: identified},
		"a cancelled run stores nothing and says nothing": {root: func(t *testing.T) string { return t.TempDir() }, fingerprint: identified, cancelled: true},
		"no evidence root configured is not a failure":    {root: func(*testing.T) string { return "" }, fingerprint: identified},
	} {
		t.Run(name, func(t *testing.T) {
			var logs bytes.Buffer
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			deps := capabilityEvidenceDependencies{
				now: func() time.Time { return now }, fingerprint: tc.fingerprint, detect: measured,
				log: slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelInfo})),
			}
			if tc.cancelled {
				deps.detect = func(context.Context, string, Profile, string) Capacity {
					cancel()
					return Capacity{Chosen: encode.EncoderNVENC, MaxChannels: 7}
				}
			}
			capacity, reused := detectObservedWithEvidence(ctx, "ffmpeg", DefaultProfile(), "nvidia", tc.root(t), deps)
			if reused || capacity.MaxChannels != 7 {
				t.Fatalf("detection = %+v reused=%v, want the measured capacity, not reused", capacity, reused)
			}
			out := logs.String()
			if len(tc.want) == 0 {
				if strings.Contains(out, "level=WARN") {
					t.Fatalf("an ordinary case warned: %s", out)
				}
				return
			}
			wantWarns := max(tc.warns, 1)
			if got := strings.Count(out, "level=WARN"); got != wantWarns {
				t.Fatalf("want %d WARN line(s), got %d: %s", wantWarns, got, out)
			}
			for _, want := range tc.want {
				if !strings.Contains(out, want) {
					t.Errorf("log %q does not mention %q", out, want)
				}
			}
		})
	}

	t.Run("a nil logger is safe on every path", func(t *testing.T) {
		deps := capabilityEvidenceDependencies{now: func() time.Time { return now }, fingerprint: identified, detect: measured}
		detectObservedWithEvidence(t.Context(), "ffmpeg", DefaultProfile(), "nvidia", rootThatIsAFile(t), deps)
		detectObservedWithEvidence(t.Context(), "ffmpeg", DefaultProfile(), "nvidia", corruptEvidence(t), deps)
	})
}
