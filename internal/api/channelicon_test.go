package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mantonx/loomarr/internal/api"
	"github.com/mantonx/loomarr/internal/schedule"
	"github.com/mantonx/loomarr/internal/store"
)

// Channel icon upload + serve had NO test coverage at all before this file — `icons_test.go`
// only ever exercised /icon-suggestions, which is a different endpoint that returns JSON.
// That gap mattered twice over: the upload is the app's only file-upload surface, and the
// pair carries a deliberate stored-XSS defence (§icon, raster-only) that nothing verified.
//
// ⚠ **The defence is a BYTE SNIFF, and the distinction is the point of TestUploadChannelIcon_
// RefusesSVGDeclaredAsPNG.** huma's multipart `contentType` tag validates the Content-Type the
// client DECLARES on the part; it only sniffs when that header is absent. If the handler ever
// starts trusting the declared type — or the sniff is dropped as redundant because "huma already
// validates it" — an SVG carrying <script> gets stored and served from Loomarr's own origin by
// the PUBLIC serve half. Deleting the sniff is a one-line change that no other test notices.

func newIconUploadServer(t *testing.T) (*httptest.Server, store.Store) {
	t.Helper()
	st, err := store.Open(context.Background(), "sqlite://"+t.TempDir()+"/iconupload.db", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.UpsertChannel(context.Background(), store.Channel{
		Channel: schedule.Channel{ID: "ch-1", Name: "Star Trek", Number: 42, Status: "live"},
	}); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(api.Router(slog.New(slog.DiscardHandler), api.Options{
		Store: st,
		Auth:  testAuthorizer{},
		Log:   slog.New(slog.DiscardHandler),
	}))
	t.Cleanup(srv.Close)
	return srv, st
}

// pngBytes is a real 1x1 PNG — encoded rather than hardcoded so the byte signature is
// genuinely what http.DetectContentType keys on.
func pngBytes(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 1, G: 2, B: 3, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// postIcon uploads `data` as the `file` field, declaring `declaredType` on the part.
func postIcon(t *testing.T, srv *httptest.Server, token, filename, declaredType string, data []byte) *http.Response {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	h := make(map[string][]string)
	h["Content-Disposition"] = []string{`form-data; name="file"; filename="` + filename + `"`}
	h["Content-Type"] = []string{declaredType}
	part, err := w.CreatePart(h)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/channels/ch-1/icon", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func TestUploadChannelIcon_StoresAndServes(t *testing.T) {
	srv, _ := newIconUploadServer(t)
	want := pngBytes(t)

	resp := postIcon(t, srv, adminToken, "logo.png", "image/png", want)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("upload = %d, want 200: %s", resp.StatusCode, b)
	}
	var out struct {
		Logo string `json:"logo"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if out.Logo == "" {
		t.Error("upload returned no logo URL; the channel would still show no icon")
	}

	// The serve half is PUBLIC — no credential — because Tunarr fetches it machine-to-machine.
	// Asserted with an empty token on purpose: a regression to the fail-closed default would
	// 401 here and break every channel logo in Tunarl silently.
	got := do(t, srv, http.MethodGet, "/v1/channels/ch-1/icon", "", "")
	defer func() { _ = got.Body.Close() }()
	if got.StatusCode != http.StatusOK {
		t.Fatalf("serve without credentials = %d, want 200 (RolePublic)", got.StatusCode)
	}
	if ct := got.Header.Get("Content-Type"); ct != "image/png" {
		t.Errorf("served Content-Type = %q, want image/png", ct)
	}
	body, _ := io.ReadAll(got.Body)
	if !bytes.Equal(body, want) {
		t.Errorf("served %d bytes, want the %d uploaded", len(body), len(want))
	}
}

// ⚠ THE SECURITY TEST. An SVG labelled image/png must be refused.
//
// huma's MimeTypeValidator accepts this part — it trusts the declared Content-Type — so the
// ONLY thing standing between this payload and a stored, publicly-served script is the byte
// sniff in uploadChannelIcon. If that sniff is removed, this is the test that goes red.
func TestUploadChannelIcon_RefusesSVGDeclaredAsPNG(t *testing.T) {
	srv, st := newIconUploadServer(t)
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)

	resp := postIcon(t, srv, adminToken, "logo.png", "image/png", svg)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnsupportedMediaType {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("SVG-as-PNG upload = %d, want 415. A stored SVG is served from Loomarr's own "+
			"origin by the public serve route, so accepting one is stored XSS: %s", resp.StatusCode, b)
	}

	// And nothing was written — a refusal that still stored the bytes would leave them
	// reachable if the serve route ever stopped checking.
	if _, _, _, ok, err := st.GetChannelIcon(context.Background(), "ch-1"); err != nil {
		t.Fatal(err)
	} else if ok {
		t.Error("refused upload still stored an icon")
	}
}

func TestUploadChannelIcon_RefusesOversize(t *testing.T) {
	srv, _ := newIconUploadServer(t)
	// A real PNG header followed by padding past the 2 MiB cap, so it fails on SIZE rather
	// than on the sniff — otherwise this would pass for the wrong reason.
	big := append(pngBytes(t), make([]byte, 3<<20)...)

	resp := postIcon(t, srv, adminToken, "big.png", "image/png", big)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusRequestEntityTooLarge && resp.StatusCode != http.StatusBadRequest {
		t.Errorf("oversize upload = %d, want 413 (or 400 from the framework's body cap)", resp.StatusCode)
	}
}

// §19 negative cases: uploading an icon is an authoring action, so member and anonymous are
// refused. Pinned because the upload moved from a hand-written requireRole check to the
// operation's declared role — the whole point is that the answer did not change.
func TestUploadChannelIcon_RoleBoundary(t *testing.T) {
	srv, _ := newIconUploadServer(t)
	data := pngBytes(t)

	for _, tc := range []struct {
		name  string
		token string
		want  int
	}{
		{"member is refused", memberToken, http.StatusForbidden},
		{"anonymous is refused", "", http.StatusUnauthorized},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := postIcon(t, srv, tc.token, "logo.png", "image/png", data)
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode != tc.want {
				t.Errorf("upload = %d, want %d", resp.StatusCode, tc.want)
			}
		})
	}
}
