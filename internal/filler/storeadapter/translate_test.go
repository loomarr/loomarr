package storeadapter_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler/acquisition"
	"github.com/loomarr/loomarr/internal/filler/defectfloor"
	"github.com/loomarr/loomarr/internal/filler/enrichment"
	"github.com/loomarr/loomarr/internal/filler/identity"
	"github.com/loomarr/loomarr/internal/filler/pipelinestate"
	"github.com/loomarr/loomarr/internal/filler/records"
	"github.com/loomarr/loomarr/internal/filler/research"
	"github.com/loomarr/loomarr/internal/filler/storeadapter"
	"github.com/loomarr/loomarr/internal/media/defects"
	"github.com/loomarr/loomarr/internal/platform/store"
	"github.com/loomarr/loomarr/internal/platform/store/clipstore"
)

type contextT = context.Context

// rowStore serves canned rows and records writes. A method it does not override panics, so an
// adapter that reaches for another one fails its test.
type rowStore struct {
	records.Combined
	err   error
	calls []string
	refs  []string
}

func (s *rowStore) call(name string) { s.calls = append(s.calls, name) }

func (s *rowStore) RetireDefectiveCopy(contextT, string, time.Time) error { return s.err }
func (s *rowStore) SetClipDefectLabel(contextT, string, defects.Label, time.Time) error {
	return s.err
}
func (s *rowStore) GetClip(contextT, string) (clipstore.Clip, error) { return clipstore.Clip{}, s.err }

// ListClips serves a held clip "b" only to a filter that asks for held clips.
func (s *rowStore) ListClips(_ contextT, f clipstore.ClipFilter) ([]clipstore.Clip, error) {
	var a, b clipstore.Clip
	a.Hash, a.Path, a.Name, a.Transcript, a.VisibleText = "a", "a.mp4", "A", "said", "shown"
	b.Hash = "b"
	if !f.IncludeHeld {
		return []clipstore.Clip{a}, nil
	}
	return []clipstore.Clip{a, b}, nil
}
func (s *rowStore) ListClipPipelines(_ contextT, f pipelinestate.PipelineFilter) ([]pipelinestate.ClipPipeline, error) {
	if !slices.Equal(f.Dispositions, []pipelinestate.Disposition{pipelinestate.DispositionRunning}) {
		return nil, nil
	}
	return []pipelinestate.ClipPipeline{{ClipHash: "a", Stage: pipelinestate.StageDefects}}, nil
}
func (s *rowStore) ListClipVariants(contextT) (map[string][]string, error) {
	return map[string][]string{"a": {"vhs"}}, nil
}
func (s *rowStore) ListClipDefects(contextT) (map[string]clipstore.ClipDefectRecord, error) {
	return map[string]clipstore.ClipDefectRecord{"a": {ClipHash: "a"}}, s.err
}
func (s *rowStore) GetClipDefects(contextT, string) (clipstore.ClipDefectRecord, bool, error) {
	return clipstore.ClipDefectRecord{ClipHash: "a"}, true, nil
}
func (s *rowStore) UpsertClipDefects(_ contextT, r clipstore.ClipDefectRecord) error {
	s.call("defects " + r.ClipHash)
	return nil
}
func (s *rowStore) ListClipDefectLabels(contextT) (map[string]clipstore.ClipDefectLabel, error) {
	return map[string]clipstore.ClipDefectLabel{"a": {Era: 1990}}, nil
}
func (s *rowStore) ListFillerSources(contextT) ([]records.FillerSource, error) {
	return []records.FillerSource{{ID: "src", Kind: "archive", URI: "item"}}, nil
}
func (s *rowStore) MarkFillerSourceFetched(_ contextT, id string, _ time.Time) error {
	s.call("fetched " + id)
	return nil
}
func (s *rowStore) ClaimFillerSourceCheck(_ contextT, id string, _, _, _ time.Time) (bool, error) {
	s.call("claim " + id)
	return true, nil
}
func (s *rowStore) CompleteFillerSourceCheck(_ contextT, id string, _ time.Time, _ acquisition.SourceCheckCompletion) error {
	s.call("complete " + id)
	return nil
}
func (s *rowStore) FailFillerSourceCheck(_ contextT, id string, _, _ time.Time) error {
	s.call("fail " + id)
	return nil
}
func (s *rowStore) ListFillerEnrichmentCandidates(contextT, string, string, string, int) ([]clipstore.Clip, error) {
	return s.ListClips(context.Background(), clipstore.ClipFilter{IncludeHeld: true})
}
func (s *rowStore) ListFillerEnrichmentCapabilityCandidates(contextT, string, string, string, int) ([]clipstore.Clip, error) {
	return nil, s.err
}
func (s *rowStore) ListFillerEnrichment(_ contextT, hash string) ([]enrichment.State, error) {
	s.call("states " + hash)
	return nil, nil
}
func (s *rowStore) ApplyFillerEnrichmentPass(contextT, enrichment.Pass) (int, error) { return 3, nil }
func (s *rowStore) ListFillerResearchCandidates(contextT, string, string, string, string, int) ([]research.Candidate, error) {
	return []research.Candidate{{}}, nil
}
func (s *rowStore) SaveFillerResearchReport(contextT, research.Report) error {
	s.call("report")
	return nil
}
func (s *rowStore) PromoteStoredFillerResearchCountries(_ contextT, limit int) (int, error) {
	return limit, nil
}
func (s *rowStore) ListWantedCopies(contextT) ([]clipstore.WantedCopy, error) {
	return []clipstore.WantedCopy{{ClipHash: "a", Path: "a.mp4"}}, nil
}
func (s *rowStore) MergeInferiorCopy(_ contextT, c clipstore.InferiorCopy) error {
	s.call("merge " + c.ClipHash + " into " + c.SurvivorHash)
	return nil
}
func (s *rowStore) ListSweepableSplitProposals(contextT, time.Time) ([]records.SweepableProposal, error) {
	return []records.SweepableProposal{{ProposalID: "p", ClipHash: "a", ClipPath: "a.mp4", Segments: 2}}, nil
}
func (s *rowStore) DeleteSplitProposal(contextT, string) error { return s.err }
func (s *rowStore) DeleteImageRefs(_ contextT, owner, id string) error {
	s.refs = append(s.refs, owner+"/"+id)
	return nil
}

func TestDefectsTranslatesStoreErrors(t *testing.T) {
	ctx := t.Context()
	gone := storeadapter.NewDefects(&rowStore{err: store.ErrNotFound})
	if err := gone.RetireDefectiveCopy(ctx, "a", time.Now()); !errors.Is(err, defectfloor.ErrCopyGone) {
		t.Errorf("retire = %v", err)
	}
	if err := gone.SetClipDefectLabel(ctx, "a", defects.Label(""), time.Now()); !errors.Is(err, defectfloor.ErrDefectNotMeasured) {
		t.Errorf("label = %v", err)
	}
	// A corrupt row is reported, and the readable rows still come back.
	rows, err := storeadapter.NewDefects(&rowStore{err: clipstore.ErrCorruptClipDefects}).ListClipDefects(ctx)
	if !errors.Is(err, defectfloor.ErrDefectRecordsCorrupt) || rows["a"].ClipHash != "a" {
		t.Errorf("corrupt list = %v, %v", rows, err)
	}
}

func TestDefectsConvertsRows(t *testing.T) {
	ctx, st := t.Context(), &rowStore{}
	a := storeadapter.NewDefects(st)
	listed, err := a.ListDefectClips(ctx)
	if err != nil || len(listed) != 2 || listed[0].Stage != pipelinestate.StageDefects || listed[1].Stage != "" {
		t.Errorf("defect clips = %+v, %v", listed, err)
	}
	if rows, err := a.ListClipDefects(ctx); err != nil || rows["a"].ClipHash != "a" {
		t.Errorf("list = %v, %v", rows, err)
	}
	if row, found, err := a.GetClipDefects(ctx, "a"); err != nil || !found || row.ClipHash != "a" {
		t.Errorf("get = %+v, %v, %v", row, found, err)
	}
	if err := a.UpsertClipDefects(ctx, defectfloor.DefectRecord{ClipHash: "z"}); err != nil || !slices.Contains(st.calls, "defects z") {
		t.Errorf("upsert: %v, calls %v", err, st.calls)
	}
	if labels, err := a.ListClipDefectLabels(ctx); err != nil || labels["a"].Era != 1990 {
		t.Errorf("labels = %v, %v", labels, err)
	}
}

func TestFetchSpeaksTheStoresSourceChecks(t *testing.T) {
	ctx, st, now := t.Context(), &rowStore{}, time.Now()
	a := storeadapter.NewFetch(st, func() time.Duration { return time.Hour }, nil)
	sources, err := a.ListFetchSources(ctx)
	if err != nil || len(sources) != 1 || sources[0].ID != "src" || sources[0].URI != "item" {
		t.Errorf("sources = %+v, %v", sources, err)
	}
	if claimed, err := a.ClaimCheck(ctx, "src", now, now, now); !claimed || err != nil {
		t.Errorf("claim = %v, %v", claimed, err)
	}
	if err := errors.Join(a.MarkFetched(ctx, "src", now), a.CompleteCheck(ctx, "src", now, acquisition.SourceCheckCompletion{}),
		a.FailCheck(ctx, "src", now, now)); err != nil {
		t.Error(err)
	}
	if want := []string{"claim src", "fetched src", "complete src", "fail src"}; !slices.Equal(st.calls, want) {
		t.Errorf("calls = %v, want %v", st.calls, want)
	}
}

func TestEnrichmentAndResearchRepositories(t *testing.T) {
	ctx, st := t.Context(), &rowStore{}
	e := storeadapter.NewEnrichment(st)
	got, err := e.ListCandidates(ctx, "p", "1", "1", 2)
	if err != nil || len(got) != 2 || got[0].ClipHash != "a" || got[0].Path != "a.mp4" || got[0].Name != "A" ||
		got[0].Transcript != "said" || got[0].VisibleText != "shown" {
		t.Errorf("candidates = %+v, %v", got, err)
	}
	failing := storeadapter.NewEnrichment(&rowStore{err: store.ErrNotFound})
	if _, err := failing.ListCapabilityCandidates(ctx, "p", "1", "1", 2); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("capability candidates error = %v", err)
	}
	if _, err := e.ListStates(ctx, "a"); err != nil {
		t.Error(err)
	}
	if n, err := e.ApplyPass(ctx, enrichment.Pass{}); n != 3 || err != nil {
		t.Errorf("apply = %d, %v", n, err)
	}
	r := storeadapter.NewResearch(st)
	if c, err := r.ListCandidates(ctx, "p", "1", "a", "1", 1); len(c) != 1 || err != nil {
		t.Errorf("research candidates = %v, %v", c, err)
	}
	if err := r.SaveReport(ctx, research.Report{}); err != nil {
		t.Error(err)
	}
	if n, err := r.PromoteStoredCountries(ctx, 4); n != 4 || err != nil {
		t.Errorf("promote = %d, %v", n, err)
	}
	if want := []string{"states a", "report"}; !slices.Equal(st.calls, want) {
		t.Errorf("calls = %v, want %v", st.calls, want)
	}
}

func TestIdentityTranslatesCopies(t *testing.T) {
	ctx, st := t.Context(), &rowStore{}
	a := storeadapter.NewIdentity(st)
	if w, err := a.ListWantedCopies(ctx); err != nil || !slices.Equal(w, []identity.WantedCopy{{ClipHash: "a", Path: "a.mp4"}}) {
		t.Errorf("wanted = %v, %v", w, err)
	}
	if err := a.MergeInferiorCopy(ctx, identity.InferiorCopy{ClipHash: "a", SurvivorHash: "b"}); err != nil ||
		!slices.Equal(st.calls, []string{"merge a into b"}) {
		t.Errorf("merge: %v, calls %v", err, st.calls)
	}
	clips, err := a.ListIdentityClips(ctx)
	if err != nil || len(clips) != 2 || clips[0].Defects == nil || clips[1].Defects != nil {
		t.Errorf("identity clips = %+v, %v", clips, err)
	}
}

func TestStoreFailuresAndExtrasAreCarried(t *testing.T) {
	ctx, broken := t.Context(), errors.New("broken")
	if _, found, err := storeadapter.NewSync(&rowStore{err: broken}).GetClip(ctx, "a"); found || !errors.Is(err, broken) {
		t.Errorf("get on a failing store: found=%v err=%v", found, err)
	}
	// A corrupt defect row does not stop the other copies ranking.
	corrupt := storeadapter.NewIdentity(&rowStore{err: clipstore.ErrCorruptClipDefects})
	if rows, err := corrupt.ListIdentityClips(ctx); err != nil || len(rows) != 2 {
		t.Errorf("identity with a corrupt row = %d, %v", len(rows), err)
	}
	pods, err := storeadapter.NewCatalog(&rowStore{}).AllClips(ctx)
	if err != nil || len(pods) != 1 || !slices.Equal(pods[0].Variants, []string{"vhs"}) {
		t.Errorf("pod clips = %+v, %v", pods, err)
	}
}

func TestDeletingASplitProposalDropsItsArtwork(t *testing.T) {
	ctx := t.Context()
	rows, err := storeadapter.NewSweep(&rowStore{}).ListSweepableSplitProposals(ctx, time.Now())
	if err != nil || len(rows) != 1 || rows[0].ProposalID != "p" || rows[0].Segments != 2 {
		t.Errorf("sweepable = %+v, %v", rows, err)
	}
	for name, remove := range map[string]func(records.Combined) error{
		"rewind": func(st records.Combined) error { return storeadapter.NewRewind(st).DeleteSplitProposal(ctx, "p") },
		"sweep":  func(st records.Combined) error { return storeadapter.NewSweep(st).DeleteSplitProposal(ctx, "p") },
	} {
		st := &rowStore{}
		if err := remove(st); err != nil || !slices.Equal(st.refs, []string{storeadapter.SplitProposalArtworkOwner + "/p"}) {
			t.Errorf("%s: %v, refs %v", name, err, st.refs)
		}
		failed := &rowStore{err: store.ErrNotFound}
		if err := remove(failed); err == nil || len(failed.refs) != 0 {
			t.Errorf("%s after a failed delete: %v, refs %v", name, err, failed.refs)
		}
	}
}
