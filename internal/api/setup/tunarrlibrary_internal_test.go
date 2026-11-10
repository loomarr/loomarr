package setup

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/api/httpcore"
)

// failingTunarr stands in for a Tunarr client with no address: every call is a transport error.
type failingTunarr struct{ calls int }

func (f *failingTunarr) Connect(context.Context) (string, int, error) { return "", 0, nil }

func (f *failingTunarr) LibrariesReady(context.Context) (bool, error) {
	f.calls++
	return false, errors.New(`GET /api/media-sources: Get "/api/media-sources": unsupported protocol scheme ""`)
}

func tunarrLibraryCheck(t *testing.T, s *Handlers) httpcore.SetupCheck {
	t.Helper()
	for _, c := range s.runConnectionChecks(context.Background()) {
		if c.Name == "tunarr_library" {
			return c
		}
	}
	t.Fatal("no tunarr_library check")
	return httpcore.SetupCheck{}
}

// #3403: a Tunarr with no address is never probed, so no Go transport error can reach a row.
func TestTunarrLibraryCheck_NoAddressIsNotSetUpAndNeverProbed(t *testing.T) {
	tunarr := &failingTunarr{}
	s := &Handlers{tunarrConnect: tunarr, liveSettings: mapSettings{}}

	check := tunarrLibraryCheck(t, s)

	if tunarr.calls != 0 {
		t.Errorf("Tunarr was probed %d times with no address", tunarr.calls)
	}
	if check.OK || !check.NotSetUp {
		t.Errorf("check = %+v, want not ok and not set up", check)
	}
	if strings.Contains(check.Hint, "unsupported protocol scheme") || check.Hint != "set the Tunarr URL" {
		t.Errorf("hint = %q, want the plain 'set the Tunarr URL'", check.Hint)
	}
}

// With an address, a probe failure is a real failure and stays "needs attention".
func TestTunarrLibraryCheck_WithAddressFailureIsNotSoftened(t *testing.T) {
	s := &Handlers{tunarrConnect: &failingTunarr{}, liveSettings: mapSettings{"tunarr.url": "http://tunarr.invalid:8000"}}

	check := tunarrLibraryCheck(t, s)

	if check.OK || check.NotSetUp {
		t.Errorf("check = %+v, want a real failure", check)
	}
}
