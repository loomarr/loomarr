package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"
)

func TestCaptureFreezesOnlyAuthoredFirstPartyPageMediaPairs(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			index := r.URL.Query().Get("case")
			_, _ = fmt.Fprintf(w, "CDC Content Source: NCEH /media.mp4?case=%s", index)
		case http.MethodHead:
			w.Header().Set("Content-Type", "video/mp4")
			w.Header().Set("Content-Length", "100")
		default:
			t.Fatalf("request = %s %s", r.Method, r.URL)
		}
	}))
	defer server.Close()
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	draft := seed{SchemaVersion: 1, Authority: "cdc.gov"}
	for i := range 10 {
		value := strconv.Itoa(i)
		draft.Cases = append(draft.Cases, seedCase{ItemID: "case-" + value, Title: "Case " + value, RoleHints: []string{"PSA"}, ItemURL: server.URL + "/page?case=" + value, MediaURL: server.URL + "/media.mp4?case=" + value, RightsAssertions: []string{"CDC page source assertion"}, RequiredPageText: []string{"Content Source: NCEH"}})
	}
	opts := options{cacheDir: t.TempDir(), userAgent: "test", pageHost: u.Hostname(), mediaHost: u.Hostname(), maxRequests: 20, maxItems: 10, maxResponseBytes: 1 << 20, maxItemBytes: 1000, maxTotalBytes: 10000, maxWallTime: time.Second, http: server.Client()}
	lane, err := capture(context.Background(), draft, opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(lane.Cases) != 10 || lane.RequestsUsed != 20 || lane.PredictedMediaBytes != 1000 {
		t.Fatalf("lane = %+v", lane)
	}
}

func TestValidateSeedCaseRejectsForeignMedia(t *testing.T) {
	candidate := seedCase{ItemID: "x", Title: "x", RoleHints: []string{"PSA"}, ItemURL: "https://www.cdc.gov/page", MediaURL: "https://example.com/media.mp4", RightsAssertions: []string{"source"}, RequiredPageText: []string{"source"}}
	if err := validateSeedCase(candidate, "www.cdc.gov", "www.cdc.gov"); err == nil {
		t.Fatal("foreign media passed")
	}
}
