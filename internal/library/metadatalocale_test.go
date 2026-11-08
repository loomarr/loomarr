package library_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/loomarr/loomarr/internal/library"
)

// The media server's metadata language and country are read from its server-wide configuration,
// trimmed, the country upper-cased.
func TestMetadataLocaleReadsTheServerConfiguration(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/System/Configuration" {
			t.Errorf("read %s, want /System/Configuration", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"PreferredMetadataLanguage":" pt-BR ","MetadataCountryCode":"br","UICulture":"en-us"}`))
	}))
	t.Cleanup(srv.Close)
	language, country, err := library.New(library.Emby, srv.URL, "test-token", "dev-1").MetadataLocale(context.Background())
	if err != nil || language != "pt-BR" || country != "BR" {
		t.Fatalf("locale = %q, %q, %v; want pt-BR, BR", language, country, err)
	}
}
