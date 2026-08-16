package tmdb

import (
	"context"
	"errors"
	"io"
	"net/http"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/mantonx/loomarr/internal/provision"
	"github.com/mantonx/loomarr/internal/testkit/httpfixture"
)

const testBaseURL = "https://tmdb.invalid/3"

func testResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func testClient(apiKey func() string, transport http.RoundTripper) *Client {
	return newDynamicWithHTTP(testBaseURL, apiKey, &http.Client{Transport: transport})
}

func TestDynamicAPIKey_RotatesAndClearsBetweenOperations(t *testing.T) {
	script := httpfixture.NewScriptedTransport(
		httpfixture.Step{Response: testResponse(`{"results":[]}`)},
		httpfixture.Step{Response: testResponse(`{}`)},
	)
	var key atomic.Value
	key.Store("key-a")
	var providerCalls atomic.Int64
	client := testClient(func() string {
		providerCalls.Add(1)
		return key.Load().(string)
	}, script)

	if _, err := client.Search(context.Background(), "matrix", 1); err != nil {
		t.Fatal(err)
	}
	key.Store("key-b")
	if _, err := client.Exists(context.Background(), provision.Movie, 603); err != nil {
		t.Fatal(err)
	}

	requests := script.Requests()
	if len(requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(requests))
	}
	if requests[0].Header.Get("Authorization") != "Bearer key-a" || requests[1].Header.Get("Authorization") != "Bearer key-b" {
		t.Errorf("authorization headers = [%q %q], want rotated bearer credentials", requests[0].Header.Get("Authorization"), requests[1].Header.Get("Authorization"))
	}
	for _, request := range requests {
		if strings.Contains(request.URL, "key-a") || strings.Contains(request.URL, "key-b") || strings.Contains(request.URL, "api_key") {
			t.Errorf("credential leaked into URL %q", request.URL)
		}
	}

	key.Store("  ")
	before := script.Calls()
	if _, err := client.Search(context.Background(), "matrix", 1); !errors.Is(err, ErrAPIKeyRequired) {
		t.Fatalf("Search with cleared key error = %v, want ErrAPIKeyRequired", err)
	}
	if got := script.Calls(); got != before {
		t.Errorf("cleared-key operation made %d HTTP requests, want 0", got-before)
	}
	if got := providerCalls.Load(); got != 3 {
		t.Errorf("provider calls = %d, want one per operation (3)", got)
	}
}

func TestDynamicAPIKey_EmptyFailsEveryOperationWithoutHTTP(t *testing.T) {
	script := httpfixture.NewScriptedTransport()
	client := testClient(func() string { return "" }, script)
	tests := []struct {
		name string
		run  func() error
	}{
		{"Search", func() error { _, err := client.Search(context.Background(), "matrix", 1); return err }},
		{"Discover", func() error { _, err := client.Discover(context.Background(), "", nil, 0, 0, 1); return err }},
		{"Recommendations", func() error {
			_, err := client.Recommendations(context.Background(), provision.Movie, 603, 1)
			return err
		}},
		{"Exists", func() error { _, err := client.Exists(context.Background(), provision.Movie, 0); return err }},
		{"CollectionID", func() error { _, err := client.CollectionID(context.Background(), provision.Series, 0); return err }},
		{"ContentRating", func() error { _, err := client.ContentRating(context.Background(), provision.Movie, 0); return err }},
		{"PosterURL", func() error { _, err := client.PosterURL(context.Background(), provision.Movie, 0); return err }},
		{"BackdropURL", func() error { _, err := client.BackdropURL(context.Background(), provision.Movie, 0); return err }},
		{"PosterURLByTVDB", func() error { _, err := client.PosterURLByTVDB(context.Background(), 0); return err }},
		{"EpisodeStillURLByTVDB", func() error { _, err := client.EpisodeStillURLByTVDB(context.Background(), 0, 1, 1); return err }},
		{"EpisodeStillURL", func() error { _, err := client.EpisodeStillURL(context.Background(), 0, 1, 1); return err }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.run(); !errors.Is(err, ErrAPIKeyRequired) {
				t.Errorf("error = %v, want ErrAPIKeyRequired", err)
			}
		})
	}
	if got := script.Calls(); got != 0 {
		t.Errorf("empty-key operations made %d HTTP requests, want 0", got)
	}
}

func TestDynamicAPIKey_SnapshotsMultiRequestOperation(t *testing.T) {
	script := httpfixture.NewScriptedTransport(
		httpfixture.Step{Response: testResponse(`{"results":[]}`)},
		httpfixture.Step{Response: testResponse(`{"results":[]}`)},
		httpfixture.Step{Response: testResponse(`{"results":[]}`)},
	)
	var key atomic.Value
	key.Store("key-a")
	var providerCalls atomic.Int64
	transport := httpfixture.RoundTripperFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path == "/3/discover/movie" {
			key.Store("key-b")
		}
		return script.RoundTrip(request)
	})
	client := testClient(func() string {
		providerCalls.Add(1)
		return key.Load().(string)
	}, transport)

	if _, err := client.Discover(context.Background(), "", nil, 0, 0, 10); err != nil {
		t.Fatal(err)
	}
	requests := script.Requests()
	if len(requests) != 2 {
		t.Fatalf("discover requests = %d, want movie + tv", len(requests))
	}
	for _, request := range requests {
		if got := request.Header.Get("Authorization"); got != "Bearer key-a" {
			t.Errorf("%s authorization = %q, want operation snapshot Bearer key-a", request.URL, got)
		}
	}
	if got := providerCalls.Load(); got != 1 {
		t.Errorf("provider calls during Discover = %d, want 1", got)
	}

	if _, err := client.Search(context.Background(), "matrix", 1); err != nil {
		t.Fatal(err)
	}
	requests = script.Requests()
	if got := requests[len(requests)-1].Header.Get("Authorization"); got != "Bearer key-b" {
		t.Errorf("next operation authorization = %q, want Bearer key-b", got)
	}
}

func TestDynamicAPIKey_SnapshotsNestedOperation(t *testing.T) {
	script := httpfixture.NewScriptedTransport(
		httpfixture.Step{Response: testResponse(`{"tv_results":[{"id":1396}]}`)},
		httpfixture.Step{Response: testResponse(`{"still_path":"/pilot.jpg"}`)},
	)
	var key atomic.Value
	key.Store("key-a")
	var providerCalls atomic.Int64
	transport := httpfixture.RoundTripperFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path == "/3/find/71663" {
			key.Store("key-b")
		}
		return script.RoundTrip(request)
	})
	client := testClient(func() string {
		providerCalls.Add(1)
		return key.Load().(string)
	}, transport)

	got, err := client.EpisodeStillURLByTVDB(context.Background(), 71663, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://image.tmdb.org/t/p/original/pilot.jpg" {
		t.Errorf("still URL = %q", got)
	}
	requests := script.Requests()
	if len(requests) != 2 {
		t.Fatalf("nested operation requests = %d, want find + episode", len(requests))
	}
	for _, request := range requests {
		if auth := request.Header.Get("Authorization"); auth != "Bearer key-a" {
			t.Errorf("%s authorization = %q, want operation snapshot Bearer key-a", request.URL, auth)
		}
	}
	if got := providerCalls.Load(); got != 1 {
		t.Errorf("provider calls during nested operation = %d, want 1", got)
	}
}

func TestDynamicAPIKey_ConcurrentRotation(t *testing.T) {
	const workers = 8
	const operations = 20
	steps := make([]httpfixture.Step, workers*operations)
	for i := range steps {
		steps[i].Response = testResponse(`{"results":[]}`)
	}
	script := httpfixture.NewScriptedTransport(steps...)
	var key atomic.Value
	key.Store("key-a")
	var providerCalls atomic.Int64
	client := testClient(func() string {
		providerCalls.Add(1)
		return key.Load().(string)
	}, script)

	done := make(chan struct{})
	var rotate sync.WaitGroup
	rotate.Add(1)
	go func() {
		defer rotate.Done()
		for i := 0; ; i++ {
			select {
			case <-done:
				return
			default:
				if i%2 == 0 {
					key.Store("key-a")
				} else {
					key.Store("key-b")
				}
				runtime.Gosched()
			}
		}
	}()

	var calls sync.WaitGroup
	errs := make(chan error, workers*operations)
	for range workers {
		calls.Add(1)
		go func() {
			defer calls.Done()
			for range operations {
				_, err := client.Search(context.Background(), "matrix", 1)
				errs <- err
			}
		}()
	}
	calls.Wait()
	close(done)
	rotate.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("concurrent Search: %v", err)
		}
	}

	want := int64(workers * operations)
	if got := providerCalls.Load(); got != want {
		t.Errorf("provider calls = %d, want exactly one per operation (%d)", got, want)
	}
	requests := script.Requests()
	if len(requests) != int(want) {
		t.Fatalf("requests = %d, want %d", len(requests), want)
	}
	for _, request := range requests {
		auth := request.Header.Get("Authorization")
		if auth != "Bearer key-a" && auth != "Bearer key-b" {
			t.Errorf("partial or unexpected credential under rotation: %q", auth)
		}
	}
}
