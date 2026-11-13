package playoutresolve

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/library/inventory"
	"github.com/loomarr/loomarr/internal/media/measure"
	"github.com/loomarr/loomarr/internal/platform/store/librarystore"
	"github.com/loomarr/loomarr/internal/testkit"
)

type submittedRefs struct{ refs []measure.SourceRef }

func (s *submittedRefs) MeasureFacts(context.Context, measure.SourceRef) (inventory.SourceFacts, error) {
	return inventory.SourceFacts{}, nil
}
func (s *submittedRefs) Submit(ref measure.SourceRef) { s.refs = append(s.refs, ref) }

// Measure ahead (#1518): a scheduled library item's file goes to the measure worker until its
// current revision is measured, then never again; an item with no readable file asks for nothing.
func TestPlayoutResolver_PremeasureQueuesUnmeasuredLibraryFilesOnce(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	path := filepath.Join(t.TempDir(), "short.mkv")
	if err := os.WriteFile(path, []byte("matroska"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	if err := st.SaveLibraryItems(t.Context(), []librarystore.LibraryItem{{ID: "m1", Kind: "movie", Name: "Short",
		Sources: []librarystore.LibrarySource{{Path: path, Plays: true}}}}, at); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveLibraryFiles(t.Context(), []librarystore.LibraryFile{{Path: path, Root: filepath.Dir(path),
		Size: info.Size(), ModifiedNs: info.ModTime().UnixNano(), Identity: "digest", Kind: "movie",
		Header: librarystore.LibraryHeaderRead, SeenAt: at, ItemID: "m1"}}); err != nil {
		t.Fatal(err)
	}
	worker := &submittedRefs{}
	r := &Resolver{inventory: inventory.New(st), analyses: st, libraryFile: st, now: time.Now, measurer: worker}

	r.Premeasure(t.Context(), []string{"m1", "no-such-item"})
	if len(worker.refs) != 1 || worker.refs[0].Path != path {
		t.Fatalf("submitted %+v; want the scheduled item's file once", worker.refs)
	}
	ref := worker.refs[0]
	if err := st.RecordInventoryAnalysis(t.Context(), inventory.Analysis{SourceID: ref.ID, Revision: ref.Revision, AnalyzedAt: at}); err != nil {
		t.Fatal(err)
	}
	r.Premeasure(t.Context(), []string{"m1"})
	if len(worker.refs) != 1 {
		t.Fatalf("submitted %+v; want a measured revision skipped", worker.refs)
	}
}

// A movie in parts is measured a part at a time: each part's file is queued, and its fades are
// break candidates at where the part airs in the programme, once every part is measured. Where
// the parts meet is no candidate of itself.
func TestAMovieInPartsBreaksAtEachPartsFades(t *testing.T) {
	st := testkit.MigratedSQLiteStore(t)
	dir := t.TempDir()
	at := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	item := librarystore.LibraryItem{ID: "m1", Kind: "movie", Name: "Long"}
	var files []librarystore.LibraryFile
	for i, name := range []string{"long cd1.mkv", "long cd2.mkv"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("matroska "+name), 0o644); err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		item.Sources = append(item.Sources, librarystore.LibrarySource{Path: path, Part: i + 1, Plays: true, DurationMs: 3_000_000})
		files = append(files, librarystore.LibraryFile{Path: path, Root: dir, Size: info.Size(),
			ModifiedNs: info.ModTime().UnixNano(), Identity: "digest", Kind: "movie",
			Header: librarystore.LibraryHeaderRead, SeenAt: at, ItemID: "m1"})
	}
	if err := st.SaveLibraryItems(t.Context(), []librarystore.LibraryItem{item}, at); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveLibraryFiles(t.Context(), files); err != nil {
		t.Fatal(err)
	}
	worker := &submittedRefs{}
	r := &Resolver{inventory: inventory.New(st), analyses: st, libraryFile: st, now: time.Now, measurer: worker}

	r.Premeasure(t.Context(), []string{"m1"})
	if len(worker.refs) != 2 || worker.refs[0].Path != item.Sources[0].Path || worker.refs[1].Path != item.Sources[1].Path {
		t.Fatalf("submitted %+v; want each part's file", worker.refs)
	}
	breaks := r.NaturalBreakSource(t.Context())
	record := func(ref measure.SourceRef, atMs int64) {
		t.Helper()
		if err := st.RecordInventoryAnalysis(t.Context(), inventory.Analysis{SourceID: ref.ID, Revision: ref.Revision, AnalyzedAt: at,
			Breaks: []inventory.Break{{AtMs: atMs, Source: "fade", OverlapMs: 400, Confidence: 0.9}}}); err != nil {
			t.Fatal(err)
		}
	}
	record(worker.refs[0], 1_200_000)
	if got := breaks.NaturalBreaks("m1"); got != nil {
		t.Fatalf("with part 2 unmeasured: %+v; want it to air whole", got)
	}
	record(worker.refs[1], 900_000)
	got := breaks.NaturalBreaks("m1")
	if len(got) != 2 || got[0].AtMs != 1_200_000 || got[1].AtMs != 3_900_000 {
		t.Fatalf("breaks = %+v; want part 1's at 1200000 and part 2's at 3000000+900000", got)
	}
}
