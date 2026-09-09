package playoutcertfixture

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/testkit/httpfixture"
)

func TestPostEventCorruptClientCorruptsOnlyExplicitHeldStreamsAfterOverload(t *testing.T) {
	const valid = "valid-media"
	held := []string{"held-one", "held-two", "held-three", "held-four"}
	client := PostEventCorruptClient(httpfixture.RoundTripperFunc(func(request *http.Request) (*http.Response, error) {
		status := http.StatusOK
		if strings.HasSuffix(request.URL.Path, "/excess") {
			status = http.StatusServiceUnavailable
		}
		return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(valid))}, nil
	}), held)

	// Raw requests unrelated to the held cohort may occur before it and must not
	// influence selection. The held responses remain valid until overload closes
	// the shared gate, just as their initial media validation does in the runner.
	for _, stream := range []string{"unrelated-before-one", "unrelated-before-two"} {
		response := getRaw(t, client, stream)
		if got := readAll(t, response); got != valid {
			t.Fatalf("unrelated response before overload = %q", got)
		}
	}
	heldResponses := make([]*http.Response, 0, len(held))
	for _, stream := range held {
		response := getRaw(t, client, stream)
		buffer := make([]byte, 5)
		if count, err := response.Body.Read(buffer); err != nil || string(buffer[:count]) != valid[:count] {
			t.Fatalf("held response before overload = %q, %v", buffer[:count], err)
		}
		heldResponses = append(heldResponses, response)
	}

	overload := getRaw(t, client, "excess")
	_ = overload.Body.Close()

	for _, response := range heldResponses {
		buffer := make([]byte, 188)
		count, err := response.Body.Read(buffer)
		if err != nil || count == 0 || strings.Trim(string(buffer[:count]), "\x7f") != "" {
			t.Fatalf("held response after overload was not corrupted: bytes=%d err=%v", count, err)
		}
		_ = response.Body.Close()
	}
	after := getRaw(t, client, "unrelated-after")
	if got := readAll(t, after); got != valid {
		t.Fatalf("unrelated response after overload = %q", got)
	}
}

func getRaw(t *testing.T, client *http.Client, stream string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, "http://fixture.invalid/v1/playout/stream/"+stream, nil)
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func readAll(t *testing.T, response *http.Response) string {
	t.Helper()
	bytes, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if err := response.Body.Close(); err != nil {
		t.Fatal(err)
	}
	return string(bytes)
}
