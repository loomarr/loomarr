package api

import (
	"errors"
	"net/http"
	"testing"
)

// Every status the single-approve handlers raise, classified the way a bulk result reports it
// (Refs #1876). 501 is a 5xx a retry can never clear: the install can't do it at all.
func TestBulkFailure_RetriesOnlyTransientServerFaults(t *testing.T) {
	cases := []struct {
		name      string
		err       error
		status    int
		retryable bool
	}{
		{"not found", errNotFound("Suggestion not found", "Gone."), http.StatusNotFound, false},
		{"already handled", errConflict("Already handled", "Decided."), http.StatusConflict, false},
		{"invalid", errUnprocessable("Choose at least one title", "Empty."), http.StatusUnprocessableEntity, false},
		{"not set up", errNotImplemented("Filler isn't set up", "No dir."), http.StatusNotImplemented, false},
		{"store fault", apiErrWithCause(http.StatusInternalServerError, "Read failed", "Try again.", errors.New("disk")), http.StatusInternalServerError, true},
		{"upstream fault", apiErrWithCause(http.StatusBadGateway, "Couldn't start the pull", "Try again.", errors.New("exit 1")), http.StatusBadGateway, true},
		{"plain error", errors.New("approve: approval coordinator is not configured"), http.StatusInternalServerError, true},
	}
	for _, c := range cases {
		status, retryable := bulkFailure(c.err)
		if status != c.status || retryable != c.retryable {
			t.Errorf("%s: bulkFailure = (%d, %v), want (%d, %v)", c.name, status, retryable, c.status, c.retryable)
		}
	}
}
