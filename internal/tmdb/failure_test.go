package tmdb

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
)

func TestFailureClass(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"nil", nil, ""},
		{"no key", ErrAPIKeyRequired, FailureNoKey},
		{"canceled", fmt.Errorf("get: %w", context.Canceled), FailureCanceled},
		{"deadline", context.DeadlineExceeded, FailureTimeout},
		{"429", &StatusError{Status: http.StatusTooManyRequests}, FailureRateLimited},
		{"401", &StatusError{Status: http.StatusUnauthorized}, FailureUnauthorized},
		{"403", &StatusError{Status: http.StatusForbidden}, FailureUnauthorized},
		{"404", &StatusError{Status: http.StatusNotFound}, FailureClientError},
		{"503", fmt.Errorf("wrapped: %w", &StatusError{Status: 503}), FailureServerError},
		{"decode", &decodeError{err: errors.New("bad json")}, FailureDecode},
		{"other", errors.New("boom"), FailureOther},
	} {
		if got := FailureClass(tc.err); got != tc.want {
			t.Errorf("%s: FailureClass = %q, want %q", tc.name, got, tc.want)
		}
	}
}
