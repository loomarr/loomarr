package app

import (
	"testing"

	"github.com/mantonx/loomarr/internal/library"
)

func TestResolvedLibraryConnectionReadsLiveSettingsTogether(t *testing.T) {
	for _, env := range []string{"LIBRARY_FLAVOR", "LIBRARY_URL", "LIBRARY_TOKEN"} {
		t.Setenv(env, "")
	}
	set := visionSet(t, map[string]string{
		"library.flavor": "emby", "library.url": "http://emby-old:8096", "library.token": "token-old",
	})
	current := set.libraryConn()

	first := current()
	assertLibraryConnection(t, first, library.Emby, "http://emby-old:8096", "token-old")
	set.svc.SetDB(map[string]string{
		"library.flavor": "jellyfin", "library.url": "http://jellyfin-new:8096", "library.token": "token-new",
	})
	assertLibraryConnection(t, current(), library.Jellyfin, "http://jellyfin-new:8096", "token-new")
	assertLibraryConnection(t, first, library.Emby, "http://emby-old:8096", "token-old")

	oldValues := map[string]string{
		"library.flavor": "emby", "library.url": "http://emby-old:8096", "library.token": "token-old",
	}
	newValues := map[string]string{
		"library.flavor": "jellyfin", "library.url": "http://jellyfin-new:8096", "library.token": "token-new",
	}
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
				set.svc.SetDB(oldValues)
				set.svc.SetDB(newValues)
			}
		}
	}()

	var mixed *library.Connection
	for range 50_000 {
		connection := current()
		oldConnection := connection.Flavor == library.Emby &&
			connection.BaseURL == "http://emby-old:8096" && connection.Token == "token-old"
		newConnection := connection.Flavor == library.Jellyfin &&
			connection.BaseURL == "http://jellyfin-new:8096" && connection.Token == "token-new"
		if !oldConnection && !newConnection {
			mixed = &connection
			break
		}
	}
	close(stop)
	<-done
	if mixed != nil {
		t.Fatalf("connection mixed values across a concurrent hot-apply: %+v", *mixed)
	}
}

func TestResolvedLibraryConfiguredUsesClientValidation(t *testing.T) {
	for _, env := range []string{"LIBRARY_FLAVOR", "LIBRARY_URL", "LIBRARY_TOKEN"} {
		t.Setenv(env, "")
	}
	tests := []struct {
		name   string
		values map[string]string
		want   bool
	}{
		{
			name: "complete HTTP connection",
			values: map[string]string{
				"library.flavor": "emby", "library.url": "http://emby:8096", "library.token": "token",
			},
			want: true,
		},
		{
			name: "complete HTTPS connection",
			values: map[string]string{
				"library.flavor": "jellyfin", "library.url": "https://jellyfin.example", "library.token": "token",
			},
			want: true,
		},
		{
			name: "unsupported URL scheme",
			values: map[string]string{
				"library.flavor": "emby", "library.url": "ftp://emby:8096", "library.token": "token",
			},
		},
		{
			name: "invalid URL",
			values: map[string]string{
				"library.flavor": "emby", "library.url": "http://[", "library.token": "token",
			},
		},
		{
			name: "unsupported flavor",
			values: map[string]string{
				"library.flavor": "plex", "library.url": "http://plex:32400", "library.token": "token",
			},
		},
		{
			name: "blank token",
			values: map[string]string{
				"library.flavor": "emby", "library.url": "http://emby:8096", "library.token": "   ",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := visionSet(t, test.values).libraryConfigured(); got != test.want {
				t.Fatalf("libraryConfigured() = %v, want %v", got, test.want)
			}
		})
	}
}

func assertLibraryConnection(t *testing.T, got library.Connection, flavor library.Flavor, url, token string) {
	t.Helper()
	if got.Flavor != flavor || got.BaseURL != url || got.Token != token {
		t.Fatalf("connection = %+v, want {%s %s %s}", got, flavor, url, token)
	}
}
