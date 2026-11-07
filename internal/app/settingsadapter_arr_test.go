package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Each arr's Test probes that arr alone and names it, whichever slot it fills.
func TestArrCheckProbesOneArrAndNamesIt(t *testing.T) {
	t.Parallel()
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/system/status" || r.Header.Get("X-Api-Key") != "good" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(ok.Close)
	conn := func(base, key string) func() (string, string) { return func() (string, string) { return base, key } }

	for _, name := range []string{"Sonarr", "Radarr"} {
		if pass, hint := arrCheck(name, conn(ok.URL, "good"))(context.Background()); !pass {
			t.Errorf("%s with a good key: %q", name, hint)
		}
		pass, hint := arrCheck(name, conn(ok.URL, "bad"))(context.Background())
		if pass || !strings.HasPrefix(hint, name+" rejected the API key") {
			t.Errorf("%s with a bad key = %t, %q", name, pass, hint)
		}
		if pass, hint := arrCheck(name, conn(" ", ""))(context.Background()); pass || hint != "set the "+name+" URL" {
			t.Errorf("%s unset = %t, %q", name, pass, hint)
		}
	}
}
