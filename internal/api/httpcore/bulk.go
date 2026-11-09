package httpcore

import (
	"errors"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// BulkFailure classifies one id's approval error for any bulk approve endpoint (Refs #1876). The
// SERVER decides retryability from the status the single-approve handler raised, so no client
// keeps its own status rule or guesses from the message text.
//
// A 5xx is a server or upstream fault that a retry may clear (a store read, a download that
// couldn't start), except 501: this install can't do it at all. Every 4xx is the item's own
// state, which a retry repeats. An error carrying no status is an unexpected fault, so it is a
// 500, as huma would answer it. It counts as retryable even though some such faults, like a
// missing approver, persist: the server can't tell them apart, and a refused retry costs less
// than a stuck item with nothing to press.
func BulkFailure(err error) (status int, retryable bool) {
	status = StatusFromError(err, http.StatusInternalServerError)
	return status, status >= http.StatusInternalServerError && status != http.StatusNotImplemented
}

// ErrMessage renders a handler error for a per-id result line, preferring the human-facing title
// a huma StatusError carries ("Already handled") over the raw error string.
func ErrMessage(err error) string {
	var se huma.StatusError
	if errors.As(err, &se) {
		return se.Error()
	}
	return err.Error()
}
