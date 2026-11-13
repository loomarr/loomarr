package channels

import (
	"context"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/loomarr/loomarr/internal/api/httpcore"
	"github.com/loomarr/loomarr/internal/platform/settings"
)

// GET /v1/household — the household facts every page may need to tell things the household's way.
// Today that is its timezone: the Guide tells times in it, and a time shown anywhere else (a
// Library retry, a request's progress) reads the same only if it uses it too.
func (s *Handlers) registerHousehold(api huma.API) {
	huma.Register(api, httpcore.WithRole(huma.Operation{
		OperationID: "household", Method: http.MethodGet, Path: "/v1/household",
		Summary:     "The household's timezone",
		Description: "Any signed-in person. The IANA timezone the Guide tells times in, or empty when none is set (times then read in the viewer's own zone).",
		Tags:        []string{"channels"},
	}, httpcore.RoleMember), s.household)
}

type householdOutput struct {
	Body struct {
		TimeZone string `json:"timeZone" doc:"IANA timezone the household tells times in; empty when unset"`
	}
}

func (s *Handlers) household(context.Context, *struct{}) (*householdOutput, error) {
	out := &householdOutput{}
	if s.liveSettings != nil {
		out.Body.TimeZone = strings.TrimSpace(settings.GuideTimezone.Get(s.liveSettings))
	}
	return out, nil
}
