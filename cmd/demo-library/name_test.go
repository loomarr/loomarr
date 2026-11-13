package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/loomarr/loomarr/internal/demolibrary"
)

// A channel the seed has just made is still being built by the backend, which moves its revision
// on, so naming it can be refused as an edit to an older version (#3565). The seed reads it again
// and names it, rather than failing the whole seed on the second channel.
func TestNameChannelRetriesAnEditTheBackendRefusedAsStale(t *testing.T) {
	revision, patches := int64(1), 0
	var named map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"name": "Untitled", "number": 102, "revision": revision})
		case http.MethodPatch:
			patches++
			var edit map[string]any
			_ = json.NewDecoder(r.Body).Decode(&edit)
			if patches == 1 {
				// The backend wrote the channel between the seed's read and its edit.
				revision++
				http.Error(w, `{"title":"Channel changed","status":409}`, http.StatusConflict)
				return
			}
			if got := int64(edit["revision"].(float64)); got != revision {
				http.Error(w, `{"title":"Channel changed","status":409}`, http.StatusConflict)
				return
			}
			named = edit
			_ = json.NewEncoder(w).Encode(map[string]any{})
		}
	}))
	t.Cleanup(srv.Close)
	b, err := newBackend(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	c := demolibrary.Channel{Number: 102, Name: "Late-Night Sci-Fi", Group: "Movies", Strategy: "sequential"}
	if err := b.nameChannel(context.Background(), "ch_1", c); err != nil {
		t.Fatalf("name channel: %v", err)
	}
	if patches != 2 || named["name"] != c.Name {
		t.Fatalf("patches = %d, named %v; want the second edit, at the new revision, to name it", patches, named)
	}
}
