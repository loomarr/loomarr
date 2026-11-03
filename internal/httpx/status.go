package httpx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
)

// statusBodyLimit caps the response text a StatusError keeps. A service's own
// explanation of a refusal is what an operator needs; more than this is noise.
const statusBodyLimit = 512

// StatusError is a non-2xx answer from a remote service. It keeps the status
// machine-readable so callers branch on Classify rather than on the message.
type StatusError struct {
	Method string
	Path   string // request path only: a query string can carry a credential
	Status int
	Body   string // trimmed, capped at 512 bytes
}

func (e *StatusError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("%s %s: status %d", e.Method, e.Path, e.Status)
	}
	return fmt.Sprintf("%s %s: status %d: %s", e.Method, e.Path, e.Status, e.Body)
}

// CheckStatus returns nil for a 2xx response and a *StatusError otherwise. It
// reads at most 512 bytes of the body and leaves closing it to the caller.
func CheckStatus(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	se := &StatusError{Status: resp.StatusCode}
	if resp.Request != nil {
		se.Method = resp.Request.Method
		if resp.Request.URL != nil {
			se.Path = resp.Request.URL.Path
		}
	}
	if resp.Body != nil {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, statusBodyLimit))
		se.Body = strings.TrimSpace(string(snippet))
	}
	return se
}

// Class says what a failed outbound call means for the caller's next move.
type Class int

const (
	// ClassNone is a success (or a nil error).
	ClassNone Class = iota
	// ClassTransient: the service is busy or unreachable; a later attempt may succeed.
	// The retry transport retries exactly these statuses for idempotent GETs.
	ClassTransient
	// ClassAuth: the service rejected our credentials (401, 403).
	ClassAuth
	// ClassNotFound: the thing is not there (404, 410).
	ClassNotFound
	// ClassPermanent: the request will fail the same way again (other 4xx, 501, 505, redirects).
	ClassPermanent
	// ClassCanceled: the caller's context ended the call.
	ClassCanceled
	// ClassUnknown: an error that carries no HTTP or network signal.
	ClassUnknown
)

func (c Class) String() string {
	switch c {
	case ClassNone:
		return "none"
	case ClassTransient:
		return "transient"
	case ClassAuth:
		return "auth"
	case ClassNotFound:
		return "not_found"
	case ClassPermanent:
		return "permanent"
	case ClassCanceled:
		return "canceled"
	default:
		return "unknown"
	}
}

// ClassifyStatus classifies an HTTP status code. It is the single definition of
// "worth retrying" shared by the retry transport and every caller.
func ClassifyStatus(code int) Class {
	switch {
	case code < 300:
		return ClassNone
	case code == http.StatusUnauthorized || code == http.StatusForbidden:
		return ClassAuth
	case code == http.StatusNotFound || code == http.StatusGone:
		return ClassNotFound
	case isRetryableStatus(code):
		return ClassTransient
	default:
		return ClassPermanent
	}
}

func isRetryableStatus(code int) bool {
	switch code {
	case http.StatusRequestTimeout, http.StatusTooManyRequests,
		http.StatusInternalServerError, http.StatusBadGateway,
		http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}
	return false
}

// Classify classifies an error from an outbound call: a *StatusError (anywhere in
// the chain) by its status, a cancelled context, a deadline or network failure as
// transient, and anything else as unknown.
func Classify(err error) Class {
	if err == nil {
		return ClassNone
	}
	var se *StatusError
	if errors.As(err, &se) {
		return ClassifyStatus(se.Status)
	}
	if errors.Is(err, context.Canceled) {
		return ClassCanceled
	}
	var netErr net.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, io.EOF) || errors.As(err, &netErr) {
		return ClassTransient
	}
	return ClassUnknown
}
