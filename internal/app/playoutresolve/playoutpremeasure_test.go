package playoutresolve

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/library/inventory"
	"github.com/loomarr/loomarr/internal/media/measure"
	"github.com/loomarr/loomarr/internal/platform/store"
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
	if err := st.SaveLibraryItems(t.Context(), []store.LibraryItem{{ID: "m1", Kind: "movie", Name: "Short",
		Sources: []store.LibrarySource{{Path: path, Plays: true}}}}, at); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveLibraryFiles(t.Context(), []store.LibraryFile{{Path: path, Root: filepath.Dir(path),
		Size: info.Size(), ModifiedNs: info.ModTime().UnixNano(), Identity: "digest", Kind: "movie",
		Header: store.LibraryHeaderRead, SeenAt: at, ItemID: "m1"}}); err != nil {
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
