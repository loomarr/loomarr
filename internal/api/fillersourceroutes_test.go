package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/mantonx/loomarr/internal/store"
)

func sourceReq(t *testing.T, method, url, body, token string) *http.Response {
	t.Helper()
	var rdr *bytes.Reader
	if body == "" {
		rdr = bytes.NewReader(nil)
	} else {
		rdr = bytes.NewReader([]byte(body))
	}
	req, err := http.NewRequest(method, url, rdr)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = res.Body.Close() })
	return res
}

// Registering a source records that it EXISTS and is allowed. It must not download anything —
// that is the ingest path, and a composed pull goes through the approval gate.
func TestAddFillerSource_RegistersEnabledAndDownloadsNothing(t *testing.T) {
	srv, st, ff := newFillerServer(t)

	res := sourceReq(t, http.MethodPost, srv.URL+"/v1/filler/sources",
		`{"uri":"https://archive.org/details/classic_tv_commercials"}`, adminToken)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	var body struct {
		ID      string `json:"id"`
		Label   string `json:"label"`
		Enabled bool   `json:"enabled"`
	}
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.ID != "classic_tv_commercials" {
		t.Errorf("id = %q, want the parsed identifier", body.ID)
	}
	// ⚠ A Go bool zero-values to false, so a source registered "off" is the failure mode this
	// asserts against: it would sit in the UI switched off, never fetch, and look like a bug.
	if !body.Enabled {
		t.Error("a newly registered source is switched off")
	}
	if ff.syncs != 0 {
		t.Errorf("registering triggered %d syncs — it must download nothing", ff.syncs)
	}

	got, err := st.ListFillerSources(context.Background())
	if err != nil || len(got) != 1 {
		t.Fatalf("store holds %d sources (%v), want 1", len(got), err)
	}
	if !got[0].Enabled {
		t.Error("persisted source is disabled")
	}
}

// An unparseable paste must be refused, not turned into a row that can never fetch.
func TestAddFillerSource_RefusesSomethingThatIsNotACollection(t *testing.T) {
	srv, st, _ := newFillerServer(t)

	res := sourceReq(t, http.MethodPost, srv.URL+"/v1/filler/sources",
		`{"uri":"https://example.com/some/page"}`, adminToken)
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", res.StatusCode)
	}
	if got, _ := st.ListFillerSources(context.Background()); len(got) != 0 {
		t.Errorf("stored %d sources for an unparseable uri", len(got))
	}
}

// The switch flips the column, and — the part worth pinning — leaves the clips alone. The
// Sources tab tells the operator "clips already in the catalog stay put"; that is a promise.
func TestSetFillerSourceEnabled_DisablingKeepsTheClips(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	ctx := context.Background()
	if err := st.UpsertFillerSource(ctx, store.NewFillerSource("classic", "archive", "classic", "Classic", time.Now().UTC())); err != nil {
		t.Fatal(err)
	}
	seedClip(t, st, "classic/ad.mp4", "commercial", 1992, "kids", "toys")

	res := sourceReq(t, http.MethodPatch, srv.URL+"/v1/filler/sources/classic", `{"enabled":false}`, adminToken)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}

	got, err := st.ListFillerSources(ctx)
	if err != nil || len(got) != 1 {
		t.Fatalf("sources = %d (%v)", len(got), err)
	}
	if got[0].Enabled {
		t.Error("source still enabled after being switched off")
	}
	if _, err := st.GetClip(ctx, "classic/ad.mp4"); err != nil {
		t.Errorf("disabling a source removed its clip: %v — the switch is not a delete", err)
	}
}

// A row with no work behind it gets no switch, and asking for one is refused rather than
// silently accepted. Storing a flag nothing reads is how a control that changes nothing ships.
func TestSetFillerSourceEnabled_RefusesRowsWithNothingToStop(t *testing.T) {
	srv, _, _ := newFillerServer(t)

	for _, id := range []string{"library", "remote"} {
		res := sourceReq(t, http.MethodPatch, srv.URL+"/v1/filler/sources/"+id, `{"enabled":false}`, adminToken)
		if res.StatusCode != http.StatusConflict {
			t.Errorf("%s: status = %d, want 409", id, res.StatusCode)
		}
	}
}

// Deleting forgets the registration. ⚠ It must NOT take the clips: they are real files, already
// tagged and possibly pinned into a channel.
func TestDeleteFillerSource_ForgetsTheSourceNotTheClips(t *testing.T) {
	srv, st, _ := newFillerServer(t)
	ctx := context.Background()
	if err := st.UpsertFillerSource(ctx, store.NewFillerSource("classic", "archive", "classic", "Classic", time.Now().UTC())); err != nil {
		t.Fatal(err)
	}
	seedClip(t, st, "classic/ad.mp4", "commercial", 1992, "kids", "toys")

	if res := sourceReq(t, http.MethodDelete, srv.URL+"/v1/filler/sources/classic", "", adminToken); res.StatusCode != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", res.StatusCode)
	}
	if got, _ := st.ListFillerSources(ctx); len(got) != 0 {
		t.Errorf("%d sources remain", len(got))
	}
	if _, err := st.GetClip(ctx, "classic/ad.mp4"); err != nil {
		t.Errorf("deleting a source removed its clip: %v", err)
	}
}

// The derived rows describe configuration; deleting one would have to mean "unset filler.dir",
// which belongs in Settings where the consequence is legible.
func TestDeleteFillerSource_RefusesTheDerivedRows(t *testing.T) {
	srv, _, _ := newFillerServer(t)

	for _, id := range []string{"folder", "library", "remote"} {
		res := sourceReq(t, http.MethodDelete, srv.URL+"/v1/filler/sources/"+id, "", adminToken)
		if res.StatusCode != http.StatusConflict {
			t.Errorf("%s: status = %d, want 409", id, res.StatusCode)
		}
	}
}

// §19 negatives: these routes name filesystem paths and change what gets downloaded.
func TestFillerSourceRoutes_RequireAdmin(t *testing.T) {
	srv, _, _ := newFillerServer(t)

	for _, tc := range []struct{ method, path, body string }{
		{http.MethodPost, "/v1/filler/sources", `{"uri":"classic"}`},
		{http.MethodPatch, "/v1/filler/sources/classic", `{"enabled":false}`},
		{http.MethodDelete, "/v1/filler/sources/classic", ""},
	} {
		res := sourceReq(t, tc.method, srv.URL+tc.path, tc.body, "")
		if res.StatusCode == http.StatusOK || res.StatusCode == http.StatusNoContent {
			t.Errorf("%s %s succeeded with no credential (%d)", tc.method, tc.path, res.StatusCode)
		}
	}
}
