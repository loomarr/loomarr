package api_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/api"
	"github.com/loomarr/loomarr/internal/clipcatalog"
	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/fillerstore"
	"github.com/loomarr/loomarr/internal/playout"
	"github.com/loomarr/loomarr/internal/schedule"
	"github.com/loomarr/loomarr/internal/store"
)

// renameFixture is a drop-folder holding one filename-named clip, catalogued as the scan would
// catalogue it, behind a server that applies that folder's layout.
func renameFixture(t *testing.T) (*apiHarness, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Widgetmax Ad 1991.mp4"), []byte("MP4BYTES"), 0o600); err != nil {
		t.Fatal(err)
	}
	layout, err := filler.NewLayout(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	h := startAPIHarness(t, func(defaults apiHarnessDefaults) http.Handler {
		return api.Router(defaults.Log, api.Options{
			Store: defaults.Store, Auth: defaults.Auth, Log: defaults.Log, FillerLayout: layout,
		})
	})
	seedFilenameNamedClip(t, h.Store)
	return h, dir
}

func seedFilenameNamedClip(t *testing.T, st fillerstore.Store) {
	t.Helper()
	if err := st.UpsertClip(context.Background(), store.Clip{Clip: filler.Clip{
		Hash: "h-widgetmax", Path: "Widgetmax Ad 1991.mp4", Name: "Widgetmax Ad 1991",
		NameSource: clipcatalog.NameFromFilename, Kind: filler.Commercial, DurationMs: 30_000,
	}, UpdatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
}

// An admin's rename (#1990) is trimmed, recorded as curated, and written to the sidecar that the
// next scan of the folder restores it from.
func TestPatchClip_RenameIsCuratedAndSurvivesARescan(t *testing.T) {
	h, dir := renameFixture(t)

	resp := h.Do(http.MethodPatch, "/v1/filler/tags", adminToken, `{"hash":"h-widgetmax","name":"  Widgetmax Turbo Blender  "}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("admin rename → %d", resp.StatusCode)
	}
	var body struct{ Name string }
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body.Name != "Widgetmax Turbo Blender" {
		t.Errorf("response name = %q, want the trimmed rename", body.Name)
	}
	got, err := h.Store.GetClip(context.Background(), "h-widgetmax")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Widgetmax Turbo Blender" || got.NameSource != clipcatalog.NameFromCurated {
		t.Errorf("catalog = %q (%q), want the rename as curated", got.Name, got.NameSource)
	}

	// The scan reading this back is TestScanDir_KeepsACuratedRename.
	tags, ok := filler.ReadSidecarTags(filepath.Join(dir, "Widgetmax Ad 1991.mp4"))
	if !ok || tags.DisplayName != "Widgetmax Turbo Blender" || tags.NameProvenance != string(clipcatalog.NameFromCurated) {
		t.Fatalf("sidecar = %+v (%v), want displayName with curated provenance", tags, ok)
	}
}

// An empty name is not a rename; a member cannot rename; an over-long name is refused. None of
// them touches the clip or its sidecar.
func TestPatchClip_RenameRefusals(t *testing.T) {
	h, dir := renameFixture(t)
	for _, tc := range []struct {
		name, token, body string
		want              int
	}{
		{"empty", adminToken, `{"hash":"h-widgetmax","name":""}`, http.StatusUnprocessableEntity},
		{"blank", adminToken, `{"hash":"h-widgetmax","name":"   "}`, http.StatusUnprocessableEntity},
		{"too long", adminToken, `{"hash":"h-widgetmax","name":"` + strings.Repeat("w", 121) + `"}`, http.StatusUnprocessableEntity},
		{"member", memberToken, `{"hash":"h-widgetmax","name":"Widgetmax Turbo Blender"}`, http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := do(t, h.Server, http.MethodPatch, "/v1/filler/tags", tc.token, tc.body)
			if resp.StatusCode != tc.want {
				t.Fatalf("→ %d, want %d", resp.StatusCode, tc.want)
			}
		})
	}
	got, err := h.Store.GetClip(context.Background(), "h-widgetmax")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Widgetmax Ad 1991" || got.NameSource != clipcatalog.NameFromFilename {
		t.Errorf("after refusals = %q (%q), want the clip untouched", got.Name, got.NameSource)
	}
	if _, err := os.Stat(filepath.Join(dir, "Widgetmax Ad 1991.info.json")); !os.IsNotExist(err) {
		t.Errorf("a refused rename wrote a sidecar (%v)", err)
	}
}

// The 120-character limit counts the trimmed name in characters, not bytes or raw input.
func TestPatchClip_RenameLengthCountsTheTrimmedName(t *testing.T) {
	h, _ := renameFixture(t)
	for _, name := range []string{" " + strings.Repeat("w", 120) + " ", strings.Repeat("é", 120)} {
		resp := h.Do(http.MethodPatch, "/v1/filler/tags", adminToken, `{"hash":"h-widgetmax","name":"`+name+`"}`)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("rename to a 120-character name (%d bytes raw) → %d, want 200", len(name), resp.StatusCode)
		}
	}
}

// A sidecar that cannot be updated refuses the whole edit: neither the name nor the era is saved.
func TestPatchClip_RenameWithAnUnreadableSidecarSavesNothing(t *testing.T) {
	h, dir := renameFixture(t)
	if err := os.WriteFile(filepath.Join(dir, "Widgetmax Ad 1991.info.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	resp := h.Do(http.MethodPatch, "/v1/filler/tags", adminToken, `{"hash":"h-widgetmax","era":1991,"name":"Widgetmax Turbo Blender"}`)
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("rename over an unreadable sidecar → %d, want 500", resp.StatusCode)
	}
	got, err := h.Store.GetClip(context.Background(), "h-widgetmax")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Widgetmax Ad 1991" || got.NameSource != clipcatalog.NameFromFilename || got.Era != 0 {
		t.Errorf("after the refused edit = %q (%q) era %d, want the clip untouched", got.Name, got.NameSource, got.Era)
	}
}

// storePods assembles a break from the catalog rows as they are now, the way the real assembler
// carries each clip's name and provenance into the pod.
type storePods struct {
	fakePods
	st     fillerstore.Store
	hashes []string
}

func (p *storePods) PreviewAt(ctx context.Context, _ string, _ int64) (filler.Pod, error) {
	return p.pod(ctx)
}

func (p *storePods) PreviewAtMany(ctx context.Context, _ string, starts []int64) ([]filler.Pod, error) {
	pod, err := p.pod(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]filler.Pod, len(starts))
	for i := range out {
		out[i] = pod
	}
	return out, nil
}

func (p *storePods) pod(ctx context.Context) (filler.Pod, error) {
	pod := filler.Pod{MatchLevel: filler.MatchExact}
	for _, h := range p.hashes {
		c, err := p.st.GetClip(ctx, h)
		if err != nil {
			return filler.Pod{}, err
		}
		pod.Entries = append(pod.Entries, filler.PodEntry{Hash: c.Hash, Name: c.Name, NameSource: c.NameSource,
			Kind: c.Kind, DurationMs: c.DurationMs})
		pod.TotalMs += c.DurationMs
	}
	return pod, nil
}

// A member watching a break sees the name an admin gave a filename-named clip (#1990), through
// the same viewer projection that withholds the filename (#1988).
func TestGuide_MemberSeesACuratedRenameDuringABreak(t *testing.T) {
	g := &fakeXMLTVGuide{withPending: map[string][]playout.Broadcast{
		"ch1": {gridBlock(schedule.SlotFiller, "", 0, 4)},
	}}
	pods := &storePods{hashes: []string{"h-widgetmax"}}
	harness := newGuideHarness(t, guideHarnessConfig{Guide: g, Pods: pods})
	pods.st = harness.Store
	seedGridChannel(t, harness.Store, "ch1", 1)
	seedFilenameNamedClip(t, harness.Store)

	breakName := func() string {
		t.Helper()
		resp := do(t, harness.Server, http.MethodGet, "/v1/guide", memberToken, "")
		defer func() { _ = resp.Body.Close() }()
		raw, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET /v1/guide as a member → %d", resp.StatusCode)
		}
		var body guideBody
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatal(err)
		}
		return body.Channels[0].Airings[0].Pod.Entries[0].Name
	}
	if name := breakName(); name != "" {
		t.Fatalf("before the rename the break showed %q, want the filename withheld", name)
	}
	if resp := do(t, harness.Server, http.MethodPatch, "/v1/filler/tags", adminToken,
		`{"hash":"h-widgetmax","name":"Widgetmax Turbo Blender"}`); resp.StatusCode != http.StatusOK {
		t.Fatalf("admin rename → %d", resp.StatusCode)
	}
	if name := breakName(); name != "Widgetmax Turbo Blender" {
		t.Errorf("after the rename the break showed %q, want the curated name", name)
	}
}
