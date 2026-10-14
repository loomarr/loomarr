package provision_test

import (
	"encoding/json"
	"testing"

	"github.com/loomarr/loomarr/internal/provision"
	"github.com/loomarr/loomarr/internal/testkit"
)

// TestWebhookKeyParity is the §21 phase-2 gate test: a key derived from a Title
// must equal the key derived from the identity a real webhook carries. Uses the
// pinned Phase-0 Radarr import fixture (remoteMovie.tmdbId). That capture is
// recorded evidence of a real Radarr, so the request side keeps its real id and title.
func TestWebhookKeyParity(t *testing.T) {
	// The request side: a Title we would have created for this movie.
	titleKey, err := provision.Title{MediaType: provision.Movie, TMDBID: 1111867, Name: "In Flames"}.Key()
	if err != nil {
		t.Fatal(err)
	}

	// The webhook side: parse the real Radarr import payload the way Phase 6 will.
	var wh struct {
		RemoteMovie struct {
			TMDBID int `json:"tmdbId"`
		} `json:"remoteMovie"`
	}
	if err := json.Unmarshal(testkit.Fixture(t, "radarr/import_webhook.json"), &wh); err != nil {
		t.Fatal(err)
	}
	if wh.RemoteMovie.TMDBID == 0 {
		t.Fatal("fixture missing remoteMovie.tmdbId — webhook correlation would break")
	}

	webhookKey, err := provision.KeyFromWebhook(provision.Movie, wh.RemoteMovie.TMDBID, 0)
	if err != nil {
		t.Fatal(err)
	}

	if titleKey != webhookKey {
		t.Errorf("key parity broken: title=%q webhook=%q", titleKey, webhookKey)
	}
}
