package httpcore

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// AmbiguousDatesError is the 422 for a policy request carrying both `era` and `dates`
// (schedule.ErrEraAndDates): era is a shortcut for dates, so the request does not say
// which one it means.
func AmbiguousDatesError(err error) huma.StatusError {
	return ProblemWithCause(http.StatusUnprocessableEntity, "Era and dates both set",
		"Send either an era or programming dates, not both. An era is a shortcut for the same year range on movie release, series premiere and episode airing.", err)
}
