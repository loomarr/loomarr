package fillerresearch

import (
	"context"
	"errors"
	"testing"
	"time"
)

type scriptedRetriever struct {
	adapter string
	version string
	packet  Packet
	err     error
}

func (r scriptedRetriever) Identity() (string, string) { return r.adapter, r.version }
func (r scriptedRetriever) Retrieve(context.Context, Lookup) (Packet, error) {
	return r.packet, r.err
}

func TestFederatedMergesDeduplicatesCapsAndKeepsStableIdentity(t *testing.T) {
	one := time.Unix(100, 0).UTC()
	two := time.Unix(200, 0).UTC()
	packet := func(adapter, version string, at time.Time, urls ...string) Packet {
		p := Packet{Query: "Tootsie Pop", Adapter: adapter, AdapterVersion: version, RetrievedAt: at}
		for i, value := range urls {
			p.Citations = append(p.Citations, Citation{ID: i + 1, Title: value, URL: value, Extract: "evidence"})
		}
		return p
	}
	wikiURLs := []string{"https://en.wikipedia.org/wiki/One", "https://en.wikipedia.org/wiki/Two", "https://en.wikipedia.org/wiki/Three"}
	archiveURLs := []string{wikiURLs[1], "https://archive.org/details/four", "https://archive.org/details/five"}
	federated, err := NewFederated(
		scriptedRetriever{adapter: "mediawiki", version: "v2", packet: packet("mediawiki", "v2", one, wikiURLs...)},
		scriptedRetriever{adapter: "archive-search", version: "v1", packet: packet("archive-search", "v1", two, archiveURLs...)},
	)
	if err != nil {
		t.Fatal(err)
	}
	got, err := federated.Retrieve(t.Context(), Lookup{Title: "Tootsie Pop"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Adapter != "federated" || got.AdapterVersion != "federated-v1:mediawiki:v2+archive-search:v1" ||
		got.RetrievedAt != two || len(got.Citations) != MaxCitations {
		t.Fatalf("packet = %+v", got)
	}
	for i, citation := range got.Citations {
		if citation.ID != i+1 {
			t.Fatalf("citation ids = %+v", got.Citations)
		}
	}
	wantURLs := []string{wikiURLs[0], archiveURLs[0], archiveURLs[1], wikiURLs[2], archiveURLs[2]}
	for i, want := range wantURLs {
		if got.Citations[i].URL != want {
			t.Fatalf("round-robin citation %d = %q, want %q", i, got.Citations[i].URL, want)
		}
	}
}

func TestFederatedContinuesAfterOneAdapterFails(t *testing.T) {
	good := Packet{Query: "HP Sauce", Adapter: "archive-search", AdapterVersion: "v1",
		RetrievedAt: time.Unix(100, 0).UTC(), Citations: []Citation{{ID: 1, Title: "HP Sauce", URL: "https://archive.org/details/hp", Extract: "British advert"}}}
	federated, err := NewFederated(
		scriptedRetriever{adapter: "mediawiki", version: "v2", err: errors.New("offline")},
		scriptedRetriever{adapter: "archive-search", version: "v1", packet: good},
	)
	if err != nil {
		t.Fatal(err)
	}
	got, err := federated.Retrieve(t.Context(), Lookup{Title: "HP Sauce Advert"})
	if err != nil || len(got.Citations) != 1 || got.AdapterVersion != "federated-v1:mediawiki:v2+archive-search:v1" {
		t.Fatalf("packet=%+v err=%v", got, err)
	}
}

func TestFederatedRequiresTwoIdentifiedAdapters(t *testing.T) {
	if _, err := NewFederated(scriptedRetriever{adapter: "only", version: "v1"}); err == nil {
		t.Fatal("one adapter accepted")
	}
	if _, err := NewFederated(scriptedRetriever{}); err == nil {
		t.Fatal("unidentified adapter accepted")
	}
}
