package app

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/loomarr/loomarr/internal/library/mediaserver"
	mptmdb "github.com/loomarr/loomarr/internal/library/metadataprovider/tmdb"
	"github.com/loomarr/loomarr/internal/platform/settings"
)

// serverLocaleRetry is how long the media server's language is trusted before it is asked again,
// and how long a server that did not answer is left alone.
const serverLocaleRetry = time.Hour

// metadataLocale is the household's metadata language and country (#2151 decision 21):
// metadata.language and metadata.country, each defaulting to the media server's, read per call so
// a saved setting applies at once. Only the US ratings ladder exists, so a media server set to
// another country reads as the US; the setting itself refuses one.
func metadataLocale(set resolved, lib *mediaserver.Client) mptmdb.Locale {
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
	lib *mediaserver.Client

	mu                sync.Mutex
	language, country string
	askedAt           time.Time
}

func (s *serverLocale) get() (string, string) {
	if s.lib == nil {
		return "", ""
	}
	s.mu.Lock()
	if !s.askedAt.IsZero() && time.Since(s.askedAt) < serverLocaleRetry {
		defer s.mu.Unlock()
		return s.language, s.country
	}
	// One caller asks, outside the lock; the others keep the last answer meanwhile.
	s.askedAt = time.Now()
	s.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	language, country, err := s.lib.MetadataLocale(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err == nil {
		s.language, s.country = language, country
	}
	return s.language, s.country
}
