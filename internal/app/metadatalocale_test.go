package app

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/loomarr/loomarr/internal/library"
)

// The household's metadata language is its setting, else the media server's, asked once an hour.
// The media server's country stands only when it is the US, the one ratings ladder there is.
func TestMetadataLocaleDefaultsToTheMediaServers(t *testing.T) {
	var asked atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		asked.Add(1)
		_, _ = w.Write([]byte(`{"PreferredMetadataLanguage":"fr","MetadataCountryCode":"CA"}`))
	}))
	t.Cleanup(srv.Close)
	lib := library.New(library.Emby, srv.URL, "test-token", "dev-1")

	locale := metadataLocale(visionSet(t, nil), lib)
	for range 3 {
		if language, country := locale(); language != "fr" || country != "" {
			t.Fatalf("locale = %q, %q; want the server's French, and no country (Canada has no ladder)", language, country)
		}
	}
	if n := asked.Load(); n != 1 {
		t.Errorf("the media server was asked %d times in three reads, want once", n)
	}

	set := visionSet(t, map[string]string{"metadata.language": "pt-BR", "metadata.country": "US"})
	if language, country := metadataLocale(set, lib)(); language != "pt-BR" || country != "US" {
		t.Errorf("locale = %q, %q; want the settings' pt-BR, US", language, country)
	}
	if language, country := metadataLocale(visionSet(t, nil), nil)(); language != "" || country != "" {
		t.Errorf("without a media server, locale = %q, %q; want none (TMDB reads it as en-US)", language, country)
	}
}
