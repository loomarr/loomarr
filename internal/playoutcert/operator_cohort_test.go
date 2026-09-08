package playoutcert

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type operatorCohortFixture struct {
	root, path string
	manifest   operatorManifest
	channels   []Channel
	media      [2][]byte
}

func newOperatorCohortFixture(t *testing.T) *operatorCohortFixture {
	t.Helper()
	f := &operatorCohortFixture{root: t.TempDir(), channels: []Channel{{ID: "private-first-channel"}, {ID: "private-second-channel"}}, media: [2][]byte{[]byte("first private programme"), []byte("second private programme")}}
	f.path = filepath.Join(f.root, "private-corpus.json")
	var programmes []string
	for i, data := range f.media {
		name := fmt.Sprintf("private-source-%d.media", i)
		if err := os.WriteFile(filepath.Join(f.root, name), data, 0600); err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(data)
		luma, rate := "0,\"max\":25", "0.012,\"max\":0.026"
		if i == 1 {
			luma, rate = "225,\"max\":255", "0.027,\"max\":0.05"
		}
		programmes = append(programmes, fmt.Sprintf(`{"file":%q,"bytes":%d,"sha256":%q,"signals":{"luma":{"min":%s},"zeroCrossingRate":{"min":%s},"rmsDB":{"min":-80,"max":-1},"silence":false}}`, name, len(data), hex.EncodeToString(digest[:]), luma, rate))
	}
	raw := fmt.Sprintf(`{"schemaVersion":1,"channels":[{"id":%q,"programmes":[%s]},{"id":%q,"programmes":[%s]}]}`, f.channels[0].ID, strings.Join(programmes, ","), f.channels[1].ID, strings.Join(programmes, ","))
	if err := json.Unmarshal([]byte(raw), &f.manifest); err != nil {
		t.Fatal(err)
	}
	f.write(t)
	return f
}

func (f *operatorCohortFixture) write(t *testing.T) {
	t.Helper()
	data, err := json.Marshal(f.manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f.path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestOperatorCohortStagesVerifiedSharedSources(t *testing.T) {
	f := newOperatorCohortFixture(t)
	cohort, err := LoadOperatorCohort(t.Context(), f.path, f.channels)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cohort.Close() }()
	if !cohort.MatchesChannels(f.channels) || cohort.MatchesChannels([]Channel{f.channels[1], f.channels[0]}) || cohort.MatchesChannels([]Channel{f.channels[0], f.channels[0]}) {
		t.Fatal("loaded cohort did not preserve its exact Channel mapping")
	}
	stage, err := cohort.Stage(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	files, err := os.ReadDir(stage.Directory)
	if err != nil || len(files) != 2 {
		t.Fatalf("shared source copies=%d err=%v", len(files), err)
	}
	first, second := stage.Channels[f.channels[0].ID], stage.Channels[f.channels[1].ID]
	for i, source := range first {
		data, err := os.ReadFile(source.Path)
		if err != nil || !bytes.Equal(data, f.media[i]) || source.Path != second[i].Path {
			t.Fatalf("staged source %d not bound/shared", i)
		}
		if err := os.WriteFile(filepath.Join(f.root, f.manifest.Channels[0].Programmes[i].File), []byte("changed original"), 0600); err != nil {
			t.Fatal(err)
		}
		data, err = os.ReadFile(source.Path)
		if err != nil || !bytes.Equal(data, f.media[i]) {
			t.Fatal("original mutation changed staged bytes")
		}
	}
	if first[0].Signature.Luma.Max != 25 || first[1].Signature.ZeroCrossingRate.Min != 0.027 {
		t.Fatal("predeclared signatures lost")
	}
	if !strings.Contains(strings.Join(cohort.PrivateValues(), "\n"), f.path) || !strings.Contains(strings.Join(cohort.PrivateValues(), "\n"), f.manifest.Channels[0].Programmes[0].SHA256) {
		t.Fatal("private audit inputs missing")
	}
}

func TestOperatorCohortRejectsInvalidDeclarations(t *testing.T) {
	for _, mode := range []string{"version", "channel order", "duplicate channel", "missing programme", "missing minimum", "missing maximum", "missing silence", "ambiguous signals", "reversed range", "absolute path", "escape path", "unclean path", "zero bytes", "oversized source", "bad digest", "uppercase digest", "conflicting source", "missing file", "directory", "symlink escape", "changed bytes", "wrong length"} {
		t.Run(mode, func(t *testing.T) {
			f := newOperatorCohortFixture(t)
			raw := &f.manifest.Channels[0].Programmes[0]
			switch mode {
			case "version":
				f.manifest.SchemaVersion = 2
			case "channel order":
				f.channels[0], f.channels[1] = f.channels[1], f.channels[0]
			case "duplicate channel":
				f.channels[1].ID = f.channels[0].ID
				f.manifest.Channels[1].ID = f.channels[0].ID
			case "missing programme":
				f.manifest.Channels[0].Programmes = f.manifest.Channels[0].Programmes[:1]
			case "missing minimum":
				raw.Signals.Luma.Min = nil
			case "missing maximum":
				raw.Signals.RMSDB.Max = nil
			case "missing silence":
				raw.Signals.Silence = nil
			case "ambiguous signals":
				f.manifest.Channels[0].Programmes[1].Signals = raw.Signals
			case "reversed range":
				*raw.Signals.Luma.Min = 30
			case "absolute path":
				raw.File = filepath.Join(f.root, raw.File)
			case "escape path":
				raw.File = "../private-source-0.media"
			case "unclean path":
				raw.File = "sub/../private-source-0.media"
			case "zero bytes":
				raw.Bytes = 0
			case "oversized source":
				raw.Bytes = operatorSourceLimit + 1
			case "bad digest":
				raw.SHA256 = "private-invalid-digest"
			case "uppercase digest":
				raw.SHA256 = strings.ToUpper(raw.SHA256)
			case "conflicting source":
				f.manifest.Channels[0].Programmes[1].File = raw.File
			case "missing file":
				raw.File = "private-missing.media"
			case "directory":
				raw.File = "private-directory"
				if err := os.Mkdir(filepath.Join(f.root, raw.File), 0700); err != nil {
					t.Fatal(err)
				}
			case "symlink escape":
				outside := filepath.Join(t.TempDir(), "private-outside.media")
				if err := os.WriteFile(outside, f.media[0], 0600); err != nil {
					t.Fatal(err)
				}
				raw.File = "private-link.media"
				if err := os.Symlink(outside, filepath.Join(f.root, raw.File)); err != nil {
					t.Fatal(err)
				}
			case "changed bytes", "wrong length":
				data := bytes.Repeat([]byte("z"), len(f.media[0]))
				if mode == "wrong length" {
					data = append(data, 'z')
				}
				if err := os.WriteFile(filepath.Join(f.root, raw.File), data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			f.write(t)
			cohort, err := LoadOperatorCohort(t.Context(), f.path, f.channels)
			if err == nil {
				_ = cohort.Close()
				t.Fatal("invalid cohort admitted")
			}
			if strings.Contains(err.Error(), "private-") || strings.Contains(err.Error(), f.root) {
				t.Fatal("error disclosed private input")
			}
		})
	}
}

func TestOperatorCohortRejectsAmbiguousOrOversizedJSON(t *testing.T) {
	for _, mode := range []string{"duplicate", "case duplicate", "unknown", "trailing", "oversized", "missing signal endpoint", "deep"} {
		t.Run(mode, func(t *testing.T) {
			f := newOperatorCohortFixture(t)
			body, err := os.ReadFile(f.path)
			if err != nil {
				t.Fatal(err)
			}
			raw := string(body)
			switch mode {
			case "duplicate":
				raw = strings.Replace(raw, `"schemaVersion":1`, `"schemaVersion":1,"schemaVersion":1`, 1)
			case "case duplicate":
				raw = strings.Replace(raw, `"schemaVersion":1`, `"schemaVersion":1,"SchemaVersion":1`, 1)
			case "unknown":
				raw = strings.Replace(raw, `"schemaVersion":1`, `"schemaVersion":1,"privateUnknown":true`, 1)
			case "trailing":
				raw += "{}"
			case "oversized":
				raw += strings.Repeat(" ", operatorManifestLimit)
			case "missing signal endpoint":
				raw = strings.Replace(raw, `"min":0,"max":25`, `"max":25`, 1)
			case "deep":
				raw = `{"privateUnknown":` + strings.Repeat("[", 16) + "0" + strings.Repeat("]", 16) + "}"
			}
			if err := os.WriteFile(f.path, []byte(raw), 0600); err != nil {
				t.Fatal(err)
			}
			cohort, err := LoadOperatorCohort(t.Context(), f.path, f.channels)
			if err == nil {
				_ = cohort.Close()
				t.Fatal("ambiguous or oversized JSON admitted")
			}
		})
	}
	f := newOperatorCohortFixture(t)
	body, err := os.ReadFile(f.path)
	if err != nil {
		t.Fatal(err)
	}
	body = append(body, bytes.Repeat([]byte(" "), operatorManifestLimit-len(body))...)
	if err := os.WriteFile(f.path, body, 0600); err != nil {
		t.Fatal(err)
	}
	cohort, err := LoadOperatorCohort(t.Context(), f.path, f.channels)
	if err != nil {
		t.Fatal("valid manifest at byte bound rejected:", err)
	}
	_ = cohort.Close()
}

func TestOperatorCohortBoundsBeforeReadingSources(t *testing.T) {
	for _, mode := range []string{"count", "total"} {
		t.Run(mode, func(t *testing.T) {
			f := newOperatorCohortFixture(t)
			base := f.manifest.Channels[0]
			f.manifest.Channels = nil
			f.channels = nil
			count := 33
			if mode == "total" {
				count = 3
			}
			for i := 0; i < count; i++ {
				entry := base
				entry.ID = fmt.Sprintf("channel-%d", i)
				entry.Programmes = append(entry.Programmes[:0:0], base.Programmes...)
				for j := range entry.Programmes {
					entry.Programmes[j].File = fmt.Sprintf("missing-%d", 2*i+j)
					if mode == "total" {
						entry.Programmes[j].Bytes = operatorSourceLimit
					}
				}
				f.manifest.Channels = append(f.manifest.Channels, entry)
				f.channels = append(f.channels, Channel{ID: entry.ID})
			}
			f.write(t)
			cohort, err := LoadOperatorCohort(t.Context(), f.path, f.channels)
			if err == nil {
				_ = cohort.Close()
				t.Fatal("resource bound ignored")
			}
			if err.Error() != "operator corpus exceeds bounds" {
				t.Fatalf("source read happened before resource preflight: %v", err)
			}
		})
	}
}

func TestOperatorCohortStagingRejectsChangedInputsAndRemovesPartialCopies(t *testing.T) {
	for _, mode := range []string{"same size mutation", "symlink replacement", "closed corpus", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			f := newOperatorCohortFixture(t)
			cohort, err := LoadOperatorCohort(t.Context(), f.path, f.channels)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = cohort.Close() }()
			ctx := t.Context()
			source := filepath.Join(f.root, f.manifest.Channels[0].Programmes[1].File)
			switch mode {
			case "same size mutation":
				if err := os.WriteFile(source, bytes.Repeat([]byte("z"), len(f.media[1])), 0600); err != nil {
					t.Fatal(err)
				}
			case "symlink replacement":
				outside := filepath.Join(t.TempDir(), "private-outside.media")
				if err := os.WriteFile(outside, f.media[1], 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(source); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, source); err != nil {
					t.Fatal(err)
				}
			case "closed corpus":
				_ = cohort.Close()
			case "canceled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			parent := t.TempDir()
			if _, err := cohort.Stage(ctx, parent); err == nil {
				t.Fatal("changed source staged")
			}
			entries, err := os.ReadDir(parent)
			if err != nil || len(entries) != 0 {
				t.Fatal("failed staging retained partial copies")
			}
		})
	}
}

func TestOperatorCohortRequiresLoadedInputBeforeStaging(t *testing.T) {
	for _, cohort := range []*OperatorCohort{nil, {}} {
		parent := t.TempDir()
		if _, err := cohort.Stage(t.Context(), parent); err == nil {
			t.Fatal("unloaded cohort staged")
		}
		entries, err := os.ReadDir(parent)
		if err != nil || len(entries) != 0 {
			t.Fatal("invalid cohort created output")
		}
	}
	f := newOperatorCohortFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if cohort, err := LoadOperatorCohort(ctx, f.path, f.channels); err == nil {
		_ = cohort.Close()
		t.Fatal("canceled cohort load succeeded")
	}
}

func TestOperatorCohortIdentityFreezesExactLoadedDocument(t *testing.T) {
	f := newOperatorCohortFixture(t)
	original, err := os.ReadFile(f.path)
	if err != nil {
		t.Fatal(err)
	}
	cohort, err := LoadOperatorCohort(t.Context(), f.path, f.channels)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cohort.Close() }()
	digest := sha256.Sum256(original)
	if cohort.ManifestSHA256() != hex.EncodeToString(digest[:]) {
		t.Fatal("identity does not bind exact loaded document")
	}
	changed := append([]byte("\n"), original...)
	if err := os.WriteFile(f.path, changed, 0600); err != nil {
		t.Fatal(err)
	}
	other, err := LoadOperatorCohort(t.Context(), f.path, f.channels)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = other.Close() }()
	if cohort.ManifestSHA256() == other.ManifestSHA256() || cohort.ManifestSHA256() != hex.EncodeToString(digest[:]) {
		t.Fatal("loaded identity changed or omitted exact input bytes")
	}
}
