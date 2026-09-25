package tmdb

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
)

// StatusError is a TMDB response outside 2xx that the caller did not treat as "not found".
// It exists so a failure can be CLASSIFIED (FailureClass) instead of being matched by message.
type StatusError struct {
	Path   string
	Status int
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("tmdb GET %s: status %d", e.Path, e.Status)
}

// decodeError marks a 2xx body that did not parse.
type decodeError struct{ err error }

func (e *decodeError) Error() string { return e.err.Error() }
func (e *decodeError) Unwrap() error { return e.err }

// Failure classes are a closed set so they are safe as a log field or metric label.
const (
	FailureNoKey        = "no_key"
	FailureCanceled     = "canceled"
	FailureTimeout      = "timeout"
	FailureNetwork      = "network"
	FailureRateLimited  = "rate_limited"
	FailureUnauthorized = "unauthorized"
	FailureClientError  = "client_error"
	FailureServerError  = "server_error"
	FailureDecode       = "decode"
	FailureOther        = "other"
)

// FailureClass buckets a TMDB error so an operator can tell a bad key from a rate limit from a
// dropped connection. A caller that swallows failures (best-effort artwork) must still be able to
// say WHICH kind it is swallowing.
func FailureClass(err error) string {
	var status *StatusError
	var dec *decodeError
	var netErr net.Error
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrAPIKeyRequired):
		return FailureNoKey
	case errors.Is(err, context.Canceled):
		return FailureCanceled
	case errors.Is(err, context.DeadlineExceeded):
		return FailureTimeout
	case errors.As(err, &status):
		switch {
		case status.Status == http.StatusTooManyRequests:
			return FailureRateLimited
		case status.Status == http.StatusUnauthorized || status.Status == http.StatusForbidden:
			return FailureUnauthorized
		case status.Status >= 500:
			return FailureServerError
		default:
			return FailureClientError
		}
	case errors.As(err, &dec):
		return FailureDecode
	case errors.As(err, &netErr):
		if netErr.Timeout() {
			return FailureTimeout
		}
		return FailureNetwork
	}
	return FailureOther
}
