package fillercorpus

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestCaptureMetInventoryFreezesOnlyObjectValidatedPublicDomainCandidate(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.Method + " " + request.URL.Path {
		case "GET /public/collection/v1/search":
			if request.URL.Query().Get("hasImages") != "true" || request.URL.Query().Get("q") != "venus" {
				t.Fatalf("search query = %q", request.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"total":1,"objectIDs":[195733]}`))
		case "GET /public/collection/v1/objects/195733":
			if request.Header.Get("X-Test-Original-Host") != metAPIHost {
				t.Fatalf("object host = %q", request.Header.Get("X-Test-Original-Host"))
			}
			_, _ = w.Write([]byte(`{"objectID":195733,"isPublicDomain":true,"primaryImage":"https://images.metmuseum.org/CRDImages/es/original/DP-919-001.jpg","title":"Venus","artistDisplayName":"Massimiliano Soldani","objectDate":"18th century","objectURL":"https://www.metmuseum.org/art/collection/search/195733","repository":"Metropolitan Museum of Art, New York, NY","creditLine":"Purchase, 1916","tags":[{"term":"Female Nudes"},{"term":"Sculpture"}]}`))
		case "HEAD /CRDImages/es/original/DP-919-001.jpg":
			if request.Header.Get("X-Test-Original-Host") != metImageHost {
				t.Fatalf("image host = %q", request.Header.Get("X-Test-Original-Host"))
			}
			w.Header().Set("Content-Type", "image/jpeg")
			w.Header().Set("Content-Length", "1156190")
		default:
			t.Fatalf("unexpected request %s %s", request.Method, request.URL.String())
		}
	}))
	defer server.Close()

	config := metTestConfig(t, server)
	inventory, err := CaptureMetInventory(context.Background(), config)
	if err != nil {
		t.Fatal(err)
	}
	if failures := ValidateInventory(inventory); len(failures) != 0 {
		t.Fatalf("inventory failures = %v", failures)
	}
	if len(inventory.Captures) != 1 || inventory.Captures[0].RequestsUsed != 3 || inventory.Captures[0].PredictedMediaBytes != 1156190 || inventory.Captures[0].SearchSHA256 == "" {
		t.Fatalf("capture = %+v", inventory.Captures)
	}
	if !strings.HasPrefix(inventory.Captures[0].Collection, "selection-sha256:") {
		t.Fatalf("selection identity = %q", inventory.Captures[0].Collection)
	}
	item := inventory.Cases[0]
	if item.CaseID != "metmuseum.org/collection/195733" || item.SourceFamily != "met-object:195733" ||
		len(item.Creator) != 1 || item.Creator[0] != "Massimiliano Soldani" || item.MetadataCache == "" ||
		len(item.Collection) != 2 || item.Collection[1] != "search-term:venus" ||
		len(item.SubjectTerms) != 2 || item.SubjectTerms[0] != "Female Nudes" || item.SubjectTerms[1] != "Sculpture" ||
		item.Representation.MIMEType != "image/jpeg" || item.Representation.Bytes != 1156190 ||
		item.Representation.URL != "https://images.metmuseum.org/CRDImages/es/original/DP-919-001.jpg" {
		t.Fatalf("case = %+v", item)
	}
}

func TestCaptureMetInventoryRequiresSourceAuthoredSubjectBeforeImageProbe(t *testing.T) {
	headRequests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.Method + " " + request.URL.Path {
		case "GET /public/collection/v1/search":
			_, _ = w.Write([]byte(`{"total":1,"objectIDs":[195733]}`))
		case "GET /public/collection/v1/objects/195733":
			_, _ = w.Write([]byte(`{"objectID":195733,"isPublicDomain":true,"primaryImage":"https://images.metmuseum.org/object.jpg","title":"Decorative vase","artistDisplayName":"Artist","objectURL":"https://www.metmuseum.org/art/collection/search/195733","tags":[{"term":"Flowers"}]}`))
		case "HEAD /object.jpg":
			headRequests++
		default:
			t.Fatalf("unexpected request %s %s", request.Method, request.URL.String())
		}
	}))
	defer server.Close()
	_, err := CaptureMetInventory(context.Background(), metTestConfig(t, server))
	if err == nil || !strings.Contains(err.Error(), "admitted 0 of 1") {
		t.Fatalf("err = %v", err)
	}
	if headRequests != 0 {
		t.Fatalf("untagged candidate triggered %d image requests", headRequests)
	}
}

func TestCaptureMetInventoryRejectsExcludedMinorSubjectBeforeImageProbe(t *testing.T) {
	headRequests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.Method + " " + request.URL.Path {
		case "GET /public/collection/v1/search":
			_, _ = w.Write([]byte(`{"total":1,"objectIDs":[195733]}`))
		case "GET /public/collection/v1/objects/195733":
			_, _ = w.Write([]byte(`{"objectID":195733,"isPublicDomain":true,"primaryImage":"https://images.metmuseum.org/object.jpg","title":"Venus with child","artistDisplayName":"Artist","objectURL":"https://www.metmuseum.org/art/collection/search/195733","tags":[{"term":"Female Nudes"},{"term":"Infants"}]}`))
		case "HEAD /object.jpg":
			headRequests++
		default:
			t.Fatalf("unexpected request %s %s", request.Method, request.URL.String())
		}
	}))
	defer server.Close()
	_, err := CaptureMetInventory(context.Background(), metTestConfig(t, server))
	if err == nil || !strings.Contains(err.Error(), "admitted 0 of 1") {
		t.Fatalf("err = %v", err)
	}
	if headRequests != 0 {
		t.Fatalf("minor-tagged candidate triggered %d image requests", headRequests)
	}
}

func TestCaptureMetInventorySearchHitCannotGrantPublicDomainStatus(t *testing.T) {
	headRequests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.Method + " " + request.URL.Path {
		case "GET /public/collection/v1/search":
			_, _ = w.Write([]byte(`{"total":1,"objectIDs":[195733]}`))
		case "GET /public/collection/v1/objects/195733":
			_, _ = w.Write([]byte(`{"objectID":195733,"isPublicDomain":false,"primaryImage":"https://images.metmuseum.org/object.jpg","title":"Venus","artistDisplayName":"Artist","objectURL":"https://www.metmuseum.org/art/collection/search/195733"}`))
		case "HEAD /object.jpg":
			headRequests++
		default:
			t.Fatalf("unexpected request %s %s", request.Method, request.URL.String())
		}
	}))
	defer server.Close()

	_, err := CaptureMetInventory(context.Background(), metTestConfig(t, server))
	if err == nil || !strings.Contains(err.Error(), "admitted 0 of 1") {
		t.Fatalf("err = %v", err)
	}
	if headRequests != 0 {
		t.Fatalf("search-only candidate triggered %d image requests", headRequests)
	}
}

func TestCaptureMetInventoryRejectsNonCanonicalTermsBeforeTransport(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("unexpected request")
	}))
	defer server.Close()
	config := metTestConfig(t, server)
	config.Terms = []string{"venus", "adam and eve"}
	config.MaxRequests = len(config.Terms) + config.MaxObjectLookups + config.MaxItems
	if _, err := CaptureMetInventory(context.Background(), config); err == nil || !strings.Contains(err.Error(), "canonical terms") {
		t.Fatalf("err = %v", err)
	}
}

func TestMetSelectionIdentityChangesWithSubjectAdmissionRule(t *testing.T) {
	terms := []string{"venus"}
	first := metSelectionDigest(terms, []string{"Female Nudes"}, []string{"Infants"})
	second := metSelectionDigest(terms, []string{"Male Nudes"}, []string{"Infants"})
	if first == second || first == metSelectionDigest([]string{"nude"}, []string{"Female Nudes"}, []string{"Infants"}) ||
		first == metSelectionDigest(terms, []string{"Female Nudes"}, []string{"Children"}) {
		t.Fatal("selection identity omitted a search or subject-admission input")
	}
}

func metTestConfig(t *testing.T, server *httptest.Server) MetCaptureConfig {
	t.Helper()
	return MetCaptureConfig{
		HTTP: metTestHTTPClient(t, server), CacheDir: t.TempDir(), UserAgent: "Loomarr test",
		Terms: []string{"venus"}, RoleHint: "policy-positive-nomination",
		RequiredSubjectTerms: []string{"Female Nudes", "Male Nudes"},
		ExcludedSubjectTerms: []string{"Children", "Infants"},
		SnapshotAt:           time.Now().Add(time.Hour).UTC(), MaxRequests: 3, MaxObjectLookups: 1, MaxItems: 1,
		MaxResponseBytes: 1 << 20, MaxItemBytes: 2 << 20, MaxTotalBytes: 2 << 20,
		Delay: 100 * time.Millisecond, MaxWallTime: 5 * time.Second,
	}
}

func metTestHTTPClient(t *testing.T, server *httptest.Server) *http.Client {
	t.Helper()
	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	transport := server.Client().Transport
	return &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		clone := request.Clone(request.Context())
		clone.URL = cloneURL(request.URL)
		clone.Header = request.Header.Clone()
		clone.Header.Set("X-Test-Original-Host", request.URL.Hostname())
		clone.URL.Scheme = serverURL.Scheme
		clone.URL.Host = serverURL.Host
		return transport.RoundTrip(clone)
	})}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (function roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return function(request)
}

func cloneURL(source *url.URL) *url.URL {
	value := *source
	return &value
}
