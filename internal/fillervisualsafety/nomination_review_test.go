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
		`const cleanReviewMode = false;`, `excludeNonProposals.hidden = cleanReviewMode`,
	} {
		if !strings.Contains(string(board), required) {
			t.Fatalf("review board does not contain %q", required)
		}
	}
}

func TestVisualCorpusNominationReviewBoardEnablesBoundedCleanReviewOnlyForCanonicalRole(t *testing.T) {
	t.Parallel()
	fixture := newVisualNominationFixture(t)
	worksheet, err := PrepareVisualCorpusNominationWorksheet(context.Background(), fixture.prepare)
	if err != nil {
		t.Fatal(err)
	}
	worksheet.Cases[0].RoleHints = []string{VisualCorpusCleanNominationRoleHint}
	worksheet.Cases[0].SubjectTerms = nil
	worksheet.SHA256 = VisualCorpusNominationWorksheetSHA256(worksheet)
	board, err := RenderVisualCorpusNominationReviewBoard(worksheet, fixture.prepare.MediaRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		`const cleanReviewMode = true;`, `const cleanPageSize = 12;`,
		`I reviewed every exact image on this page for adult nudity, minors, sexual or graphic content, hate symbols or text`,
		`pageItems.every(item => cleanPageLoads.get(item.caseId) === true)`,
		`!decisions[item.caseId] && !needsIndividualReview(item)`,
		`decisions[item.caseId] === "clean" && needsIndividualReview(item)`,
		`Positive decisions require an individual click or key.`,
		`(item.subjectTerms || []).join(" · ")`,
		`Open exact image at full resolution`,
		`inspect.textContent = "Inspect required"`,
		`!assistanceLoaded || !allLoaded`, `Load a bound machine-assistance manifest before confirming a page.`,
		`filler-visual-corpus-clean-assistance-v1`, `two_local_vlm_plus_local_ocr_text_v1`,
		`document.leftModel === document.rightModel`,
		`OCR text detected`,
	} {
		if !strings.Contains(string(board), required) {
			t.Fatalf("clean review board does not contain %q", required)
		}
	}

	worksheet.Cases[0].RoleHints = []string{VisualCorpusCleanNominationRoleHint, "another-role"}
	worksheet.SHA256 = VisualCorpusNominationWorksheetSHA256(worksheet)
	if visualCorpusCleanReviewMode(worksheet) {
		t.Fatal("clean review mode accepted a worksheet with an additional role")
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
