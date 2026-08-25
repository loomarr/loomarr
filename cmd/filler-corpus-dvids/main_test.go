package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/testkit/httpfixture"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return fn(request) }

func TestParseCandidateRequiresMatchingInstitutionalPublicDomainEvidence(t *testing.T) {
	retrievedAt := time.Date(2026, 8, 25, 18, 0, 0, 0, time.UTC)
	ref := searchAsset{ID: "video:123", Type: "video", Category: "Commercials", Duration: 30, Timestamp: "2026-08-25T17:00:00Z", URL: "https://www.dvidshub.net/video/123/recruiting-spot"}
	detail := []byte(`{"results":{"id":"video:123","type":"video","title":"Recruiting spot","description":"A short federal recruiting spot.","category":"Commercials","unit_id":"42","unit_name":"Defense Media Activity","timestamp":"2026-08-25T17:00:00Z","url":"https://www.dvidshub.net/video/123/recruiting-spot","credit":[{"id":7,"name":"Jane Doe","rank":"SSgt."}],"virin":"260825-F-AB123-001","duration":30,"files":[{"src":"https://cdn.dvidshub.net/video/123-480.mp4","type":"video/mp4","height":480,"width":854,"size":12000000,"bitrate":800},{"src":"https://cdn.dvidshub.net/video/123-720.mp4","type":"video/mp4","height":720,"width":1280,"size":24000000,"bitrate":1500},{"src":"https://cdn.dvidshub.net/video/123-master.mp4","type":"video/mp4","height":1080,"width":1920,"size":90000000,"bitrate":6000}]}}`)
	page := []byte(`<h4 class="public-domain"> PUBLIC DOMAIN &nbsp; </h4><p class="copyright_info">This work, Recruiting spot, by Jane Doe, identified by DVIDS, must comply with the restrictions shown on https://www.dvidshub.net/about/copyright.</p>`)

	got, ok := parseCandidate(ref, detail, page, "video-123.json", "video-123.html", retrievedAt, 50_000_000)
	if !ok {
		t.Fatal("institutionally marked public-domain candidate was rejected")
	}
	if got.Identifier != "dvids-video-123" || got.File.URL != "https://cdn.dvidshub.net/video/123-720.mp4" || got.File.Bytes != 24_000_000 || got.VIRIN != "260825-F-AB123-001" || got.Unit != "Defense Media Activity" || got.MetadataSHA256 == "" || got.RightsPageSHA256 == "" {
		t.Fatalf("candidate = %+v", got)
	}
	if len(got.Creator) != 1 || !strings.Contains(got.Creator[0], "Jane Doe") {
		t.Fatalf("creator = %v", got.Creator)
	}

	if _, ok := parseCandidate(ref, detail, []byte(`<p>Copyright status not supplied.</p>`), "video-123.json", "video-123.html", retrievedAt, 50_000_000); ok {
		t.Fatal("candidate without the item-level PUBLIC DOMAIN block was accepted")
	}
}

func TestBuildInventoryIsBoundedAndNeverPersistsAPIKey(t *testing.T) {
	secret := "key-super-secret/+"
	search := `{"page_info":{"total_results":1,"results_per_page":50},"results":[{"id":"video:123","type":"video","title":"Recruiting spot","category":"Commercials","duration":30,"timestamp":"2026-08-25T17:00:00Z","url":"https://www.dvidshub.net/video/123/recruiting-spot"}]}`
	detail := `{"results":{"id":"video:123","type":"video","title":"Recruiting spot","description":"A short federal recruiting spot.","date":"2026-08-25T16:00:00Z","category":"Commercials","unit_id":"42","unit_name":"Defense Media Activity","timestamp":"2026-08-25T17:00:00Z","url":"https://www.dvidshub.net/video/123/recruiting-spot","credit":[{"id":7,"name":"Jane Doe","rank":"SSgt."}],"virin":"260825-F-AB123-001","duration":30,"files":[{"src":"https://cdn.dvidshub.net/video/123-720.mp4","type":"video/mp4","height":720,"width":1280,"size":24000000,"bitrate":1500}]}}`
	page := `<h4 class="public-domain">PUBLIC DOMAIN &nbsp;</h4><p class="copyright_info">This work, Recruiting spot, by Jane Doe, identified by DVIDS, must comply with the restrictions shown on https://www.dvidshub.net/about/copyright.</p>`
	transport := httpfixture.NewScriptedTransport(
		httpfixture.Step{Response: response(http.StatusOK, search)},
		httpfixture.Step{Response: response(http.StatusOK, detail)},
		httpfixture.Step{Response: response(http.StatusOK, page)},
	)
	opts := options{
		apiBaseURL: "https://api.dvidshub.net", siteBaseURL: "https://www.dvidshub.net", apiKey: secret,
		categories: []string{"Commercials"}, cacheDir: t.TempDir(), userAgent: "Loomarr test contact@example.invalid",
		snapshotAt: time.Date(2026, 8, 25, 18, 0, 0, 0, time.UTC), maxRequests: 3, maxItems: 1,
		maxDuration: 90, maxItemBytes: 50_000_000, maxTotalBytes: 50_000_000,
	}
	got, err := buildInventory(context.Background(), &http.Client{Transport: transport}, opts)
	if err != nil {
		t.Fatal(err)
	}
	if got.RequestsUsed != 3 || len(got.Cases) != 1 || got.SelectedBytes != 24_000_000 || got.Source != "dvids" {
		t.Fatalf("inventory = %+v", got)
	}
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), secret) {
		t.Fatal("API key leaked into the inventory")
	}
	requests := transport.Requests()
	if len(requests) != 3 || !strings.Contains(requests[0].URL, "category=Commercials") || !strings.Contains(requests[0].URL, "sort=timestamp") || !strings.Contains(requests[0].URL, "to_duration=90") || !strings.Contains(requests[1].URL, "id=video%3A123") || requests[2].URL != "https://www.dvidshub.net/video/123/recruiting-spot" {
		t.Fatalf("requests = %+v", requests)
	}
}

func TestRunRequiresHardCeilingsAndSecret(t *testing.T) {
	if code := run(nil, io.Discard, io.Discard); code != 2 {
		t.Fatalf("run() = %d, want usage failure", code)
	}
	if _, err := parseCategories("Commercials,not-a-category"); err == nil {
		t.Fatal("unknown DVIDS category was accepted")
	}
	categories, err := parseCategories("PSA,Commercials,PSA")
	if err != nil || strings.Join(categories, ",") != "Commercials,PSA" {
		t.Fatalf("categories = %v, %v", categories, err)
	}
	if got := cacheSlug("In The Fight"); got != "in-the-fight" {
		t.Fatalf("cache slug = %q", got)
	}
}

func TestFetcherRedactsAPIKeyFromTransportErrors(t *testing.T) {
	secret := "key-super-secret/+"
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return nil, errors.New("dial failed for " + request.URL.String())
	})}
	fetch := &fetcher{client: client, userAgent: "Loomarr test", maxRequests: 1, apiKey: secret}
	requestURL, evidenceURL, err := dvidsDetailURL(defaultAPIBaseURL, secret, "video:123")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = fetch.get(context.Background(), requestURL, evidenceURL, t.TempDir()+"/detail.json", maxDetailBody)
	if err == nil {
		t.Fatal("transport failure was not returned")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("API key leaked in error: %v", err)
	}
}

func response(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Status: http.StatusText(status), Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}
