package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestCaptureExhaustsCategoryContinuationAndBatchesMediaInfo(t *testing.T) {
	calls := 0
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("User-Agent") != "test" || r.URL.Query().Get("maxlag") != "5" {
			t.Fatalf("identity/maxlag = %q/%q", r.Header.Get("User-Agent"), r.URL.Query().Get("maxlag"))
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Query().Get("action") {
		case "query":
			start := 1
			response := categoryResponse{}
			if r.URL.Query().Get("gcmcontinue") == "next" {
				start = 6
			} else {
				response.Continue.Category = "next"
			}
			for i := start; i < start+5; i++ {
				response.Query.Pages = append(response.Query.Pages, testPage(server.URL, i))
			}
			if err := json.NewEncoder(w).Encode(response); err != nil {
				t.Fatal(err)
			}
		case "wbgetentities":
			response := mediaInfoResponse{Entities: map[string]json.RawMessage{}}
			for _, id := range strings.Split(r.URL.Query().Get("ids"), "|") {
				pageID, err := strconv.ParseInt(strings.TrimPrefix(id, "M"), 10, 64)
				if err != nil {
					t.Fatal(err)
				}
				entity := mediaEntity{PageID: pageID, LastRevID: pageID + 100, Modified: time.Date(2026, 8, 26, 12, 0, 0, 0, time.UTC), Statements: map[string]json.RawMessage{"P275": json.RawMessage(`[{"rank":"normal"}]`)}}
				raw, err := json.Marshal(entity)
				if err != nil {
					t.Fatal(err)
				}
				response.Entities[id] = raw
			}
			if err := json.NewEncoder(w).Encode(response); err != nil {
				t.Fatal(err)
			}
		default:
			t.Fatalf("action = %q", r.URL.Query().Get("action"))
		}
	}))
	defer server.Close()
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	opts := options{apiBase: server.URL, apiHost: u.Hostname(), uploadHost: u.Hostname(), category: "Advertising videos", roleHint: "commercial", cacheDir: t.TempDir(), userAgent: "test", maxRequests: 3, maxPages: 2, maxItems: 10, maxResponseBytes: 1 << 20, maxItemBytes: 1000, maxTotalBytes: 10000, maxWallTime: time.Second, http: server.Client()}
	lane, err := capture(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 3 || lane.RequestsUsed != 3 || len(lane.Cases) != 10 || lane.PredictedMediaBytes != 1000 {
		t.Fatalf("calls=%d lane=%+v", calls, lane)
	}
	if !strings.Contains(strings.Join(lane.Cases[0].RightsAssertions, "\n"), "P7482: null") {
		t.Fatalf("assertions = %v", lane.Cases[0].RightsAssertions)
	}
}

func TestCategoryURLPinsDirectRootVideoEvidence(t *testing.T) {
	raw, err := categoryURL(defaultAPIBase, "Advertising videos", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"generator=categorymembers", "gcmtype=file", "gcmlimit=50", "maxlag=5", "iiprop="} {
		if !strings.Contains(raw, required) {
			t.Fatalf("URL %s omitted %s", raw, required)
		}
	}
}

func testPage(base string, id int) commonsPage {
	pageID := int64(id)
	return commonsPage{PageID: pageID, Title: "File:Commercial " + strconv.Itoa(id) + ".webm", ImageInfo: []imageInfo{{Timestamp: time.Date(2026, 8, 26, 11, 0, 0, 0, time.UTC), User: "uploader", Size: 100, URL: base + "/media/" + strconv.Itoa(id) + ".webm", DescriptionURL: base + "/wiki/File:" + strconv.Itoa(id), SHA1: strings.Repeat(strconv.Itoa(id%10), 40), MIME: "video/webm", MediaType: "VIDEO", ExtMetadata: map[string]metadataValue{"LicenseShortName": {Value: "CC BY 4.0", Source: "commons-desc-page"}}}}}
}
