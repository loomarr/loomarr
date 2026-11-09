package suggest

import (
	"bytes"
	"encoding/json"
	"slices"
	"testing"

	"github.com/loomarr/loomarr/internal/library/titlesearch"
	"github.com/loomarr/loomarr/internal/proposals"
	"github.com/loomarr/loomarr/internal/provision"
)

func TestRankGroundedCandidatesAppliesExplicitFeedbackDeterministically(t *testing.T) {
	candidates := []titlesearch.Candidate{
		{MediaType: provision.Movie, TMDBID: 90000001, Name: "Obvious Space Adventure", Genres: []string{"Science Fiction"}, Keywords: []string{"space"}, InLibrary: true},
		{MediaType: provision.Movie, TMDBID: 90000002, Name: "Quiet Space Discovery", Genres: []string{"Science Fiction"}, Keywords: []string{"space"}},
		{MediaType: provision.Movie, TMDBID: 90000003, Name: "Forbidden Space Film", Genres: []string{"Science Fiction"}, Keywords: []string{"space"}},
		{MediaType: provision.Movie, TMDBID: 90000004, Name: "Grounded Mystery", Genres: []string{"Mystery"}},
	}
	signals := []FeedbackSignal{
		{Target: "movie:tmdb:90000001", Action: FeedbackLess},
		{Target: "movie:tmdb:90000003", Action: FeedbackNever},
		{Target: "movie:tmdb:90000002", Action: FeedbackSurprise},
	}
	first := RankGroundedCandidates("space discoveries", candidates, signals)
	second := RankGroundedCandidates("space discoveries", candidates, signals)
	if !slices.EqualFunc(first, second, func(a, b titlesearch.Candidate) bool { return a.TMDBID == b.TMDBID }) {
		t.Fatalf("identical inputs produced different ranking: %+v / %+v", first, second)
	}
	var ids []int
	for _, candidate := range first {
		ids = append(ids, candidate.TMDBID)
	}
	if slices.Contains(ids, 90000003) {
		t.Fatalf("never target survived ranking: %v", ids)
	}
	if len(ids) < 2 || ids[0] != 90000002 || ids[1] != 90000001 {
		t.Fatalf("feedback ranking = %v, want surprise discovery before demoted related anchor", ids)
	}
	trace := RankGroundedCandidatesWithTrace("space discoveries", candidates, signals).Trace
	foundNever := false
	for _, decision := range trace.Candidates {
		if decision.Key == "movie:tmdb:90000003" {
			foundNever = decision.Disposition == proposals.DispositionNotSelected && decision.Reason == proposals.ReasonNever
		}
	}
	if !foundNever || trace.RecordedTotal != len(candidates) || trace.Terminal != "" {
		t.Fatalf("never exclusion was not preserved as a bounded decision: %+v", trace)
	}
}

func TestRankGroundedCandidatesSurpriseDiversifiesWithinRelevanceBand(t *testing.T) {
	candidates := []titlesearch.Candidate{
		{MediaType: provision.Movie, TMDBID: 90000100, Name: "Space Anchor", Genres: []string{"Science Fiction"}, Keywords: []string{"space"}, InLibrary: true},
		{MediaType: provision.Movie, TMDBID: 90000200, Name: "Space Familiar", Genres: []string{"Science Fiction"}, Keywords: []string{"space"}},
		{MediaType: provision.Movie, TMDBID: 90000300, Name: "Space Western", Genres: []string{"Western"}, Keywords: []string{"space"}},
		{MediaType: provision.Movie, TMDBID: 90000400, Name: "Unrelated Western", Genres: []string{"Western"}},
	}
	baseline := candidateIDs(RankGroundedCandidates("space", candidates, nil))
	if baseline[0] != 90000200 {
		t.Fatalf("baseline = %v, fixture must put the familiar canonical tie first", baseline)
	}

	signals := []FeedbackSignal{{Target: "movie:tmdb:90000100", Action: FeedbackSurprise}}
	ranked := RankGroundedCandidatesWithTrace("space", candidates, signals)
	got := candidateIDs(ranked.Candidates)
	if got[0] != 90000300 || got[1] != 90000200 || got[2] != 90000100 {
		t.Fatalf("surprise ranking = %v, want diverse neighbor before familiar and marked titles", got)
	}
	if got[3] != 90000400 {
		t.Fatalf("surprise ranking = %v, irrelevant diversity crossed the relevance floor", got)
	}
	if ranked.Trace.Candidates[0].Rank.Novelty != proposals.RankNoveltyMax {
		t.Fatalf("diverse candidate trace = %+v, want bounded novelty/diversity term", ranked.Trace.Candidates[0].Rank)
	}
	if err := proposals.ValidateDecisionTrace(ranked.Trace); err != nil {
		t.Fatalf("surprise trace rejected by shared validator: %v", err)
	}
}

func TestRankGroundedCandidatesLessDemotesGroundedGenreNeighbors(t *testing.T) {
	candidates := []titlesearch.Candidate{
		{MediaType: provision.Movie, TMDBID: 90000100, Name: "Night Anchor", Genres: []string{"Science Fiction"}, Keywords: []string{"night"}},
		{MediaType: provision.Movie, TMDBID: 90000200, Name: "Night Neighbor", Genres: []string{"Science Fiction"}, Keywords: []string{"night"}},
		{MediaType: provision.Movie, TMDBID: 90000300, Name: "Night Mystery", Genres: []string{"Mystery"}, Keywords: []string{"night"}},
	}
	got := candidateIDs(RankGroundedCandidates("night", candidates,
		[]FeedbackSignal{{Target: "movie:tmdb:90000100", Action: FeedbackLess}}))
	if !slices.Equal(got, []int{90000300, 90000200, 90000100}) {
		t.Fatalf("less ranking = %v, want unrelated, related demotion, exact demotion", got)
	}
}

func TestRankGroundedCandidatesProducesByteIdenticalResult(t *testing.T) {
	candidates := []titlesearch.Candidate{
		{MediaType: provision.Movie, TMDBID: 90000100, Name: "Space Anchor", Genres: []string{"Science Fiction"}, InLibrary: true},
		{MediaType: provision.Movie, TMDBID: 90000200, Name: "Space Western", Genres: []string{"Western"}},
		{MediaType: provision.Movie, TMDBID: 90000300, Name: "Space Mystery", Genres: []string{"Mystery"}},
	}
	signals := []FeedbackSignal{{Target: "movie:tmdb:90000100", Action: FeedbackSurprise}}
	want, err := json.Marshal(RankGroundedCandidatesWithTrace("space", candidates, signals))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		input := append([]titlesearch.Candidate(nil), candidates...)
		if i%2 == 1 {
			slices.Reverse(input)
		}
		got, err := json.Marshal(RankGroundedCandidatesWithTrace("space", input, signals))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("run %d was not byte-identical:\n%s\n%s", i, want, got)
		}
	}
}

func candidateIDs(candidates []titlesearch.Candidate) []int {
	ids := make([]int, 0, len(candidates))
	for _, candidate := range candidates {
		ids = append(ids, candidate.TMDBID)
	}
	return ids
}

func TestRankGroundedCandidatesWithTracePublishesExactLexicographicTupleAndBound(t *testing.T) {
	candidates := make([]titlesearch.Candidate, 0, proposals.DecisionTraceMaxCandidates+1)
	for i := 0; i < proposals.DecisionTraceMaxCandidates+1; i++ {
		candidates = append(candidates, titlesearch.Candidate{MediaType: provision.Movie, TMDBID: 90_000_001 + i, Name: "same", Genres: []string{"same"}})
	}
	got := RankGroundedCandidatesWithTrace("same", candidates, nil)
	if !got.Trace.Truncated || got.Trace.SurfacedTotal != proposals.DecisionTraceMaxCandidates+1 || len(got.Trace.Candidates) != proposals.DecisionTraceMaxCandidates {
		t.Fatalf("trace bounds = %+v", got.Trace)
	}
	if err := proposals.ValidateDecisionTrace(got.Trace); err != nil {
		t.Fatalf("ranker emitted trace rejected by shared validator: %v", err)
	}
	for i, item := range got.Trace.Candidates {
		if item.Rank.TieKey != item.Key || item.Rank.Relevance != 1 || item.Rank.Preference != 0 || item.Rank.Novelty != 1 {
			t.Fatalf("trace[%d] = %+v; tuple must be independently reconstructable", i, item)
		}
		if i > 0 && got.Trace.Candidates[i-1].Rank.TieKey >= item.Rank.TieKey {
			t.Fatalf("tie keys not stable: %+v", got.Trace.Candidates)
		}
	}
}

func TestRankGroundedCandidatesWithTraceClassifiesRetrievalEmpty(t *testing.T) {
	got := RankGroundedCandidatesWithTrace("nothing", nil, nil)
	if got.Trace.Version != proposals.DecisionTraceVersion || got.Trace.Terminal != proposals.ReasonRetrievalEmpty || got.Trace.SurfacedTotal != 0 || got.Trace.RecordedTotal != 0 {
		t.Fatalf("empty retrieval trace = %+v", got.Trace)
	}
}

func TestDecisionRankQueryRecordsMatchedConstraintCategoriesWithoutRawTerms(t *testing.T) {
	intent := Intent{Intent: proposals.Intent{
		Description: "science fiction", Tone: "energetic", Era: "1990s",
		MustInclude: []string{"marigold"}, MustExclude: []string{"comedy"}, RefineText: "more sequels"}}

	candidate := titlesearch.Candidate{
		MediaType: provision.Movie, TMDBID: 90000603, Name: "Marigold Protocol sequel",
		Overview: "An energetic 1990s science fiction story with more sequels",
	}
	got := rankGroundedCandidatesWithTrace(decisionRankQuery(intent), []titlesearch.Candidate{candidate}, nil).Trace.Candidates[0]
	want := proposals.ConstraintMatches{Request: true, Tone: true, Era: true, MustInclude: true, Refine: true}
	if got.Constraints != want || got.Rank.Relevance != 7 {
		t.Fatalf("constraint evidence = %+v relevance=%d, want %+v relevance=7", got.Constraints, got.Rank.Relevance, want)
	}
}

func TestDecisionRankQueryDoesNotTreatRequestScaffoldingAsEvidence(t *testing.T) {
	intent := Intent{Intent: proposals.Intent{Description: "Please build a channel like this reference"}}
	candidate := titlesearch.Candidate{
		MediaType: provision.Movie, TMDBID: 90000604, Name: "Unrelated Mechanics",
		Overview: "They build this machine for a difficult job.",
	}

	got := rankGroundedCandidatesWithTrace(decisionRankQuery(intent), []titlesearch.Candidate{candidate}, nil).Trace.Candidates[0]
	if got.Rank.Relevance != 0 || got.Constraints.Request {
		t.Fatalf("request scaffolding became relevance evidence: terms=%v decision=%+v", decisionRankQuery(intent).request, got)
	}
}

func TestMembershipEvidenceGateTargetsNamedSetsWithoutHardcodingOneBlock(t *testing.T) {
	for _, description := range []string{
		"QFUN",
		"QFUN with Crumb Street Bakery and Bowling Alley Blues.",
		"Bellwether",
		"The Bellwether Channel",
		"Bellwether Channel",
		"Lantern",
		"Lantern Parade",
		"a channel like QFUN from the 90s",
		"a named Friday-night family sitcom collection",
		"Bellwether Collection lineup",
		"Lantern Collection",
		"Classic Mudpuddles",
		"Bellwether Channel classics",
		"QFUN like the 90s, with shows from the ZBN network",
		"make a channel for FFS like the 90s",
		"shows from NQN",
		"the XYZ programming block",
		"lineup from Friday Family Showcase",
	} {
		if !requiresMembershipEvidence(Intent{Intent: proposals.Intent{Description: description}}) {
			t.Fatalf("named-set request was not gated: %q", description)
		}
	}
	for _, description := range []string{
		"UFO movies",
		"shows from the ZBN network",
		"shows from network ZBN",
		"network ZBN shows",
		"ZBN network shows",
		"ZBN network shows, not QFUN",
		"movies starring Rowan Pike",
		"feel-good sci-fi",
		"1990s family comedies",
		"Comedy",
		"Science Fiction",
		"Classic Comedy",
		"Classic Science Fiction",
		"Comedy Classics",
	} {
		if requiresMembershipEvidence(Intent{Intent: proposals.Intent{Description: description}}) {
			t.Fatalf("ordinary theme request was over-gated: %q", description)
		}
	}
	for _, intent := range []Intent{
		{Description: "Please suggest comedy"},
		{Description: "Let us make a family comedy channel"},
		{Description: "90s family comedies", MustInclude: []string{"The Understudies"}},
		{Description: "family comedies, but not The Understudies"},
	} {
		if requiresMembershipEvidence(intent) {
			t.Fatalf("ordinary request was over-gated: %+v", intent)
		}
	}
	for _, intent := range []Intent{
		{Description: "Bellwether Channel", RefineText: "best picks"},
		{Description: "Bellwether Channel", MustInclude: []string{"Classic Mudpuddles"}},
		{Description: "Bellwether Collection lineup", RefineText: "best picks"},
		{Description: "Bellwether Channel", RefineText: "Classic Comedy"},
	} {
		if !requiresMembershipEvidence(intent) {
			t.Fatalf("cross-field cue bypassed named-set gate: %+v", intent)
		}
	}
}

func TestMergeDecisionTracePreservesPreTruncationTotalsAndLatestDuplicateFacts(t *testing.T) {
	dst := proposals.DecisionTrace{Version: proposals.DecisionTraceVersion, SurfacedTotal: 1, RecordedTotal: 1,
		Candidates: []proposals.DecisionCandidate{{Key: "movie:tmdb:90000603", Source: string(titlesearch.ScopeAdjacent)}}}
	src := proposals.DecisionTrace{Version: proposals.DecisionTraceVersion, SurfacedTotal: 70, RecordedTotal: 70, Truncated: true,
		Candidates: []proposals.DecisionCandidate{
			{Key: "movie:tmdb:90000603", Source: string(titlesearch.ScopeAll)},
			{Key: "movie:tmdb:90000604", Source: string(titlesearch.ScopeAll)},
		}}
	mergeDecisionTrace(&dst, &src)
	if dst.SurfacedTotal != 71 || dst.RecordedTotal != 71 || !dst.Truncated || len(dst.Candidates) != 2 {
		t.Fatalf("merged bounds = %+v", dst)
	}
	if dst.Candidates[0].Source != string(titlesearch.ScopeAll) {
		t.Fatalf("latest decision facts did not replace adjacency facts: %+v", dst.Candidates[0])
	}

	full := proposals.DecisionTrace{Version: proposals.DecisionTraceVersion, RecordedTotal: proposals.DecisionTraceMaxCandidates,
		Candidates: make([]proposals.DecisionCandidate, proposals.DecisionTraceMaxCandidates)}
	traceDecision(&full, proposals.DecisionCandidate{Disposition: proposals.DispositionValidationDropped, Reason: proposals.ReasonMalformedID})
	if full.RecordedTotal != proposals.DecisionTraceMaxCandidates+1 || !full.Truncated {
		t.Fatalf("truncated decision total = %+v", full)
	}
}

func TestRelevanceNeedsMetadataEvidenceBeyondTitleWords(t *testing.T) {
	query := rankQuery{request: intentWordSet("Halloween channel with ghosts, witches and haunted houses")}
	ghostProtocol := titlesearch.Candidate{MediaType: provision.Movie, TMDBID: 90000501, Name: "Mission: Impossible - Ghost Protocol", Genres: []string{"Action", "Thriller"}}
	ghostWorld := titlesearch.Candidate{MediaType: provision.Movie, TMDBID: 90000502, Name: "Ghost World", Genres: []string{"Comedy", "Drama"}}
	hocusPocus := titlesearch.Candidate{MediaType: provision.Movie, TMDBID: 90000503, Name: "Hocus Pocus", Genres: []string{"Family", "Fantasy"}, Keywords: []string{"witch", "halloween"}}
	for _, title := range []titlesearch.Candidate{ghostProtocol, ghostWorld} {
		if got, _ := relevanceForCandidate(query, title); got != 0 {
			t.Errorf("%s relevance = %d, want 0: a title word alone is not evidence", title.Name, got)
		}
	}
	if got, _ := relevanceForCandidate(query, hocusPocus); got == 0 {
		t.Fatalf("Hocus Pocus relevance = 0, want keyword evidence to count")
	}
	ranked := RankGroundedCandidates("Halloween channel with ghosts, witches and haunted houses", []titlesearch.Candidate{ghostProtocol, ghostWorld, hocusPocus}, nil)
	if len(ranked) == 0 || ranked[0].TMDBID != hocusPocus.TMDBID {
		t.Fatalf("ranked first = %+v, want Hocus Pocus above title-word matches", ranked)
	}
}

func TestRelevanceCountsTitleWordsOnceMetadataAgrees(t *testing.T) {
	query := rankQuery{request: intentWordSet("space adventure")}
	both := titlesearch.Candidate{MediaType: provision.Movie, TMDBID: 90000504, Name: "Space Anchor", Keywords: []string{"adventure"}}
	metaOnly := titlesearch.Candidate{MediaType: provision.Movie, TMDBID: 90000505, Name: "Anchor", Keywords: []string{"adventure"}}
	a, _ := relevanceForCandidate(query, both)
	b, _ := relevanceForCandidate(query, metaOnly)
	if a <= b || b == 0 {
		t.Fatalf("relevance with agreeing title = %d, metadata only = %d; want title to add once metadata agrees", a, b)
	}
}
