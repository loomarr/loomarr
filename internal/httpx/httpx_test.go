package httpx

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/loomarr/loomarr/internal/testkit/httpfixture"
)

type contextBody struct{ ctx context.Context }

func (b *contextBody) Read([]byte) (int, error) {
	<-b.ctx.Done()
	return 0, b.ctx.Err()
}

func (*contextBody) Close() error { return nil }

func TestNew_WholeRequestTimeoutAbortsSlowBody(t *testing.T) {
	transport := httpfixture.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: &contextBody{ctx: req.Context()}}, nil
	})
	c := newClient(20*time.Millisecond, transport, nil)
	resp, err := c.Get("http://service/stream")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if _, err := io.ReadAll(resp.Body); err == nil {
		t.Fatal("expected whole-request timeout to abort the response body")
	}
}

func TestNewStreaming_HasNoWholeRequestTimeout(t *testing.T) {
	transport := httpfixture.RoundTripperFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("one\ntwo\nthree\n")),
		}, nil
	})
	c := newStreamingClient(transport)
	if c.Timeout != 0 {
		t.Fatalf("NewStreaming timeout = %v, want zero", c.Timeout)
	}
	resp, err := c.Get("http://service/stream")
	if err != nil {
		t.Fatalf("streaming GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read stream: %v", err)
	}
	if string(body) != "one\ntwo\nthree\n" {
		t.Fatalf("body = %q", body)
	}
}

func TestNewNamedIsTransparent(t *testing.T) {
	transport := httpfixture.RoundTripperFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader("ok")),
		}, nil
	})
	c := newNamedClient("test-target", 5*time.Second, transport)
	if c.Timeout != 5*time.Second {
		t.Fatalf("NewNamed timeout = %v, want 5s", c.Timeout)
	}
	resp, err := c.Get("http://service/")
	if err != nil {
		t.Fatalf("instrumented GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "ok" {
		t.Fatalf("body = %q, want ok", body)
	}
}

func TestRedirectPolicy(t *testing.T) {
	newRedirectClient := func() (*http.Client, *int) {
		calls := 0
		transport := httpfixture.RoundTripperFunc(func(req *http.Request) (*http.Response, error) {
			calls++
			if calls == 1 {
				return &http.Response{
					StatusCode: http.StatusFound,
					Header:     http.Header{"Location": []string{"http://service/final"}},
					Body:       http.NoBody,
					Request:    req,
				}, nil
			}
			return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: req}, nil
		})
		return newClient(time.Second, transport, nil), &calls
	}

	get, getCalls := newRedirectClient()
	resp, err := get.Get("http://service/start")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || *getCalls != 2 {
		t.Fatalf("GET redirect status=%d calls=%d, want 200 and 2", resp.StatusCode, *getCalls)
	}

	post, postCalls := newRedirectClient()
	req, _ := http.NewRequest(http.MethodPost, "http://service/start", nil)
	resp, err = post.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusFound || *postCalls != 1 {
		t.Fatalf("POST redirect status=%d calls=%d, want 302 and 1", resp.StatusCode, *postCalls)
	}
}
