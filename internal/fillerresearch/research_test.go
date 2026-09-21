package fillerresearch

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/llm"
)

type fixtureRetriever struct {
	packet Packet
	lookup Lookup
	calls  int
	err    error
}

type sequenceProvider struct {
	responses []string
	calls     int
}

func (*sequenceProvider) Name() string { return "fixture" }
func (p *sequenceProvider) Chat(context.Context, []llm.Message, llm.ChatOptions) (llm.Response, error) {
	if p.calls >= len(p.responses) {
		return llm.Response{}, errors.New("unexpected model call")
	}
	response := p.responses[p.calls]
	p.calls++
	return llm.Response{Content: response}, nil
}

func (r *fixtureRetriever) Identity() (string, string) { return "fixture", "fixture-v1" }
func (r *fixtureRetriever) Retrieve(_ context.Context, lookup Lookup) (Packet, error) {
	r.calls++
	r.lookup = lookup
	return r.packet, r.err
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
	if retriever.lookup.Title != "Tootsie Pop Classic Commercial" || retriever.lookup.Description != "The classic commercial." ||
		!provider.options.JSONMode || len(provider.messages) != 2 {
		t.Fatalf("lookup/options/messages = %+v %+v %#v", retriever.lookup, provider.options, provider.messages)
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

func TestResearchIncludesCanonicalSourceMetadataAsEvidence(t *testing.T) {
	provider := &fixtureProvider{content: `{"decade":1980,"countryCode":"GB","country":"United Kingdom","confidence":70,"explanation":"Likely from the source description.","citationIds":[1]}`}
	researcher := New(&fixtureRetriever{packet: researchPacket()}, provider, "fixture", "model", time.Now)
	report, err := researcher.Research(t.Context(), Input{ClipHash: "hash", Title: "HP Sauce Advert",
		Description: "British condiment advertisement from the 1980s.", InputRevision: 1,
		SourceKind: "archive", SourceID: "archive:classic", SourceURL: "https://archive.org/details/hp-sauce"})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Packet.Citations) != 2 || report.Packet.Citations[0].ID != 1 ||
		report.Packet.Citations[0].URL != "https://archive.org/details/hp-sauce" ||
		report.Packet.Citations[1].ID != 2 || len(report.Cited()) != 1 {
		t.Fatalf("packet = %+v", report.Packet)
	}
}

func TestResearchUsesExactSourceMetadataWhenStructuredSearchMisses(t *testing.T) {
	retriever := &fixtureRetriever{err: errors.New("no related results")}
	provider := &fixtureProvider{content: `{"decade":1980,"countryCode":"GB","country":"United Kingdom","confidence":70,"explanation":"The exact source description identifies a British advert.","citationIds":[1]}`}
	researcher := New(retriever, provider, "fixture", "model", func() time.Time { return time.Unix(300, 0).UTC() })
	report, err := researcher.Research(t.Context(), Input{ClipHash: "hash", Title: "HP Sauce Advert",
		Description: "British condiment advertisement from the 1980s.", InputRevision: 1,
		SourceKind: "archive", SourceID: "archive:classic", SourceURL: "https://archive.org/details/hp-sauce"})
	if err != nil {
		t.Fatal(err)
	}
	if report.Suggestion.CountryCode != "GB" || len(report.Packet.Citations) != 1 ||
		report.Packet.Citations[0].URL != "https://archive.org/details/hp-sauce" || len(report.Cited()) != 1 {
		t.Fatalf("source-only report = %+v", report)
	}
}

func TestResearchRejectsAnArbitraryClaimedSourceURLBeforeRetrieval(t *testing.T) {
	retriever := &fixtureRetriever{packet: researchPacket()}
	researcher := New(retriever, &fixtureProvider{}, "fixture", "model", time.Now)
	_, err := researcher.Research(t.Context(), Input{ClipHash: "hash", Title: "HP Sauce Advert",
		InputRevision: 1, SourceKind: "archive", SourceID: "archive:classic", SourceURL: "https://example.com/details/hp"})
	if err == nil || retriever.calls != 0 {
		t.Fatalf("arbitrary source err=%v calls=%d", err, retriever.calls)
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

func TestReportCountryFactRequiresCitedConfidentCountry(t *testing.T) {
	report := Report{ClipHash: "hash", InputRevision: 1, Producer: "context-model:fixture", ProducerVersion: "v1",
		CompletedAt: time.Now(), Packet: researchPacket(), Suggestion: Suggestion{
			CountryCode: "US", Country: "United States", Confidence: MinCountryProjectionConfidence,
			CitationIDs: []int{1},
		}}
	country, citations, ok := report.CountryFact()
	if !ok || country != "US" || len(citations) != 1 || citations[0] != 1 {
		t.Fatalf("CountryFact() = %q, %v, %v", country, citations, ok)
	}
	report.Suggestion.Confidence--
	if _, _, ok := report.CountryFact(); ok {
		t.Fatal("low-confidence country became authoritative")
	}
}

func TestResearchUsesWebFallbackOnlyAfterStructuredEvidenceAbstains(t *testing.T) {
	primary := &fixtureRetriever{packet: researchPacket()}
	fallbackPacket := Packet{Query: "Tootsie Pop", Adapter: "web", AdapterVersion: "web-v1", RetrievedAt: time.Unix(150, 0).UTC(),
		Citations: []Citation{{ID: 1, Title: "Campaign history", URL: "https://example.com/history", Extract: "The campaign started in the United States in 1970."}}}
	fallback := &fixtureRetriever{packet: fallbackPacket}
	provider := &sequenceProvider{responses: []string{
		`{"confidence":0,"explanation":"Not enough structured evidence.","citationIds":[]}`,
		`{"decade":1970,"countryCode":"US","country":"United States","confidence":60,"explanation":"Likely campaign context.","citationIds":[2]}`,
	}}
	report, err := New(primary, provider, "fixture", "model", time.Now).WithFallback(fallback).Research(t.Context(), Input{
		ClipHash: "hash", Title: "Tootsie Pop", InputRevision: 1, SourceKind: "archive", SourceID: "archive:classic",
	})
	if err != nil {
		t.Fatal(err)
	}
	if primary.calls != 1 || fallback.calls != 1 || provider.calls != 2 || report.Suggestion.Decade != 1970 || len(report.Packet.Citations) != 2 {
		t.Fatalf("primary=%d fallback=%d model=%d report=%+v", primary.calls, fallback.calls, provider.calls, report)
	}
}

func TestResearchSkipsFallbackWhenStructuredEvidenceAnswersAndPreservesItOnFallbackFailure(t *testing.T) {
	for _, tc := range []struct {
		name        string
		first       string
		fallbackErr error
		wantCalls   int
	}{
		{name: "answered", first: `{"decade":1970,"countryCode":"US","country":"United States","confidence":60,"explanation":"Likely.","citationIds":[1]}`, wantCalls: 0},
		{name: "fallback unavailable", first: `{"confidence":0,"explanation":"Unknown.","citationIds":[]}`, fallbackErr: errors.New("offline"), wantCalls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fallback := &fixtureRetriever{err: tc.fallbackErr}
			provider := &sequenceProvider{responses: []string{tc.first}}
			report, err := New(&fixtureRetriever{packet: researchPacket()}, provider, "fixture", "model", time.Now).
				WithFallback(fallback).Research(t.Context(), Input{ClipHash: "hash", Title: "Tootsie Pop", InputRevision: 1,
				SourceKind: "archive", SourceID: "archive:classic"})
			if err != nil || fallback.calls != tc.wantCalls || provider.calls != 1 {
				t.Fatalf("report=%+v err=%v fallback=%d model=%d", report, err, fallback.calls, provider.calls)
			}
		})
	}
}

func TestResearchLiveFixtureInvokesWebOnceOnlyAfterStructuredAbstention(t *testing.T) {
	requests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"title":"Campaign archive","url":"https://example.org/campaign","content":"The campaign ran in the United States during the 1970s."}]}`))
	}))
	defer server.Close()
	ledger := &memoryWebLedger{}
	web := NewWeb(func() WebConfig {
		return WebConfig{Provider: WebProviderSearXNG, SearXNGURL: server.URL, MonthlyLimit: 5}
	}, ledger, WebOptions{Client: server.Client(), Now: func() time.Time { return time.Unix(500, 0).UTC() }})
	primary := &fixtureRetriever{packet: researchPacket()}
	provider := &sequenceProvider{responses: []string{
		`{"confidence":0,"explanation":"The structured source does not establish the requested details.","citationIds":[]}`,
		`{"decade":1970,"countryCode":"US","country":"United States","confidence":60,"explanation":"Likely campaign context.","citationIds":[2]}`,
	}}

	report, err := New(primary, provider, "fixture", "model", time.Now).WithFallback(web).Research(t.Context(), Input{
		ClipHash: "hash", Title: "Hard-to-identify advert", InputRevision: 1,
		SourceKind: "archive", SourceID: "archive:hard",
	})
	if err != nil {
		t.Fatal(err)
	}
	if primary.calls != 1 || provider.calls != 2 || requests != 1 || ledger.usage.RequestCount != 1 {
		t.Fatalf("primary=%d model=%d web=%d usage=%+v", primary.calls, provider.calls, requests, ledger.usage)
	}
	if report.Suggestion.Decade != 1970 || report.Suggestion.CountryCode != "US" {
		t.Fatalf("suggestion = %+v", report.Suggestion)
	}
}
