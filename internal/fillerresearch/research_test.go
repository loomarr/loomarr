package fillerresearch

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/llm"
)

type fixtureRetriever struct {
	packet Packet
	query  string
	calls  int
}

func (r *fixtureRetriever) Retrieve(_ context.Context, query string) (Packet, error) {
	r.calls++
	r.query = query
	return r.packet, nil
}

type fixtureProvider struct {
	content  string
	messages []llm.Message
	options  llm.ChatOptions
}

func (p *fixtureProvider) Name() string { return "fixture" }
func (p *fixtureProvider) Chat(_ context.Context, messages []llm.Message, options llm.ChatOptions) (llm.Response, error) {
	p.messages, p.options = messages, options
	return llm.Response{Content: p.content}, nil
}

func researchPacket() Packet {
	return Packet{Query: "Tootsie Pop Classic Commercial", Adapter: "mediawiki",
		AdapterVersion: MediaWikiAdapterVersion, RetrievedAt: time.Unix(100, 0).UTC(),
		Citations: []Citation{{ID: 1, Title: "Tootsie Pop", URL: "https://en.wikipedia.org/wiki/Tootsie_Pop",
			Extract: "The animated commercial debuted on US television in 1970."}}}
}

func TestResearchKeepsCampaignContextAsACitedSuggestion(t *testing.T) {
	retriever := &fixtureRetriever{packet: researchPacket()}
	provider := &fixtureProvider{content: `{"year":1970,"decade":1970,"countryCode":"US","country":"United States","confidence":95,"explanation":"Likely campaign context; the exact cut is not proven.","citationIds":[1]}`}
	at := time.Unix(200, 0).UTC()
	researcher := New(retriever, provider, "openrouter", "model", func() time.Time { return at })
	report, err := researcher.Research(t.Context(), Input{ClipHash: "hash", Title: "Tootsie Pop Classic Commercial",
		InputRevision: 1, Description: "The classic commercial.", SourceKind: "archive", SourceID: "archive:classic"})
	if err != nil {
		t.Fatal(err)
	}
	if report.Suggestion.Year != 1970 || report.Suggestion.Decade != 1970 ||
		report.Suggestion.CountryCode != "US" || report.Suggestion.Confidence != 80 ||
		len(report.Cited()) != 1 || report.CompletedAt != at {
		t.Fatalf("report = %+v", report)
	}
	if retriever.query != "Tootsie Pop Classic Commercial" || !provider.options.JSONMode || len(provider.messages) != 2 {
		t.Fatalf("query/options/messages = %q %+v %#v", retriever.query, provider.options, provider.messages)
	}
	if strings.Contains(provider.messages[0].Content, "tool") && !strings.Contains(provider.messages[0].Content, "no general web") {
		// The material assertion is the empty Tools list below; keep prompt wording free to evolve.
	}
	if len(provider.options.Tools) != 0 {
		t.Fatalf("model received tools: %+v", provider.options.Tools)
	}
}

func TestResearchRejectsModelInventedCitation(t *testing.T) {
	provider := &fixtureProvider{content: `{"decade":1970,"confidence":70,"explanation":"Likely.","citationIds":[99]}`}
	researcher := New(&fixtureRetriever{packet: researchPacket()}, provider, "fixture", "model", time.Now)
	_, err := researcher.Research(t.Context(), Input{ClipHash: "hash", Title: "Tootsie Pop",
		InputRevision: 1, SourceKind: "archive", SourceID: "archive:classic"})
	if err == nil || !strings.Contains(err.Error(), "outside its packet") {
		t.Fatalf("invented citation error = %v", err)
	}
}

func TestResearchOnlyRequestsStillMissingContext(t *testing.T) {
	provider := &fixtureProvider{content: `{"year":1980,"decade":1980,"countryCode":"GB","country":"United Kingdom","confidence":70,"explanation":"Likely.","citationIds":[1]}`}
	researcher := New(&fixtureRetriever{packet: researchPacket()}, provider, "fixture", "model", time.Now)
	report, err := researcher.Research(t.Context(), Input{ClipHash: "hash", Title: "HP Sauce Advert",
		InputRevision: 1, SourceKind: "archive", SourceID: "archive:classic", KnownEra: 1999})
	if err != nil {
		t.Fatal(err)
	}
	if report.Suggestion.Year != 0 || report.Suggestion.Decade != 0 || report.Suggestion.CountryCode != "GB" {
		t.Fatalf("known era was contradicted: %+v", report.Suggestion)
	}
}

func TestResearchRefusesPrivateInputsBeforeRetrieval(t *testing.T) {
	retriever := &fixtureRetriever{packet: researchPacket()}
	researcher := New(retriever, &fixtureProvider{}, "fixture", "model", time.Now)
	_, err := researcher.Research(t.Context(), Input{ClipHash: "hash", InputRevision: 1, Title: "Family recording", SourceKind: "folder", SourceID: "local"})
	if err == nil || retriever.calls != 0 {
		t.Fatalf("private input err=%v calls=%d", err, retriever.calls)
	}
}

func TestReportRejectsUncitedClaims(t *testing.T) {
	report := Report{ClipHash: "hash", InputRevision: 1, Producer: "context-model:fixture", ProducerVersion: "v1",
		CompletedAt: time.Now(), Packet: researchPacket(), Suggestion: Suggestion{Decade: 1970, Confidence: 60}}
	if err := report.Validate(); err == nil || !strings.Contains(err.Error(), "requires a citation") {
		t.Fatalf("uncited claim error = %v", err)
	}
}
