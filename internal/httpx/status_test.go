package httpx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/metrics"
)

func TestClassifyStatus(t *testing.T) {
	cases := map[int]Class{
		200: ClassNone, 204: ClassNone,
		401: ClassAuth, 403: ClassAuth,
		404: ClassNotFound, 410: ClassNotFound,
		408: ClassTransient, 429: ClassTransient, 500: ClassTransient,
		502: ClassTransient, 503: ClassTransient, 504: ClassTransient,
		400: ClassPermanent, 409: ClassPermanent, 422: ClassPermanent,
		501: ClassPermanent, 505: ClassPermanent,
	}
	for code, want := range cases {
		if got := ClassifyStatus(code); got != want {
			t.Errorf("ClassifyStatus(%d) = %v, want %v", code, got, want)
		}
	}
}

// ClassifyStatus is the one status ladder: "try again later" is 408, 425, 429 and every 5xx
// except the two that name a permanent mismatch (501 Not Implemented, 505 HTTP Version Not
// Supported). That keeps a CDN's 52x, 507 and 511 transient.
func TestClassifyStatusLadder(t *testing.T) {
	for code := 100; code <= 599; code++ {
		want := ClassPermanent
		switch {
		case code < 300:
			want = ClassNone
		case code == 401 || code == 403:
			want = ClassAuth
		case code == 404 || code == 410:
			want = ClassNotFound
		case code == 408 || code == 425 || code == 429:
			want = ClassTransient
		case code >= 500 && code != 501 && code != 505:
			want = ClassTransient
		}
		if got := ClassifyStatus(code); got != want {
			t.Errorf("ClassifyStatus(%d) = %v, want %v", code, got, want)
		}
	}
}

// A transient status without its own metric reason is retried and counted as other_status.
func TestRetryReasonForUnlistedTransientStatus(t *testing.T) {
	for _, code := range []int{425, 507, 511, 522} {
		reason, retried := retryReason(context.Background(), &http.Response{StatusCode: code}, nil)
		if !retried || reason != metrics.OutboundRetryStatusOther {
			t.Errorf("status %d: reason %v retried %v, want other_status retried", code, reason, retried)
		}
	}
}

// The retry loop and Classify must agree on which statuses are worth another
// attempt; a drift would make callers and the transport disagree.
func TestRetryDecisionFollowsClassifyStatus(t *testing.T) {
	for code := 100; code <= 599; code++ {
		_, retried := retryReason(context.Background(), &http.Response{StatusCode: code}, nil)
		if want := ClassifyStatus(code) == ClassTransient; retried != want {
			t.Errorf("status %d: retried = %v, ClassifyStatus transient = %v", code, retried, want)
		}
	}
}

func TestClassifyErrors(t *testing.T) {
	status := &StatusError{Method: "GET", Path: "/x", Status: http.StatusServiceUnavailable}
	cases := []struct {
		name string
		err  error
		want Class
	}{
		{"nil", nil, ClassNone},
		{"status", status, ClassTransient},
		{"wrapped status", fmt.Errorf("fetch: %w", &StatusError{Status: 401}), ClassAuth},
		{"not found", &StatusError{Status: 404}, ClassNotFound},
		{"canceled", fmt.Errorf("do: %w", context.Canceled), ClassCanceled},
		{"net error", &net.OpError{Op: "dial", Err: errors.New("refused")}, ClassTransient},
		{"deadline", fmt.Errorf("do: %w", context.DeadlineExceeded), ClassTransient},
		{"unexpected eof", io.ErrUnexpectedEOF, ClassTransient},
		{"other", errors.New("boom"), ClassUnknown},
	}
	for _, tc := range cases {
		if got := Classify(tc.err); got != tc.want {
			t.Errorf("%s: Classify = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestCheckStatus(t *testing.T) {
	req, _ := http.NewRequest(http.MethodPost, "http://svc.test/api/thing?secret=1", nil)
	body := func(s string) io.ReadCloser { return io.NopCloser(strings.NewReader(s)) }

	if err := CheckStatus(&http.Response{StatusCode: 204, Body: body(""), Request: req}); err != nil {
		t.Fatalf("2xx: %v", err)
	}

	err := CheckStatus(&http.Response{StatusCode: 502, Body: body("  upstream down \n"), Request: req})
	var se *StatusError
	if !errors.As(err, &se) {
		t.Fatalf("err = %v, want *StatusError", err)
	}
	if se.Status != 502 || se.Method != "POST" || se.Path != "/api/thing" || se.Body != "upstream down" {
		t.Errorf("StatusError = %+v", se)
	}
	if got, want := err.Error(), "POST /api/thing: status 502: upstream down"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
	if strings.Contains(err.Error(), "secret") {
		t.Error("query string leaked into the error")
	}
}

func TestCheckStatusCapsBody(t *testing.T) {
	big := strings.Repeat("x", 4096)
	err := CheckStatus(&http.Response{StatusCode: 500, Body: io.NopCloser(strings.NewReader(big))})
	var se *StatusError
	if !errors.As(err, &se) || len(se.Body) != statusBodyLimit {
		t.Fatalf("body len = %d, want %d", len(se.Body), statusBodyLimit)
	}
}

func TestStatusErrorWithoutBody(t *testing.T) {
	if got, want := (&StatusError{Method: "GET", Path: "/p", Status: 404}).Error(), "GET /p: status 404"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}
