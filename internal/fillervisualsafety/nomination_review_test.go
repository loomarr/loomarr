package fillervisualsafety

import (
	"context"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

func TestVisualCorpusNominationReviewBoardUsesBoundSourceAndExportsLockCSV(t *testing.T) {
	t.Parallel()
	fixture := newVisualNominationFixture(t)
	worksheet, err := PrepareVisualCorpusNominationWorksheet(context.Background(), fixture.prepare)
	if err != nil {
		t.Fatal(err)
	}
	board, err := RenderVisualCorpusNominationReviewBoard(worksheet, fixture.prepare.MediaRoot)
	if err != nil {
		t.Fatal(err)
	}
	assetURL := (&url.URL{Scheme: "file", Path: filepath.Join(fixture.prepare.MediaRoot, worksheet.Cases[0].LocalFile)}).String()
	for _, required := range []string{
		worksheet.SHA256, worksheet.Cases[0].CaseID, worksheet.Cases[0].Asset.SHA256, assetURL,
		`const header = ["rank","inventory_sha256"`, `link.download = "review.csv"`,
		`proposed.sourceSha256 !== item.contentSha256`, `document.inventorySha256 !== first[1]`,
		`manifest proposal fields are invalid`, `Positive and clean always require an individual click or key.`,
	} {
		if !strings.Contains(string(board), required) {
			t.Fatalf("review board does not contain %q", required)
		}
	}
}

func TestVisualCorpusNominationReviewBoardRejectsUnboundInputs(t *testing.T) {
	t.Parallel()
	fixture := newVisualNominationFixture(t)
	worksheet, err := PrepareVisualCorpusNominationWorksheet(context.Background(), fixture.prepare)
	if err != nil {
		t.Fatal(err)
	}
	worksheet.SHA256 = strings.Repeat("0", 64)
	if _, err := RenderVisualCorpusNominationReviewBoard(worksheet, fixture.prepare.MediaRoot); err == nil {
		t.Fatal("RenderVisualCorpusNominationReviewBoard accepted a changed worksheet")
	}
	worksheet.SHA256 = VisualCorpusNominationWorksheetSHA256(worksheet)
	if _, err := RenderVisualCorpusNominationReviewBoard(worksheet, "relative"); err == nil {
		t.Fatal("RenderVisualCorpusNominationReviewBoard accepted a relative media root")
	}
}
