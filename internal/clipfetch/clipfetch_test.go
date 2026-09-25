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
	"github.com/loomarr/loomarr/internal/storagegovernor"
	"github.com/loomarr/loomarr/internal/store"
	"github.com/loomarr/loomarr/internal/testkit"
	"github.com/loomarr/loomarr/internal/testkit/recordfixture"
)

type downloaderFunc func(context.Context, clipfetch.Source, string) (clipfetch.DownloadResult, error)

func (f downloaderFunc) Download(ctx context.Context, src clipfetch.Source, dir string) (clipfetch.DownloadResult, error) {
	return f(ctx, src, dir)
}

type estimatingDownloader struct {
	budget   storagegovernor.MediaBudget
	estimate error
	download downloaderFunc
}

func (d estimatingDownloader) Estimate(context.Context, clipfetch.Source) (storagegovernor.MediaBudget, error) {
	return d.budget, d.estimate
}

func (d estimatingDownloader) Download(ctx context.Context, source clipfetch.Source, dir string) (clipfetch.DownloadResult, error) {
	return d.download(ctx, source, dir)
}

type capacityMeter struct {
	measurement storagegovernor.Measurement
	managed     int64
}

func (m *capacityMeter) Measure(context.Context, string) (storagegovernor.Measurement, error) {
	return m.measurement, nil
}

func (m *capacityMeter) ManagedBytes(context.Context, string, storagegovernor.Domain) (int64, error) {
	return m.managed, nil
}

type artifactWriterFunc func(context.Context, []filler.AcquisitionArtifact) error

type downloadRequest struct {
	source clipfetch.Source
	dir    string
}

func (f artifactWriterFunc) UpsertAcquisitionArtifacts(ctx context.Context, artifacts []filler.AcquisitionArtifact) error {
	return f(ctx, artifacts)
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
	var yt, arch recordfixture.Recorder[clipfetch.Source, clipfetch.DownloadResult]
	yt.Respond = func(clipfetch.Source) (clipfetch.DownloadResult, error) {
		return clipfetch.DownloadResult{Fetched: 2}, nil
	}
	arch.Respond = func(clipfetch.Source) (clipfetch.DownloadResult, error) {
		return clipfetch.DownloadResult{Fetched: 5}, nil
	}
	ing := clipfetch.New(downloaderFunc(func(_ context.Context, src clipfetch.Source, attemptDir string) (clipfetch.DownloadResult, error) {
		return yt.Call(src)
	}), downloaderFunc(func(_ context.Context, src clipfetch.Source, _ string) (clipfetch.DownloadResult, error) {
		return arch.Call(src)
	}), "/drop", discardLog())

	res := ing.Run(context.Background(), []clipfetch.Source{
		{Kind: clipfetch.YouTube, URL: "https://youtube.com/playlist?list=a"},
		{Kind: clipfetch.Archive, URL: "https://archive.org/details/x"},
	})
	if len(yt.Inputs()) != 1 || yt.Inputs()[0].Kind != clipfetch.YouTube {
		t.Errorf("youtube downloader got %+v", yt.Inputs())
	}
	if len(arch.Inputs()) != 1 || arch.Inputs()[0].Kind != clipfetch.Archive {
		t.Errorf("archive downloader got %+v", arch.Inputs())
	}
	if res.Fetched != 7 { // 2 + 5
		t.Errorf("fetched = %d, want 7", res.Fetched)
	}
}

func TestPrepareReservesBeforeDownloadAndRevalidatesBeforeExecution(t *testing.T) {
	t.Parallel()
	meter := &capacityMeter{measurement: storagegovernor.Measurement{
		ID: "disk", TotalBytes: 64 * storagegovernor.GiB, FreeBytes: 20 * storagegovernor.GiB,
	}}
	governor := storagegovernor.New(meter, func(storagegovernor.Domain) storagegovernor.Policy {
		return storagegovernor.Policy{SoftBudgetBytes: 10 * storagegovernor.GiB}
	})
	downloaded := false
	downloader := estimatingDownloader{
		budget: storagegovernor.MediaBudget{WriteCeilingBytes: storagegovernor.GiB, ReservationBytes: 4 * storagegovernor.GiB},
		download: func(context.Context, clipfetch.Source, string) (clipfetch.DownloadResult, error) {
			downloaded = true
			return clipfetch.DownloadResult{Fetched: 1}, nil
		},
	}
	ingestor := clipfetch.New(downloader, nil, "/filler", discardLog()).WithStorageGovernor(governor)
	plan, err := ingestor.Prepare(t.Context(), []clipfetch.Source{{Kind: clipfetch.YouTube, URL: "https://example.invalid/one"}}, storagegovernor.Automatic)
	if err != nil {
		t.Fatal(err)
	}
	if downloaded {
		t.Fatal("Prepare downloaded media before the caller durably queued the run")
	}
	// The host changed after queue admission. Revalidation must stop execution.
	meter.measurement.FreeBytes = 7 * storagegovernor.GiB
	result := plan.Run(t.Context())
	if downloaded || result.Failed != 1 {
		t.Fatalf("run = %+v downloaded=%v, want capacity refusal before execution", result, downloaded)
	}
	if snapshot := governor.Snapshot(t.Context(), "/filler"); snapshot.Snapshot.ReservedBytes != 0 {
		t.Fatalf("reservation after run = %d, want released", snapshot.Snapshot.ReservedBytes)
	}
}

func TestPrepareRollsBackEarlierReservationsWhenBatchCannotFit(t *testing.T) {
	t.Parallel()
	meter := &capacityMeter{measurement: storagegovernor.Measurement{
		ID: "disk", TotalBytes: 128 * storagegovernor.GiB, FreeBytes: 80 * storagegovernor.GiB,
	}}
	governor := storagegovernor.New(meter, func(storagegovernor.Domain) storagegovernor.Policy {
		return storagegovernor.Policy{SoftBudgetBytes: 6 * storagegovernor.GiB}
	})
	downloader := estimatingDownloader{
		budget: storagegovernor.MediaBudget{WriteCeilingBytes: storagegovernor.GiB, ReservationBytes: 4 * storagegovernor.GiB},
		download: func(context.Context, clipfetch.Source, string) (clipfetch.DownloadResult, error) {
			t.Fatal("download ran for a batch that could not be reserved")
			return clipfetch.DownloadResult{}, nil
		},
	}
	ingestor := clipfetch.New(downloader, nil, "/filler", discardLog()).WithStorageGovernor(governor)
	_, err := ingestor.Prepare(t.Context(), []clipfetch.Source{
		{Kind: clipfetch.YouTube, URL: "https://example.invalid/one"},
		{Kind: clipfetch.YouTube, URL: "https://example.invalid/two"},
	}, storagegovernor.Automatic)
	var capacityErr *clipfetch.CapacityError
	if !errors.As(err, &capacityErr) || capacityErr.Decision.Snapshot.Reason != storagegovernor.ReasonLibraryLimit {
		t.Fatalf("Prepare error = %v, want library_limit", err)
	}
	if snapshot := governor.Snapshot(t.Context(), "/filler"); snapshot.Snapshot.ReservedBytes != 0 {
		t.Fatalf("rolled-back reservation = %d, want zero", snapshot.Snapshot.ReservedBytes)
	}
}

func TestPreparedYtDlpOverrunStopsProcessAndRemovesPrivateStaging(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	executable := testkit.Executable(t, "fake-yt-dlp", `#!/bin/sh
case "$*" in
  *--simulate*) printf '%s\n' '{"filesize":1,"duration":1,"height":480}'; exit 0 ;;
esac
result=""
while test "$#" -gt 0; do
  case "$1" in
    --print-to-file) result="$3"; shift 3 ;;
    *) shift ;;
  esac
done
stage=$(dirname "$result")
truncate -s 40000000 "$stage/too-large.mp4"
sleep 2
`)
	governor, err := storagegovernor.NewFilesystem([]storagegovernor.ManagedRoot{{
		Path: root, Domain: storagegovernor.DomainFiller,
	}}, func(storagegovernor.Domain) storagegovernor.Policy {
		return storagegovernor.Policy{SoftBudgetBytes: storagegovernor.GiB}
	})
	if err != nil {
		t.Fatal(err)
	}
	ingestor := clipfetch.New(clipfetch.NewYtDlpDownloader(executable, "ffmpeg"), nil, root, discardLog()).
		WithArtifactWriter(artifactWriterFunc(func(context.Context, []filler.AcquisitionArtifact) error { return nil })).
		WithStorageGovernor(governor)
	plan, err := ingestor.Prepare(t.Context(), []clipfetch.Source{{
		ID: "youtube:test", AcquisitionID: "overrun", Kind: clipfetch.YouTube,
		URL: "https://youtube.com/watch?v=too-large",
	}}, storagegovernor.Automatic)
	if err != nil {
		t.Fatal(err)
	}
	result := plan.Run(t.Context())
	if result.Failed != 1 || result.Fetched != 0 || len(result.Artifacts) != 0 {
		t.Fatalf("overrun result = %+v, want one failed source and no published artifact", result)
	}
	stage := filepath.Join(root, ".loomarr-acquisitions", "overrun", "000")
	if _, err := os.Stat(stage); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unsafe private staging remains after overrun: %v", err)
	}
}

// A failing source is counted and logged, never fatal — the rest still run.
func TestRun_ResilientToSourceFailure(t *testing.T) {
	var yt, arch recordfixture.Recorder[clipfetch.Source, clipfetch.DownloadResult]
	yt.Respond = func(clipfetch.Source) (clipfetch.DownloadResult, error) {
		return clipfetch.DownloadResult{}, errors.New("playlist gone")
	}
	arch.Respond = func(clipfetch.Source) (clipfetch.DownloadResult, error) {
		return clipfetch.DownloadResult{Fetched: 3}, nil
	}
	ing := clipfetch.New(downloaderFunc(func(_ context.Context, src clipfetch.Source, attemptDir string) (clipfetch.DownloadResult, error) {
		return yt.Call(src)
	}), downloaderFunc(func(_ context.Context, src clipfetch.Source, _ string) (clipfetch.DownloadResult, error) {
		return arch.Call(src)
	}), "/drop", discardLog())

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
	var empty, good recordfixture.Recorder[clipfetch.Source, clipfetch.DownloadResult]
	empty.Respond = func(clipfetch.Source) (clipfetch.DownloadResult, error) { return clipfetch.DownloadResult{}, nil } // no error, nothing fetched — the silent case
	good.Respond = func(clipfetch.Source) (clipfetch.DownloadResult, error) {
		return clipfetch.DownloadResult{Fetched: 2}, nil
	}
	ing := clipfetch.New(downloaderFunc(func(_ context.Context, src clipfetch.Source, attemptDir string) (clipfetch.DownloadResult, error) {
		return empty.Call(src)
	}), downloaderFunc(func(_ context.Context, src clipfetch.Source, _ string) (clipfetch.DownloadResult, error) {
		return good.Call(src)
	}), "/drop", discardLog())

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
	var writer recordfixture.Recorder[[]filler.AcquisitionArtifact, struct{}]
	targetSeen := false
	writer.Respond = func(artifacts []filler.AcquisitionArtifact) (struct{}, error) {
		if writer.Calls() == 1 {
			if _, err := os.Stat(filepath.Join(dir, "download.mp4")); err == nil {
				targetSeen = true
			}
		}
		return struct{}{}, nil
	}
	var downloader recordfixture.Recorder[downloadRequest, clipfetch.DownloadResult]
	downloader.Respond = func(req downloadRequest) (clipfetch.DownloadResult, error) {
		media := filepath.Join(req.dir, "download.mp4")
		if err := os.WriteFile(media, []byte("downloaded video bytes"), 0o600); err != nil {
			return clipfetch.DownloadResult{}, err
		}
		digest, clipHash := strings.Repeat("a", 64), strings.Repeat("b", 64)
		sidecar := filepath.Join(req.dir, "download.info.json")
		if err := os.WriteFile(sidecar, []byte(`{"loomarr":{"fetchedBy":"loomarr"}}`), 0o600); err != nil {
			return clipfetch.DownloadResult{}, err
		}
		return clipfetch.DownloadResult{Fetched: 1, Outputs: []clipfetch.Output{{MediaPath: media, SidecarPath: sidecar, SHA256: digest, Bytes: 22, ClipHash: clipHash}}}, nil
	}
	ing := clipfetch.New(nil, downloaderFunc(func(_ context.Context, src clipfetch.Source, attemptDir string) (clipfetch.DownloadResult, error) {
		return downloader.Call(downloadRequest{source: src, dir: attemptDir})
	}), dir, discardLog()).WithArtifactWriter(artifactWriterFunc(func(ctx context.Context, artifacts []filler.AcquisitionArtifact) error {
		_, err := writer.Call(append([]filler.AcquisitionArtifact(nil), artifacts...))
		return err
	}))

	res := ing.Run(t.Context(), []clipfetch.Source{{
		ID: "archive:classic", AcquisitionID: "acq-1", Kind: clipfetch.Archive,
		URL: "https://archive.org/details/one", RemoteID: "one",
	}})
	if res.Failed != 0 || res.Fetched != 1 || len(res.Artifacts) != 1 {
		t.Fatalf("result = %+v, want one published artifact", res)
	}
	if targetSeen {
		t.Fatal("download became intake-visible before its manifest was durable")
	}
	snapshots := writer.Inputs()
	if len(snapshots) != 2 || snapshots[0][0].State != filler.ArtifactStaged || snapshots[1][0].State != filler.ArtifactPublished {
		t.Fatalf("manifest snapshots = %+v, want staged then published", snapshots)
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
	var writer recordfixture.Recorder[[]filler.AcquisitionArtifact, struct{}]
	writer.Respond = func([]filler.AcquisitionArtifact) (struct{}, error) { return struct{}{}, nil }
	var downloader recordfixture.Recorder[downloadRequest, clipfetch.DownloadResult]
	downloader.Respond = func(req downloadRequest) (clipfetch.DownloadResult, error) {
		media := filepath.Join(req.dir, "download.mp4")
		if err := os.WriteFile(media, []byte("downloaded video bytes"), 0o600); err != nil {
			return clipfetch.DownloadResult{}, err
		}
		return clipfetch.DownloadResult{Fetched: 1, Outputs: []clipfetch.Output{{MediaPath: media, SHA256: strings.Repeat("a", 64), Bytes: 22, ClipHash: strings.Repeat("b", 64), Repair: "missing sidecar"}}}, errors.New("missing sidecar")
	}
	ing := clipfetch.New(downloaderFunc(func(_ context.Context, src clipfetch.Source, attemptDir string) (clipfetch.DownloadResult, error) {
		return downloader.Call(downloadRequest{source: src, dir: attemptDir})
	}), nil, dir, discardLog()).WithArtifactWriter(artifactWriterFunc(func(ctx context.Context, artifacts []filler.AcquisitionArtifact) error {
		_, err := writer.Call(append([]filler.AcquisitionArtifact(nil), artifacts...))
		return err
	}))

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

// The per-attempt archive must be reset from the durable shared archive. A failed manifest
// write therefore retains its private bytes and archive for repair without poisoning the retry.
func TestRun_YtDlpArchiveCommitsOnlyAfterDurableManifest(t *testing.T) {
	dir := t.TempDir()
	dsn := "sqlite://" + filepath.Join(t.TempDir(), "acquisitions.db")
	source := clipfetch.Source{ID: "youtube:current", AcquisitionID: "acq-current", Kind: clipfetch.YouTube, URL: "https://youtube.com/watch?v=current-id"}
	ytdlp := testkit.Executable(t, "yt-dlp", `#!/bin/sh
archive=""; result=""
while test "$#" -gt 0; do case "$1" in --download-archive) archive="$2"; shift 2 ;; --print-to-file) result="$3"; shift 3 ;; *) shift ;; esac; done
if test -s "$archive"; then exit 0; fi
stage=$(dirname "$result")
printf 'video bytes' > "$stage/download.mp4"
printf '{"id":"current-id","title":"download"}\n' > "$stage/download.info.json"
printf 'youtube current-id\n' > "$archive"
printf 'current-id\t"%s"\n' "$stage/download.mp4" >> "$result"
`)
	closed, err := store.Open(t.Context(), dsn, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := closed.UpsertAcquisitionRun(t.Context(), filler.AcquisitionRun{ID: source.AcquisitionID, SourceID: source.ID, Trigger: filler.AcquisitionPull, Status: filler.AcquisitionRunning, Requested: 1}); err != nil {
		t.Fatal(err)
	}
	if err := closed.Close(); err != nil {
		t.Fatal(err)
	}
	first := clipfetch.New(clipfetch.NewYtDlpDownloader(ytdlp, "ffmpeg"), nil, dir, discardLog()).WithArtifactWriter(closed).Run(t.Context(), []clipfetch.Source{source})
	if first.Failed != 1 {
		t.Fatalf("first = %+v, want manifest failure", first)
	}
	attemptArchive := filepath.Join(dir, ".loomarr-acquisitions", source.AcquisitionID, "000", ".yt-dlp-archive.txt")
	if _, err := os.Stat(attemptArchive); err != nil {
		t.Fatalf("failed attempt archive was not retained: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".yt-dlp-archive.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed manifest advanced shared archive: %v", err)
	}

	reopened, err := store.Open(t.Context(), dsn, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	retry := clipfetch.New(clipfetch.NewYtDlpDownloader(ytdlp, "ffmpeg"), nil, dir, discardLog()).WithArtifactWriter(reopened).Run(t.Context(), []clipfetch.Source{source})
	if retry.Fetched != 1 || len(retry.Artifacts) != 1 || retry.Artifacts[0].State != filler.ArtifactPublished {
		t.Fatalf("retry = %+v, want one published artifact", retry)
	}
	if _, err := os.Stat(filepath.Join(dir, "download.mp4")); err != nil {
		t.Fatalf("retry did not publish media: %v", err)
	}
	archive, err := os.ReadFile(filepath.Join(dir, ".yt-dlp-archive.txt"))
	if err != nil || string(archive) != "youtube current-id\n" {
		t.Fatalf("shared archive = %q, %v", archive, err)
	}

	second := source
	second.AcquisitionID = "acq-second"
	if err := reopened.UpsertAcquisitionRun(t.Context(), filler.AcquisitionRun{ID: second.AcquisitionID, SourceID: second.ID, Trigger: filler.AcquisitionPull, Status: filler.AcquisitionRunning, Requested: 1}); err != nil {
		t.Fatal(err)
	}
	deduped := clipfetch.New(clipfetch.NewYtDlpDownloader(ytdlp, "ffmpeg"), nil, dir, discardLog()).WithArtifactWriter(reopened).Run(t.Context(), []clipfetch.Source{second})
	if deduped.Empty != 1 || deduped.Fetched != 0 || len(deduped.Artifacts) != 0 {
		t.Fatalf("deduped retry = %+v, want shared archive skip", deduped)
	}
}

func TestYtDlpArchiveCommitExcludesUnreportedOutputs(t *testing.T) {
	dir := t.TempDir()
	attempt := filepath.Join(dir, "attempt")
	if err := os.MkdirAll(attempt, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(attempt, ".yt-dlp-archive.txt"), []byte("youtube owned-id\nyoutube omitted-id\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	d := clipfetch.NewYtDlpDownloader("yt-dlp", "ffmpeg")
	if err := d.CommitArchive(attempt, dir, []clipfetch.Output{{ArchiveID: "owned-id", ArchiveEntry: "youtube owned-id"}}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, ".yt-dlp-archive.txt"))
	if err != nil || string(got) != "youtube owned-id\n" {
		t.Fatalf("shared archive = %q, %v", got, err)
	}
}

// perSourceDownloader lets a batch mix estimable and unestimable sources, which is the
// production shape from #1394: one archive item in a batch has no size or duration metadata.
type perSourceDownloader struct {
	budgets  map[string]storagegovernor.MediaBudget
	estimate map[string]error
	download downloaderFunc
}

func (d perSourceDownloader) Estimate(_ context.Context, source clipfetch.Source) (storagegovernor.MediaBudget, error) {
	if err := d.estimate[source.URL]; err != nil {
		return storagegovernor.MediaBudget{}, err
	}
	return d.budgets[source.URL], nil
}

func (d perSourceDownloader) Download(ctx context.Context, source clipfetch.Source, dir string) (clipfetch.DownloadResult, error) {
	return d.download(ctx, source, dir)
}

// One item with no size estimate used to fail the whole batch (Prepare released everything and
// returned estimate_unknown), and because selection is deterministic the same item was picked on
// every retry. The unestimable item must instead get a bounded fallback and the batch proceed.
func TestPrepareQueuesTheRestOfABatchWhenOneItemHasNoEstimate(t *testing.T) {
	t.Parallel()
	meter := &capacityMeter{measurement: storagegovernor.Measurement{
		ID: "disk", TotalBytes: 600 * storagegovernor.GiB, FreeBytes: 400 * storagegovernor.GiB,
	}}
	governor := storagegovernor.New(meter, func(storagegovernor.Domain) storagegovernor.Policy {
		return storagegovernor.Policy{SoftBudgetBytes: 20 * storagegovernor.GiB}
	})
	good := storagegovernor.MediaBudget{WriteCeilingBytes: storagegovernor.GiB, ReservationBytes: storagegovernor.GiB}
	var downloaded []string
	downloader := perSourceDownloader{
		budgets: map[string]storagegovernor.MediaBudget{
			"https://archive.example/one": good, "https://archive.example/three": good,
		},
		estimate: map[string]error{"https://archive.example/two": clipfetch.ErrEstimateUnavailable},
		download: func(_ context.Context, source clipfetch.Source, _ string) (clipfetch.DownloadResult, error) {
			downloaded = append(downloaded, source.URL)
			return clipfetch.DownloadResult{Fetched: 1}, nil
		},
	}
	ingestor := clipfetch.New(nil, downloader, "/filler", discardLog()).WithStorageGovernor(governor)
	plan, err := ingestor.Prepare(t.Context(), []clipfetch.Source{
		{Kind: clipfetch.Archive, URL: "https://archive.example/one"},
		{Kind: clipfetch.Archive, URL: "https://archive.example/two"},
		{Kind: clipfetch.Archive, URL: "https://archive.example/three"},
	}, storagegovernor.Automatic)
	if err != nil {
		t.Fatalf("Prepare failed the whole batch for one unestimable item: %v", err)
	}
	result := plan.Run(t.Context())
	if len(downloaded) != 3 || result.Fetched != 3 || result.Failed != 0 {
		t.Fatalf("downloaded = %v result = %+v, want every item to proceed", downloaded, result)
	}
}

// The fallback for an unknown estimate is a byte CAP enforced during the download, not a blind
// download: an item that turns out larger than the cap is aborted and leaves nothing behind.
func TestPrepareCapsAnUnknownEstimateDownloadAndAbortsOnOverrun(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	executable := testkit.Executable(t, "fake-yt-dlp", `#!/bin/sh
case "$*" in
  *--simulate*) printf '%s\n' '{}'; exit 0 ;;
esac
result=""
while test "$#" -gt 0; do
  case "$1" in
    --print-to-file) result="$3"; shift 3 ;;
    *) shift ;;
  esac
done
stage=$(dirname "$result")
truncate -s 2000000000 "$stage/unbounded.mp4"
sleep 2
`)
	governor, err := storagegovernor.NewFilesystem([]storagegovernor.ManagedRoot{{
		Path: root, Domain: storagegovernor.DomainFiller,
	}}, func(storagegovernor.Domain) storagegovernor.Policy {
		return storagegovernor.Policy{SoftBudgetBytes: 20 * storagegovernor.GiB}
	})
	if err != nil {
		t.Fatal(err)
	}
	ingestor := clipfetch.New(clipfetch.NewYtDlpDownloader(executable, "ffmpeg"), nil, root, discardLog()).
		WithArtifactWriter(artifactWriterFunc(func(context.Context, []filler.AcquisitionArtifact) error { return nil })).
		WithStorageGovernor(governor)
	plan, err := ingestor.Prepare(t.Context(), []clipfetch.Source{{
		ID: "youtube:test", AcquisitionID: "unknown-size", Kind: clipfetch.YouTube,
		URL: "https://youtube.com/watch?v=no-metadata",
	}}, storagegovernor.Automatic)
	if err != nil {
		t.Fatalf("Prepare refused an item whose size is unknown instead of capping it: %v", err)
	}
	result := plan.Run(t.Context())
	if result.Failed != 1 || result.Fetched != 0 || len(result.Artifacts) != 0 {
		t.Fatalf("overrun result = %+v, want one aborted source and no published artifact", result)
	}
	stage := filepath.Join(root, ".loomarr-acquisitions", "unknown-size", "000")
	if _, err := os.Stat(stage); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("staging remains after the cap aborted the download: %v", err)
	}
}

// Each item's reservation is released as soon as ITS download ends (success or failure), so a
// finished item never keeps holding budget while the rest of the batch runs.
func TestPlanReleasesEachReservationWhenItsItemFinishes(t *testing.T) {
	t.Parallel()
	meter := &capacityMeter{measurement: storagegovernor.Measurement{
		ID: "disk", TotalBytes: 600 * storagegovernor.GiB, FreeBytes: 400 * storagegovernor.GiB,
	}}
	governor := storagegovernor.New(meter, func(storagegovernor.Domain) storagegovernor.Policy {
		return storagegovernor.Policy{SoftBudgetBytes: 20 * storagegovernor.GiB}
	})
	budget := storagegovernor.MediaBudget{WriteCeilingBytes: storagegovernor.GiB, ReservationBytes: storagegovernor.GiB}
	var reservedDuring []int64
	downloader := estimatingDownloader{
		budget: budget,
		download: func(_ context.Context, source clipfetch.Source, _ string) (clipfetch.DownloadResult, error) {
			reservedDuring = append(reservedDuring, governor.Snapshot(t.Context(), "/filler").Snapshot.ReservedBytes)
			if strings.HasSuffix(source.URL, "/one") {
				return clipfetch.DownloadResult{}, errors.New("provider refused")
			}
			return clipfetch.DownloadResult{Fetched: 1}, nil
		},
	}
	ingestor := clipfetch.New(nil, downloader, "/filler", discardLog()).WithStorageGovernor(governor)
	plan, err := ingestor.Prepare(t.Context(), []clipfetch.Source{
		{Kind: clipfetch.Archive, URL: "https://archive.example/one"},
		{Kind: clipfetch.Archive, URL: "https://archive.example/two"},
		{Kind: clipfetch.Archive, URL: "https://archive.example/three"},
	}, storagegovernor.Automatic)
	if err != nil {
		t.Fatal(err)
	}
	if held := governor.Snapshot(t.Context(), "/filler").Snapshot.ReservedBytes; held != 3*storagegovernor.GiB {
		t.Fatalf("reserved after Prepare = %d, want the sum of the three item reservations", held)
	}
	plan.Run(t.Context())
	want := []int64{3 * storagegovernor.GiB, 2 * storagegovernor.GiB, storagegovernor.GiB}
	for index := range want {
		if reservedDuring[index] != want[index] {
			t.Fatalf("reserved while item %d ran = %v, want %v (finished items release)", index, reservedDuring, want)
		}
	}
	if held := governor.Snapshot(t.Context(), "/filler").Snapshot.ReservedBytes; held != 0 {
		t.Fatalf("reserved after Run = %d, want zero", held)
	}
}

// An item Download would skip (nothing to fetch) is dropped and counted, never reserved, and it
// does not stop its neighbours.
func TestPrepareDropsAnItemWithNothingToFetchWithoutReservingIt(t *testing.T) {
	t.Parallel()
	meter := &capacityMeter{measurement: storagegovernor.Measurement{
		ID: "disk", TotalBytes: 600 * storagegovernor.GiB, FreeBytes: 400 * storagegovernor.GiB,
	}}
	governor := storagegovernor.New(meter, func(storagegovernor.Domain) storagegovernor.Policy {
		return storagegovernor.Policy{SoftBudgetBytes: 20 * storagegovernor.GiB}
	})
	good := storagegovernor.MediaBudget{WriteCeilingBytes: storagegovernor.GiB, ReservationBytes: storagegovernor.GiB}
	var downloaded []string
	downloader := perSourceDownloader{
		budgets: map[string]storagegovernor.MediaBudget{
			"https://archive.example/one": good, "https://archive.example/three": good,
		},
		estimate: map[string]error{"https://archive.example/two": clipfetch.ErrNothingToFetch},
		download: func(_ context.Context, source clipfetch.Source, _ string) (clipfetch.DownloadResult, error) {
			downloaded = append(downloaded, source.URL)
			return clipfetch.DownloadResult{Fetched: 1}, nil
		},
	}
	ingestor := clipfetch.New(nil, downloader, "/filler", discardLog()).WithStorageGovernor(governor)
	plan, err := ingestor.Prepare(t.Context(), []clipfetch.Source{
		{Kind: clipfetch.Archive, URL: "https://archive.example/one"},
		{Kind: clipfetch.Archive, URL: "https://archive.example/two"},
		{Kind: clipfetch.Archive, URL: "https://archive.example/three"},
	}, storagegovernor.Automatic)
	if err != nil {
		t.Fatal(err)
	}
	if held := governor.Snapshot(t.Context(), "/filler").Snapshot.ReservedBytes; held != 2*storagegovernor.GiB {
		t.Fatalf("reserved = %d, want two items' worth (the skipped one reserves nothing)", held)
	}
	result := plan.Run(t.Context())
	if len(downloaded) != 2 || result.Fetched != 2 || result.Skipped != 1 || result.Failed != 0 {
		t.Fatalf("downloaded = %v result = %+v, want 2 fetched and 1 skipped", downloaded, result)
	}
}

// A provider outage that fails every estimate is still an error (so the source backs off), and it
// is a plain estimate failure, not a storage pause.
func TestPrepareFailsWhenEveryEstimateFailsForAnotherReason(t *testing.T) {
	t.Parallel()
	meter := &capacityMeter{measurement: storagegovernor.Measurement{
		ID: "disk", TotalBytes: 600 * storagegovernor.GiB, FreeBytes: 400 * storagegovernor.GiB,
	}}
	governor := storagegovernor.New(meter, func(storagegovernor.Domain) storagegovernor.Policy { return storagegovernor.Policy{} })
	outage := errors.New("archive.org unreachable")
	downloader := perSourceDownloader{estimate: map[string]error{"https://archive.example/one": outage}}
	ingestor := clipfetch.New(nil, downloader, "/filler", discardLog()).WithStorageGovernor(governor)
	_, err := ingestor.Prepare(t.Context(), []clipfetch.Source{{Kind: clipfetch.Archive, URL: "https://archive.example/one"}}, storagegovernor.Automatic)
	var capacityErr *clipfetch.CapacityError
	if !errors.Is(err, outage) || errors.As(err, &capacityErr) {
		t.Fatalf("Prepare error = %v, want the provider failure and not a capacity pause", err)
	}
}
