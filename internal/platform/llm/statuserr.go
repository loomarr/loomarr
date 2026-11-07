package llm

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/loomarr/loomarr/internal/platform/httpx"
)

// describedStatusError is a non-2xx reply whose operator-facing text is fixed by the caller.
// It wraps the *httpx.StatusError, so httpx.Classify and errors.As see the status, while the
// provider's own body stays out of the text (and so out of any persisted cause).
type describedStatusError struct {
	text string
	err  *httpx.StatusError
}

func (e *describedStatusError) Error() string { return e.text }
func (e *describedStatusError) Unwrap() error { return e.err }

// checkStatus returns nil for a 2xx reply, otherwise an httpx.StatusError whose text is
// "<op>: status N" (the provider's body is kept on the error, not in the text).
func checkStatus(resp *http.Response, op string) error {
	err := httpx.CheckStatus(resp)
	if err == nil {
		return nil
	}
	var se *httpx.StatusError
	if !errors.As(err, &se) {
		return err
	}
	return &describedStatusError{text: fmt.Sprintf("%s: status %d", op, se.Status), err: se}
}

// checkStatusWithBody is checkStatus for a call whose text carries the provider's reason:
// "<op>: status N: <body>". SafeCause reduces it to the provider's message field.
func checkStatusWithBody(resp *http.Response, op string) error {
	err := httpx.CheckStatus(resp)
	if err == nil {
		return nil
	}
	var se *httpx.StatusError
	if !errors.As(err, &se) {
		return err
	}
	return &callStatusError{op: op, err: se}
}
