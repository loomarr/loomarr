package api_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/mantonx/loomarr/internal/filler"
)

type coverageBody struct {
	Rungs []struct {
		Level string `json:"level"`
		Clips int    `json:"clips"`
	} `json:"rungs"`
	Level string `json:"level"`
	Total int    `json:"total"`
}

func getCoverage(t *testing.T, url, token string) (*http.Response, coverageBody) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = res.Body.Close() })

	var body coverageBody
	if res.StatusCode == http.StatusOK {
		if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
			t.Fatalf("decode: %v", err)
		}
	}
	return res, body
}

// The meter's whole claim is that it describes the same ladder the pods come from, so the
// route renders the rungs verbatim — tightest first, with the level a break would actually
// resolve at.
func TestChannelFillerCoverage_RendersTheLadder(t *testing.T) {
	srv, _, fp := newPodsServer(t)
	fp.coverage = filler.CoverageReport{
		Rungs: []filler.RungCoverage{
			{Level: filler.MatchExact, Clips: 4},
			{Level: filler.MatchWidened, Clips: 5},
			{Level: filler.MatchAudience, Clips: 9},
		},
		Level: filler.MatchExact,
		Total: 9,
	}

	res, body := getCoverage(t, srv.URL+"/v1/channels/ch-1/filler/coverage", memberToken)

	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if body.Level != "exact" {
		t.Errorf("level = %q, want exact", body.Level)
	}
	// Order is load-bearing: the UI reads the rungs positionally, so a reversed slice
	// renders "mostly loose matches" for a catalog that is mostly exact.
	want := []string{"exact", "widened", "audience"}
	if len(body.Rungs) != len(want) {
		t.Fatalf("got %d rungs, want %d", len(body.Rungs), len(want))
	}
	for i, w := range want {
		if body.Rungs[i].Level != w {
			t.Errorf("rung %d = %q, want %q", i, body.Rungs[i].Level, w)
		}
	}
	// Total is the widest rung, NOT a sum — the rungs nest, and 4+5+9 would report a
	// catalog twice its real size.
	if body.Total != 9 {
		t.Errorf("total = %d, want 9 (the widest rung, not the sum)", body.Total)
	}
}

// It must ask about the channel in the URL. A handler that dropped the id would report some
// other channel's coverage, which is exactly the class of wrong-but-plausible answer a meter
// makes hardest to notice.
func TestChannelFillerCoverage_AsksAboutTheRequestedChannel(t *testing.T) {
	srv, _, fp := newPodsServer(t)

	if res, _ := getCoverage(t, srv.URL+"/v1/channels/ch-1/filler/coverage", memberToken); res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	if len(fp.asked) != 1 || fp.asked[0] != "ch-1" {
		t.Errorf("service asked about %v, want [ch-1]", fp.asked)
	}
}

// ⚠ An empty catalog reports `rungs: []`, never `null`. A JSON null would make every consumer
// guard before iterating, and "no rungs" is a real answer (nothing configured), not a missing
// one.
func TestChannelFillerCoverage_EmptyRungsAreAnArrayNotNull(t *testing.T) {
	srv, _, fp := newPodsServer(t)
	fp.coverage = filler.CoverageReport{Level: filler.MatchBumperCard}

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/v1/channels/ch-1/filler/coverage", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+memberToken)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()

	var raw map[string]json.RawMessage
	if err := json.NewDecoder(res.Body).Decode(&raw); err != nil {
		t.Fatal(err)
	}
	if got := string(raw["rungs"]); got != "[]" {
		t.Errorf("rungs = %s, want [] — a null forces every caller to guard", got)
	}
	// bumper_card is the honest "nothing matches, breaks run on the embedded card" answer,
	// distinct from a zero that reads as a loading state.
	if got := string(raw["level"]); got != `"bumper_card"` {
		t.Errorf("level = %s, want \"bumper_card\"", got)
	}
}

func TestChannelFillerCoverage_UnknownChannelIs404(t *testing.T) {
	srv, _, _ := newPodsServer(t)

	res, _ := getCoverage(t, srv.URL+"/v1/channels/nope/filler/coverage", memberToken)
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", res.StatusCode)
	}
}

// Read-only, so member-visible — the same rule as the pod preview it describes.
func TestChannelFillerCoverage_RequiresAuth(t *testing.T) {
	srv, _, _ := newPodsServer(t)

	anon, _ := getCoverage(t, srv.URL+"/v1/channels/ch-1/filler/coverage", "")
	if anon.StatusCode == http.StatusOK {
		t.Error("served coverage to an anonymous caller")
	}
}
