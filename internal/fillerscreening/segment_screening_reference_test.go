package fillerscreening

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
)

// These tests pin filler.SegmentScreeningReference, the sidecar's join to a recorded aggregate. They
// need an aggregate built through this package's axis-evidence fixtures.

func TestSegmentScreeningReferenceRoundTripsBesideRenderedChild(t *testing.T) {
	subject := screeningChildSubjectFixture(t)
	aggregate := screeningAggregateFixture(t, subject)
	reference, err := filler.NewSegmentScreeningReference(subject, aggregate)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	media := filepath.Join(dir, "child.mp4")
	if err := os.WriteFile(media, []byte("rendered child"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := filler.WriteSidecarTags(media, filler.SidecarTags{SegmentScreening: &reference}, false); err != nil {
		t.Fatal(err)
	}
	tags, state := filler.ReadSidecarTagsState(media)
	if state != filler.SidecarValid || tags.SegmentScreening == nil || *tags.SegmentScreening != reference {
		t.Fatalf("screening reference did not round-trip: state=%v tags=%+v", state, tags)
	}
}

func TestSegmentScreeningReferenceRejectsSubjectDrift(t *testing.T) {
	subject := screeningChildSubjectFixture(t)
	aggregate := screeningAggregateFixture(t, subject)
	other := subject
	other.CatalogHash = strings.Repeat("f", 64)
	other.SHA256 = filler.SegmentScreeningSubjectSHA256(other)
	if _, err := filler.NewSegmentScreeningReference(other, aggregate); err == nil {
		t.Fatal("reference joined an aggregate to a different subject")
	}
}

func screeningAggregateFixture(t *testing.T, subject filler.SegmentScreeningSubject) filler.SegmentScreeningEvidence {
	t.Helper()
	records := passingAxisEvidence(t, subject)
	results := make([]filler.SegmentScreeningResult, 0, len(records))
	for _, recorded := range records {
		results = append(results, recorded.Evidence.Result())
	}
	aggregate, err := filler.NewSegmentScreeningEvidence(subject, results, screeningAirworthinessDecision(t, subject, records), time.Date(2026, time.September, 12, 4, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	return aggregate
}
