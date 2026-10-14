package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/filler"
	"github.com/loomarr/loomarr/internal/fillerenrichment"
)

func TestFillerExactClipIncludesServerOwnedEnrichmentPresentation(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	putClip(t, st, filler.Clip{Hash: "details", Path: "details.mp4", Name: "Details", Kind: filler.Commercial})
	now := time.Unix(1_700_000_500, 0).UTC()
	state := fillerenrichment.State{
		ClipHash: "details", Axis: fillerenrichment.AxisKind, Status: fillerenrichment.StatusComplete,
		Value: fillerenrichment.Value{Text: "commercial"},
		Evidence: fillerenrichment.Evidence{Kind: fillerenrichment.EvidenceItem,
			Reference: "item.title", Confidence: 100, Producer: "metadata", ProducerVersion: "1", ObservedAt: now},
	}
	if _, changed, err := st.ApplyFillerEnrichment(context.Background(), state, now); err != nil || !changed {
		t.Fatalf("seed enrichment = changed %v, err %v", changed, err)
	}

	type enrichment struct {
		State string `json:"state"`
		Facts []struct {
			Axis     string `json:"axis"`
			Evidence string `json:"evidence"`
		} `json:"facts"`
	}
	type clip struct {
		Hash       string      `json:"hash"`
		Enrichment *enrichment `json:"enrichment"`
	}
	read := func(path string) []clip {
		t.Helper()
		resp := do(t, srv, http.MethodGet, path, adminToken, "")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s = %d", path, resp.StatusCode)
		}
		var body struct {
			Clips []clip `json:"clips"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		return body.Clips
	}

	exact := read("/v1/filler?hashes=details")
	if len(exact) != 1 || exact[0].Enrichment == nil || exact[0].Enrichment.State != "details_limited" ||
		len(exact[0].Enrichment.Facts) != 1 || exact[0].Enrichment.Facts[0].Axis != "kind" ||
		exact[0].Enrichment.Facts[0].Evidence != "item_metadata" {
		t.Fatalf("exact clip enrichment = %+v", exact)
	}
	page := read("/v1/filler")
	if len(page) != 1 || page[0].Enrichment != nil {
		t.Fatalf("catalog page should keep detail projection out of row payloads: %+v", page)
	}
}
