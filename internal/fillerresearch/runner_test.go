package fillerresearch

import (
	"context"
	"testing"
	"time"
)

type runnerRepository struct {
	adapter string
	version string
	reports []Report
}

func (r *runnerRepository) ListCandidates(_ context.Context, _, _, adapter, version string, _ int) ([]Candidate, error) {
	r.adapter, r.version = adapter, version
	return []Candidate{{ClipHash: "hash", Name: "Tootsie Pop Classic Commercial", SourceID: "archive:classic", InputRevision: 1}}, nil
}

func (r *runnerRepository) SaveReport(_ context.Context, report Report) error {
	r.reports = append(r.reports, report)
	return nil
}

func TestRunnerSelectsCandidatesWithTheConfiguredRetrieverIdentity(t *testing.T) {
	at := time.Unix(200, 0).UTC()
	wiki := scriptedRetriever{adapter: "mediawiki", version: "v2", packet: researchPacket()}
	archive := scriptedRetriever{adapter: "archive-search", version: "v1", err: context.DeadlineExceeded}
	retriever, err := NewFederated(wiki, archive)
	if err != nil {
		t.Fatal(err)
	}
	repository := &runnerRepository{}
	provider := &fixtureProvider{content: `{"decade":1970,"countryCode":"US","country":"United States","confidence":70,"explanation":"Likely campaign context.","citationIds":[1]}`}
	runner := NewRunner(repository, New(retriever, provider, "fixture", "model", func() time.Time { return at }),
		func(_ context.Context, candidate Candidate) (Input, error) {
			return Input{Title: candidate.Name, SourceKind: "archive"}, nil
		}, func() int { return 1 })
	result, err := runner.Run(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if repository.adapter != "federated" || repository.version != "federated-v1:mediawiki:v2+archive-search:v1" {
		t.Fatalf("candidate identity = %s/%s", repository.adapter, repository.version)
	}
	if result.Considered != 1 || result.Updated != 1 || len(repository.reports) != 1 {
		t.Fatalf("result=%+v reports=%+v", result, repository.reports)
	}
}
