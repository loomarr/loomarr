package app

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/loomarr/loomarr/internal/library"
	mptmdb "github.com/loomarr/loomarr/internal/metadataprovider/tmdb"
	"github.com/loomarr/loomarr/internal/platform/settings"
)

// serverLocaleRetry is how long the media server's language is trusted before it is asked again,
// and how long a server that did not answer is left alone.
const serverLocaleRetry = time.Hour

// metadataLocale is the household's metadata language and country (#2151 decision 21):
// metadata.language and metadata.country, each defaulting to the media server's, read per call so
// a saved setting applies at once. Only the US ratings ladder exists, so a media server set to
// another country reads as the US; the setting itself refuses one.
func metadataLocale(set resolved, lib *library.Client) mptmdb.Locale {
	server := &serverLocale{lib: lib}
	return func() (string, string) {
		language := strings.TrimSpace(settings.MetadataLanguage.Get(set))
		country := strings.TrimSpace(settings.MetadataCountry.Get(set))
		if language == "" || country == "" {
			serverLanguage, serverCountry := server.get()
			if language == "" {
				language = serverLanguage
			}
			if country == "" && serverCountry == "US" {
				country = serverCountry
			}
		}
		return language, country
	}
}

// serverLocale caches the media server's metadata language and country. A server that does not
// answer keeps the last answer it gave, so a blip does not switch every title's language back and
// forth.
type serverLocale struct {
	lib *library.Client

	mu                sync.Mutex
	language, country string
	askedAt           time.Time
}

func (s *serverLocale) get() (string, string) {
	if s.lib == nil {
		return "", ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.askedAt.IsZero() && time.Since(s.askedAt) < serverLocaleRetry {
		return s.language, s.country
	}
	s.askedAt = time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if language, country, err := s.lib.MetadataLocale(ctx); err == nil {
		s.language, s.country = language, country
	}
	return s.language, s.country
}
