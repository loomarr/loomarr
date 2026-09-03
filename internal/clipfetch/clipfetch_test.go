package clipfetch_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/clipfetch"
	"github.com/loomarr/loomarr/internal/filler"
)

// fakeDL records which sources it was handed and returns scripted results.
type fakeDL struct {
	got     []clipfetch.Source
	fetched int
	err     error
}

type outputDL struct{ missingSidecar bool }

func (d outputDL) Download(_ context.Context, _ clipfetch.Source, dir string) (clipfetch.DownloadResult, error) {
	media := filepath.Join(dir, "download.mp4")
	if err := os.WriteFile(media, []byte("downloaded video bytes"), 0o600); err != nil {
		return clipfetch.DownloadResult{}, err
	}
	digest, clipHash := strings.Repeat("a", 64), strings.Repeat("b", 64)
	output := clipfetch.Output{MediaPath: media, SHA256: digest, Bytes: 22, ClipHash: clipHash}
	if d.missingSidecar {
		output.Repair = "missing sidecar"
		return clipfetch.DownloadResult{Fetched: 1, Outputs: []clipfetch.Output{output}}, errors.New("missing sidecar")
	}
	sidecar := filepath.Join(dir, "download.info.json")
	if err := os.WriteFile(sidecar, []byte(`{"loomarr":{"fetchedBy":"loomarr"}}`), 0o600); err != nil {
		return clipfetch.DownloadResult{}, err
	}
	output.SidecarPath = sidecar
	return clipfetch.DownloadResult{Fetched: 1, Outputs: []clipfetch.Output{output}}, nil
}

type recordingArtifactWriter struct {
	t          *testing.T
	root       string
	snapshots  [][]filler.AcquisitionArtifact
	targetSeen bool
}

func (w *recordingArtifactWriter) UpsertAcquisitionArtifacts(_ context.Context, artifacts []filler.AcquisitionArtifact) error {
	if len(w.snapshots) == 0 {
		if _, err := os.Stat(filepath.Join(w.root, "download.mp4")); err == nil {
			w.targetSeen = true
		}
	}
	w.snapshots = append(w.snapshots, append([]filler.AcquisitionArtifact(nil), artifacts...))
	return nil
}

func (f *fakeDL) Download(_ context.Context, src clipfetch.Source, _ string) (clipfetch.DownloadResult, error) {
	f.got = append(f.got, src)
	if f.err != nil {
		return clipfetch.DownloadResult{}, f.err
	}
	return clipfetch.DownloadResult{Fetched: f.fetched}, nil
}

func discardLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestKindForURL(t *testing.T) {
	cases := map[string]clipfetch.Kind{
		"https://youtube.com/playlist?list=abc":      clipfetch.YouTube,
		"https://youtu.be/xyz":                       clipfetch.YouTube,
		"https://archive.org/details/classic-tv-ads": clipfetch.Archive,
		"https://www.someothersite.com/vid":          clipfetch.YouTube, // default yt-dlp
	}
	for url, want := range cases {
		if got := clipfetch.KindForURL(url); got != want {
			t.Errorf("KindForURL(%q) = %q, want %q", url, got, want)
		}
	}
}

// Run dispatches each source to the downloader for its kind.
func TestRun_DispatchesByKind(t *testing.T) {
	yt, arch := &fakeDL{fetched: 2}, &fakeDL{fetched: 5}
	ing := clipfetch.New(yt, arch, "/drop", discardLog())

	res := ing.Run(context.Background(), []clipfetch.Source{
		{Kind: clipfetch.YouTube, URL: "https://youtube.com/playlist?list=a"},
		{Kind: clipfetch.Archive, URL: "https://archive.org/details/x"},
	})
	if len(yt.got) != 1 || yt.got[0].Kind != clipfetch.YouTube {
		t.Errorf("youtube downloader got %+v", yt.got)
	}
	if len(arch.got) != 1 || arch.got[0].Kind != clipfetch.Archive {
		t.Errorf("archive downloader got %+v", arch.got)
	}
	if res.Fetched != 7 { // 2 + 5
		t.Errorf("fetched = %d, want 7", res.Fetched)
	}
}

// A failing source is counted and logged, never fatal — the rest still run.
func TestRun_ResilientToSourceFailure(t *testing.T) {
	yt := &fakeDL{err: errors.New("playlist gone")}
	arch := &fakeDL{fetched: 3}
	ing := clipfetch.New(yt, arch, "/drop", discardLog())

	res := ing.Run(context.Background(), []clipfetch.Source{
		{Kind: clipfetch.YouTube, URL: "https://youtube.com/bad"},
		{Kind: clipfetch.Archive, URL: "https://archive.org/details/good"},
	})
	if res.Failed != 1 {
		t.Errorf("failed = %d, want 1", res.Failed)
	}
	if res.Fetched != 3 {
		t.Errorf("the good source should still run: fetched = %d, want 3", res.Fetched)
	}
}

// A source that returns no clips AND no error (a nonexistent/typo'd Archive id —
// Archive serves 200 {} for unknown items — or an empty source) must be counted
// as Empty, not silently reported as success. Otherwise the operator sees
// "fetched:0 failed:0" with no reason why nothing landed.
func TestRun_EmptySourceIsSurfaced(t *testing.T) {
	empty := &fakeDL{fetched: 0} // no error, nothing fetched — the silent case
	good := &fakeDL{fetched: 2}
	ing := clipfetch.New(empty, good, "/drop", discardLog())

	res := ing.Run(context.Background(), []clipfetch.Source{
		{Kind: clipfetch.YouTube, URL: "https://youtube.com/nonexistent"},
		{Kind: clipfetch.Archive, URL: "https://archive.org/details/real"},
	})
	if res.Empty != 1 {
		t.Errorf("empty = %d, want 1 (the yield-nothing source surfaced)", res.Empty)
	}
	if res.Failed != 0 {
		t.Errorf("an empty source is not a failure: failed = %d, want 0", res.Failed)
	}
	if res.Fetched != 2 {
		t.Errorf("the good source still runs: fetched = %d, want 2", res.Fetched)
	}
}

func TestRun_PersistsExactManifestBeforePublishing(t *testing.T) {
	dir := t.TempDir()
	writer := &recordingArtifactWriter{t: t, root: dir}
	ing := clipfetch.New(outputDL{}, nil, dir, discardLog()).WithArtifactWriter(writer)

	res := ing.Run(t.Context(), []clipfetch.Source{{
		ID: "youtube:classic", AcquisitionID: "acq-1", Kind: clipfetch.YouTube,
		URL: "https://youtube.com/watch?v=one", RemoteID: "one",
	}})
	if res.Failed != 0 || res.Fetched != 1 || len(res.Artifacts) != 1 {
		t.Fatalf("result = %+v, want one published artifact", res)
	}
	if writer.targetSeen {
		t.Fatal("download became intake-visible before its manifest was durable")
	}
	if len(writer.snapshots) != 2 || writer.snapshots[0][0].State != filler.ArtifactStaged || writer.snapshots[1][0].State != filler.ArtifactPublished {
		t.Fatalf("manifest snapshots = %+v, want staged then published", writer.snapshots)
	}
	if res.Artifacts[0].RemoteID != "one" {
		t.Fatalf("artifact remote id = %q, want one", res.Artifacts[0].RemoteID)
	}
	if _, err := os.Stat(filepath.Join(dir, "download.mp4")); err != nil {
		t.Fatalf("published media: %v", err)
	}
}

func TestRun_MissingProvenanceRemainsInHiddenRepairQuarantine(t *testing.T) {
	dir := t.TempDir()
	writer := &recordingArtifactWriter{t: t, root: dir}
	ing := clipfetch.New(outputDL{missingSidecar: true}, nil, dir, discardLog()).WithArtifactWriter(writer)

	res := ing.Run(t.Context(), []clipfetch.Source{{
		ID: "youtube:classic", AcquisitionID: "acq-1", Kind: clipfetch.YouTube,
		URL: "https://youtube.com/watch?v=one",
	}})
	if res.Failed != 1 || len(res.Artifacts) != 1 || res.Artifacts[0].State != filler.ArtifactRepair {
		t.Fatalf("result = %+v, want one repair artifact", res)
	}
	if _, err := os.Stat(filepath.Join(dir, "download.mp4")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("repair media became intake-visible: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, res.Artifacts[0].StagingPath)); err != nil {
		t.Fatalf("repair media was not retained for recovery: %v", err)
	}
}
