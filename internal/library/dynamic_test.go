package library

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/mantonx/loomarr/internal/testkit/httpfixture"
)

func dynamicResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func dynamicClient(current ConnectionSource, transport http.RoundTripper) *Client {
	return newDynamicWithHTTP(current, "device-1", &http.Client{Transport: transport})
}

func TestConnectionGenerationUsesFullValidatedConnection(t *testing.T) {
	base := Connection{Flavor: Emby, BaseURL: "  https://emby.invalid/  ", Token: "token-a"}
	normalized, err := base.Validate()
	if err != nil {
		t.Fatal(err)
	}
	if normalized.BaseURL != "https://emby.invalid" {
		t.Fatalf("normalized BaseURL = %q, want https://emby.invalid", normalized.BaseURL)
	}

	generation, err := base.Generation()
	if err != nil {
		t.Fatal(err)
	}
	equivalent, err := (Connection{
		Flavor: Emby, BaseURL: "https://emby.invalid", Token: "token-a",
	}).Generation()
	if err != nil {
		t.Fatal(err)
	}
	if generation != equivalent {
		t.Fatal("normalized URL spellings produced different generations")
	}

	for _, test := range []struct {
		name       string
		connection Connection
	}{
		{name: "flavor", connection: Connection{Flavor: Jellyfin, BaseURL: "https://emby.invalid", Token: "token-a"}},
		{name: "URL", connection: Connection{Flavor: Emby, BaseURL: "https://other.invalid", Token: "token-a"}},
		{name: "token", connection: Connection{Flavor: Emby, BaseURL: "https://emby.invalid", Token: "token-b"}},
	} {
		other, err := test.connection.Generation()
		if err != nil {
			t.Fatal(err)
		}
		if generation == other {
			t.Fatalf("changed %s reused the original generation", test.name)
		}
	}
}

func TestConnectionGenerationRejectsIncompleteConnection(t *testing.T) {
	for _, test := range []struct {
		name       string
		connection Connection
		want       error
	}{
		{name: "flavor", connection: Connection{BaseURL: "https://emby.invalid", Token: "token"}, want: ErrConnectionFlavorRequired},
		{name: "URL", connection: Connection{Flavor: Emby, Token: "token"}, want: ErrConnectionURLRequired},
		{name: "invalid URL", connection: Connection{Flavor: Emby, BaseURL: "/emby", Token: "token"}, want: ErrConnectionURLRequired},
		{name: "token", connection: Connection{Flavor: Emby, BaseURL: "https://emby.invalid"}, want: ErrConnectionTokenRequired},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := test.connection.Generation()
			if !errors.Is(err, test.want) || !errors.Is(err, ErrConnectionRequired) {
				t.Fatalf("Generation error = %v, want %v wrapping ErrConnectionRequired", err, test.want)
			}
		})
	}
}

func TestDynamicConnection_RotatesFlavorURLAndTokenBetweenOperations(t *testing.T) {
	script := httpfixture.NewScriptedTransport(
		httpfixture.Step{Response: dynamicResponse(http.StatusOK, `[]`)},
		httpfixture.Step{Response: dynamicResponse(http.StatusOK, `[]`)},
	)
	var connection atomic.Value
	connection.Store(Connection{Flavor: Emby, BaseURL: "https://emby-a.invalid/", Token: "token-a"})
	var providerCalls atomic.Int64
	client := dynamicClient(func() Connection {
		providerCalls.Add(1)
		return connection.Load().(Connection)
	}, script)

	if _, err := client.ListUsers(context.Background()); err != nil {
		t.Fatal(err)
	}
	connection.Store(Connection{Flavor: Jellyfin, BaseURL: "https://jellyfin-b.invalid", Token: "token-b"})
	if _, err := client.ListUsers(context.Background()); err != nil {
		t.Fatal(err)
	}

	requests := script.Requests()
	if len(requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(requests))
	}
	assertEmbyConnection(t, requests[0], "emby-a.invalid", "token-a")
	assertJellyfinConnection(t, requests[1], "jellyfin-b.invalid", "token-b")
	if got := providerCalls.Load(); got != 2 {
		t.Errorf("provider calls = %d, want one per operation (2)", got)
	}
}

func TestDynamicConnection_IncompleteFailsClosedWithoutHTTP(t *testing.T) {
	tests := []struct {
		name       string
		connection Connection
	}{
		{name: "missing flavor", connection: Connection{BaseURL: "https://emby.invalid", Token: "token"}},
		{name: "invalid flavor", connection: Connection{Flavor: Flavor("plex"), BaseURL: "https://emby.invalid", Token: "token"}},
		{name: "missing URL", connection: Connection{Flavor: Emby, Token: "token"}},
		{name: "relative URL", connection: Connection{Flavor: Emby, BaseURL: "/media", Token: "token"}},
		{name: "unsupported URL scheme", connection: Connection{Flavor: Emby, BaseURL: "ftp://emby.invalid", Token: "token"}},
		{name: "missing token", connection: Connection{Flavor: Jellyfin, BaseURL: "https://jellyfin.invalid"}},
		{name: "blank token", connection: Connection{Flavor: Jellyfin, BaseURL: "https://jellyfin.invalid", Token: "  "}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			script := httpfixture.NewScriptedTransport()
			client := dynamicClient(func() Connection { return test.connection }, script)
			if _, err := client.ListUsers(context.Background()); !errors.Is(err, ErrConnectionRequired) {
				t.Errorf("ListUsers error = %v, want ErrConnectionRequired", err)
			}
			if _, err := client.ItemMetadataByID(context.Background(), nil); !errors.Is(err, ErrConnectionRequired) {
				t.Errorf("ItemMetadataByID error = %v, want ErrConnectionRequired", err)
			}
			if got := client.StreamURL("item-1"); got != "" {
				t.Errorf("StreamURL = %q, want empty", got)
			}
			if got := script.Calls(); got != 0 {
				t.Errorf("incomplete connection made %d HTTP requests, want 0", got)
			}
		})
	}
}

func TestDynamicConnection_SnapshotsRefreshGuideAcrossRotation(t *testing.T) {
	script := httpfixture.NewScriptedTransport(
		httpfixture.Step{Response: dynamicResponse(http.StatusOK, `[{"Id":"task-a","Key":"RefreshGuide"}]`)},
		httpfixture.Step{Response: dynamicResponse(http.StatusNoContent, "")},
		httpfixture.Step{Response: dynamicResponse(http.StatusOK, `[]`)},
	)
	var connection atomic.Value
	connection.Store(Connection{Flavor: Emby, BaseURL: "https://emby-a.invalid", Token: "token-a"})
	var providerCalls atomic.Int64
	transport := httpfixture.RoundTripperFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path == "/ScheduledTasks" {
			connection.Store(Connection{Flavor: Jellyfin, BaseURL: "https://jellyfin-b.invalid", Token: "token-b"})
		}
		return script.RoundTrip(request)
	})
	client := dynamicClient(func() Connection {
		providerCalls.Add(1)
		return connection.Load().(Connection)
	}, transport)

	if err := client.RefreshGuide(context.Background()); err != nil {
		t.Fatal(err)
	}
	requests := script.Requests()
	if len(requests) != 2 {
		t.Fatalf("RefreshGuide requests = %d, want lookup + run", len(requests))
	}
	for _, request := range requests {
		assertEmbyConnection(t, request, "emby-a.invalid", "token-a")
	}
	if got := providerCalls.Load(); got != 1 {
		t.Errorf("provider calls during RefreshGuide = %d, want 1", got)
	}

	if _, err := client.ListUsers(context.Background()); err != nil {
		t.Fatal(err)
	}
	requests = script.Requests()
	assertJellyfinConnection(t, requests[2], "jellyfin-b.invalid", "token-b")
	if got := providerCalls.Load(); got != 2 {
		t.Errorf("provider calls after next operation = %d, want 2", got)
	}
}

func TestDynamicConnection_SnapshotsBatchedMetadataAcrossRotation(t *testing.T) {
	script := httpfixture.NewScriptedTransport(
		httpfixture.Step{Response: dynamicResponse(http.StatusOK, `{"Items":[]}`)},
		httpfixture.Step{Response: dynamicResponse(http.StatusOK, `{"Items":[]}`)},
	)
	var connection atomic.Value
	connection.Store(Connection{Flavor: Emby, BaseURL: "https://emby-a.invalid", Token: "token-a"})
	var providerCalls atomic.Int64
	var requestCount atomic.Int64
	transport := httpfixture.RoundTripperFunc(func(request *http.Request) (*http.Response, error) {
		if requestCount.Add(1) == 1 {
			connection.Store(Connection{Flavor: Jellyfin, BaseURL: "https://jellyfin-b.invalid", Token: "token-b"})
		}
		return script.RoundTrip(request)
	})
	client := dynamicClient(func() Connection {
		providerCalls.Add(1)
		return connection.Load().(Connection)
	}, transport)
	ids := make([]string, maxIDsPerRequest+1)
	for i := range ids {
		ids[i] = fmt.Sprintf("item-%03d", i)
	}

	if _, err := client.ItemMetadataByID(context.Background(), ids); err != nil {
		t.Fatal(err)
	}
	requests := script.Requests()
	if len(requests) != 2 {
		t.Fatalf("metadata requests = %d, want 2 batches", len(requests))
	}
	for _, request := range requests {
		assertEmbyConnection(t, request, "emby-a.invalid", "token-a")
	}
	if got := providerCalls.Load(); got != 1 {
		t.Errorf("provider calls during batched metadata = %d, want 1", got)
	}
}

func TestDynamicConnection_SnapshotPinsCollectionIndexAcrossRotation(t *testing.T) {
	script := httpfixture.NewScriptedTransport(
		httpfixture.Step{Response: dynamicResponse(http.StatusOK, `{"Items":[{"Id":"collection-a","Name":"A"}]}`)},
		httpfixture.Step{Response: dynamicResponse(http.StatusOK, `{"Items":[]}`)},
		httpfixture.Step{Response: dynamicResponse(http.StatusOK, `[]`)},
	)
	var connection atomic.Value
	connection.Store(Connection{Flavor: Emby, BaseURL: "https://emby-a.invalid", Token: "token-a"})
	var providerCalls atomic.Int64
	transport := httpfixture.RoundTripperFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Query().Get("IncludeItemTypes") == "BoxSet" {
			connection.Store(Connection{Flavor: Jellyfin, BaseURL: "https://jellyfin-b.invalid", Token: "token-b"})
		}
		return script.RoundTrip(request)
	})
	client := dynamicClient(func() Connection {
		providerCalls.Add(1)
		return connection.Load().(Connection)
	}, transport)

	operation := client.Snapshot()
	reported := operation.Connection()
	reported.Token = "caller-mutation"
	collections, err := operation.Collections(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(collections) != 1 {
		t.Fatalf("collections = %d, want 1", len(collections))
	}
	if _, err := operation.CollectionMembers(context.Background(), collections[0].ID); err != nil {
		t.Fatal(err)
	}
	requests := script.Requests()
	if len(requests) != 2 {
		t.Fatalf("collection-index requests = %d, want collections + members", len(requests))
	}
	for _, request := range requests {
		assertEmbyConnection(t, request, "emby-a.invalid", "token-a")
	}
	if got := providerCalls.Load(); got != 1 {
		t.Errorf("provider calls during collection index = %d, want 1", got)
	}

	if _, err := client.ListUsers(context.Background()); err != nil {
		t.Fatal(err)
	}
	requests = script.Requests()
	assertJellyfinConnection(t, requests[2], "jellyfin-b.invalid", "token-b")
}

func TestDynamicConnection_SnapshotPinsFillerResolutionAcrossRotation(t *testing.T) {
	script := httpfixture.NewScriptedTransport(
		httpfixture.Step{Response: dynamicResponse(http.StatusOK, `[{"Name":"Commercials","ItemId":"library-a"}]`)},
		httpfixture.Step{Response: dynamicResponse(http.StatusOK, `{"Items":[]}`)},
		httpfixture.Step{Response: dynamicResponse(http.StatusOK, `[]`)},
	)
	var connection atomic.Value
	connection.Store(Connection{Flavor: Emby, BaseURL: "https://emby-a.invalid", Token: "token-a"})
	var providerCalls atomic.Int64
	transport := httpfixture.RoundTripperFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path == "/Library/VirtualFolders" {
			connection.Store(Connection{Flavor: Jellyfin, BaseURL: "https://jellyfin-b.invalid", Token: "token-b"})
		}
		return script.RoundTrip(request)
	})
	client := dynamicClient(func() Connection {
		providerCalls.Add(1)
		return connection.Load().(Connection)
	}, transport)

	operation := client.Snapshot()
	id, err := operation.LibraryIDByName(context.Background(), "Commercials")
	if err != nil {
		t.Fatal(err)
	}
	if id != "library-a" {
		t.Fatalf("library id = %q, want library-a", id)
	}
	if _, err := operation.ListFillerClips(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	requests := script.Requests()
	if len(requests) != 2 {
		t.Fatalf("filler requests = %d, want library lookup + clip list", len(requests))
	}
	for _, request := range requests {
		assertEmbyConnection(t, request, "emby-a.invalid", "token-a")
	}
	if got := providerCalls.Load(); got != 1 {
		t.Errorf("provider calls during filler resolution = %d, want 1", got)
	}

	if _, err := client.ListUsers(context.Background()); err != nil {
		t.Fatal(err)
	}
	requests = script.Requests()
	assertJellyfinConnection(t, requests[2], "jellyfin-b.invalid", "token-b")
}

func assertEmbyConnection(t *testing.T, request httpfixture.Request, host, token string) {
	t.Helper()
	parsed, err := url.Parse(request.URL)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Host != host {
		t.Errorf("request host = %q, want %q", parsed.Host, host)
	}
	if got := request.Header.Get("X-Emby-Token"); got != token {
		t.Errorf("X-Emby-Token = %q, want %q", got, token)
	}
	if got := request.Header.Get("Authorization"); got != "" {
		t.Errorf("Authorization = %q, want empty for Emby", got)
	}
}

func assertJellyfinConnection(t *testing.T, request httpfixture.Request, host, token string) {
	t.Helper()
	parsed, err := url.Parse(request.URL)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Host != host {
		t.Errorf("request host = %q, want %q", parsed.Host, host)
	}
	if got := request.Header.Get("X-Emby-Token"); got != "" {
		t.Errorf("X-Emby-Token = %q, want empty for Jellyfin", got)
	}
	if got := request.Header.Get("Authorization"); !strings.Contains(got, `Token="`+token+`"`) {
		t.Errorf("Authorization = %q, want Jellyfin token %q", got, token)
	}
}
