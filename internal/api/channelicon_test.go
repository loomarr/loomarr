package api_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	"github.com/mantonx/loomarr/internal/images"
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

func newIconUploadServer(t *testing.T) (*httptest.Server, store.Store, *fakeImageService) {
	t.Helper()
	st := openTestStore(t, t.TempDir()+"/iconupload.db")
	t.Cleanup(func() { _ = st.Close() })
	if err := st.UpsertChannel(context.Background(), store.Channel{
		Channel: schedule.Channel{ID: "ch-1", Name: "Star Trek", Number: 42, Status: "live"},
	}); err != nil {
		t.Fatal(err)
	}
	imgs := newFakeImageService()
	srv := httptest.NewServer(api.Router(slog.New(slog.DiscardHandler), api.Options{
		Store:  st,
		Auth:   testAuthorizer{},
		Images: imgs,
		Log:    slog.New(slog.DiscardHandler),
	}))
	t.Cleanup(srv.Close)
	return srv, st, imgs
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

// altPNGBytes is a real 1x1 PNG with a DIFFERENT pixel, so its sha256 differs from pngBytes'.
// ⚠ Derived by changing the content, never by changing a filename or a field — the whole claim
// under test is that the URL follows the bytes.
func altPNGBytes(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 200, G: 100, B: 50, A: 255})
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

// The upload now hands the bytes to the image service (§22, V52 phase 5) instead of writing a
// BLOB to `channel_icons`, and points the channel's logo at the content-addressed URL.
func TestUploadChannelIcon_IngestsAndLinksTheChannel(t *testing.T) {
	srv, st, imgs := newIconUploadServer(t)
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

	req, gotBytes, n := imgs.snapshot()
	if n != 1 {
		t.Fatalf("ingests = %d, want exactly 1", n)
	}
	if !bytes.Equal(gotBytes, want) {
		t.Errorf("ingested %d bytes, want the %d uploaded", len(gotBytes), len(want))
	}

	// ⚠ Every field here is a SILENT failure if it drifts, which is why they are pinned
	// individually rather than as one struct compare that a future field would break.
	if req.Role != images.RoleIcon {
		t.Errorf("role = %q, want %q — the width ladder is per-role, so a poster role would "+
			"serve a channel logo at poster rungs", req.Role, images.RoleIcon)
	}
	if req.Visibility != images.VisibilityPublic {
		t.Errorf("visibility = %q, want %q — Tunarr fetches the icon machine-to-machine with no "+
			"credentials, so a member-visible icon 404s for it and every channel logo vanishes",
			req.Visibility, images.VisibilityPublic)
	}
	if req.Origin != images.OriginUpload {
		t.Errorf("origin = %q, want %q — origin decides recoverability (§22 Durability); a "+
			"mislabelled upload would be treated as re-fetchable and silently dropped by the GC",
			req.Origin, images.OriginUpload)
	}
	// The Ref is what stops the GC collecting a live icon as an orphan.
	if req.OwnerKind != "channel" || req.OwnerID != "ch-1" {
		t.Errorf("owner = %q/%q, want channel/ch-1 — without the Ref the GC collects the icon "+
			"as an orphan while the channel still points at it", req.OwnerKind, req.OwnerID)
	}

	// The logo is the content-addressed URL, and it is the HASH OF THE BYTES — not the channel
	// id, not a fixed string. That is what makes the `immutable` cache header honest and what
	// removes the old `?v=` cache-bust.
	sum := sha256.Sum256(want)
	wantLogo := "/v1/images/" + hex.EncodeToString(sum[:]) + "/w500.jpg"
	if out.Logo != wantLogo {
		t.Errorf("logo = %q, want %q", out.Logo, wantLogo)
	}
	ch, err := st.GetChannel(context.Background(), "ch-1")
	if err != nil {
		t.Fatal(err)
	}
	if ch.Logo != wantLogo {
		t.Errorf("persisted channel logo = %q, want %q — the response and the row disagreeing "+
			"means Tunarr gets whatever the reconcile reads, not what the FE was told", ch.Logo, wantLogo)
	}
}

// ⚠ Re-uploading the SAME bytes must yield the SAME logo URL, and a DIFFERENT image a different
// one. That is content addressing working, and it is what replaced the `?v=` cache-bust: the old
// URL addressed the channel, so a replaced icon reused it and needed a query param to defeat
// downstream caches. If this ever regresses to a channel-addressed URL, Tunarr and Emby serve a
// stale logo indefinitely and nothing else in the suite notices.
func TestUploadChannelIcon_URLFollowsTheBytesNotTheChannel(t *testing.T) {
	srv, _, _ := newIconUploadServer(t)

	logoFor := func(data []byte) string {
		t.Helper()
		resp := postIcon(t, srv, adminToken, "logo.png", "image/png", data)
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
		return out.Logo
	}

	first := logoFor(pngBytes(t))
	if again := logoFor(pngBytes(t)); again != first {
		t.Errorf("same bytes gave %q then %q; identical content must be one URL", first, again)
	}
	if other := logoFor(altPNGBytes(t)); other == first {
		t.Errorf("different bytes both gave %q; a replaced icon would be served stale forever", other)
	}
}

// The round trip: after an upload, the channel resource carries `logoImage` so the frontend's
// <Image> primitive has real dimensions, a ThumbHash and srcsets to work with.
//
// ⚠ Asserted through the HTTP surface rather than by calling channelToDTO, because the failure
// this guards against is a WIRING one — the resolver returning nil, or the handler forgetting to
// pass it — and a direct call to the mapper would pass while the endpoint served nothing.
func TestChannel_CarriesLogoImageAfterUpload(t *testing.T) {
	srv, _, _ := newIconUploadServer(t)
	png := pngBytes(t)

	up := postIcon(t, srv, adminToken, "logo.png", "image/png", png)
	defer func() { _ = up.Body.Close() }()
	if up.StatusCode != http.StatusOK {
		t.Fatalf("upload = %d, want 200", up.StatusCode)
	}

	resp := do(t, srv, http.MethodGet, "/v1/channels/ch-1", adminToken, "")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get channel = %d, want 200", resp.StatusCode)
	}
	var ch struct {
		Logo      string `json:"logo"`
		LogoImage *struct {
			Hash       string `json:"hash"`
			Role       string `json:"role"`
			Width      int    `json:"width"`
			SrcSetWebP string `json:"srcSetWebp"`
		} `json:"logoImage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&ch); err != nil {
		t.Fatal(err)
	}
	if ch.LogoImage == nil {
		t.Fatalf("logoImage absent for logo %q — the frontend falls back to a plain <img> and "+
			"loses srcset, the placeholder and the designed failure state", ch.Logo)
	}
	sum := sha256.Sum256(png)
	if want := hex.EncodeToString(sum[:]); ch.LogoImage.Hash != want {
		t.Errorf("logoImage.hash = %q, want %q", ch.LogoImage.Hash, want)
	}
	if ch.LogoImage.Width == 0 {
		t.Error("logoImage.width is 0 — <Image> reserves the box from this, so a zero means " +
			"cumulative layout shift returns on every surface that renders a channel icon")
	}
	if ch.LogoImage.SrcSetWebP == "" {
		t.Error("logoImage.srcSetWebp is empty; the whole point of the record is the ladder")
	}
}

// ⚠ An external logo must NOT get a logoImage, and this is the case that keeps the fallback path
// honest. Pasting a URL is a supported way to set a channel icon (§icon), so a resolver that
// guessed — or that treated any non-empty logo as one of ours — would either 404 the record or
// hand <Image> a fabricated one.
func TestChannel_OmitsLogoImageForAnExternalURL(t *testing.T) {
	srv, st, _ := newIconUploadServer(t)
	ch, err := st.GetChannel(context.Background(), "ch-1")
	if err != nil {
		t.Fatal(err)
	}
	ch.Logo = "https://image.tmdb.org/t/p/w500/abc.jpg"
	if err := st.UpsertChannel(context.Background(), ch); err != nil {
		t.Fatal(err)
	}

	resp := do(t, srv, http.MethodGet, "/v1/channels/ch-1", adminToken, "")
	defer func() { _ = resp.Body.Close() }()
	var got struct {
		Logo      string          `json:"logo"`
		LogoImage json.RawMessage `json:"logoImage"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Logo != ch.Logo {
		t.Errorf("logo = %q, want the external URL %q preserved", got.Logo, ch.Logo)
	}
	if len(got.LogoImage) != 0 {
		t.Errorf("logoImage = %s, want absent for an external URL", got.LogoImage)
	}
}

// ⚠ THE SECURITY TEST. An SVG labelled image/png must be refused.
//
// huma's MimeTypeValidator accepts this part — it trusts the declared Content-Type — so the
// ONLY thing standing between this payload and a stored, publicly-served script is the byte
// sniff in uploadChannelIcon. If that sniff is removed, this is the test that goes red.
func TestUploadChannelIcon_RefusesSVGDeclaredAsPNG(t *testing.T) {
	srv, _, imgs := newIconUploadServer(t)
	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`)

	resp := postIcon(t, srv, adminToken, "logo.png", "image/png", svg)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusUnsupportedMediaType {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("SVG-as-PNG upload = %d, want 415. A stored SVG is served from Loomarr's own "+
			"origin by the public serve route, so accepting one is stored XSS: %s", resp.StatusCode, b)
	}

	// And nothing was ingested — a refusal that still handed the bytes to the image service
	// would leave them reachable at their content URL, which is PUBLIC by role for icons. The
	// sniff has to run BEFORE the ingest, and this is what pins that ordering.
	if _, _, n := imgs.snapshot(); n != 0 {
		t.Errorf("refused upload still ingested %d image(s); the SVG would be served from "+
			"Loomarr's own origin at its content URL", n)
	}
}

func TestUploadChannelIcon_RefusesOversize(t *testing.T) {
	srv, _, _ := newIconUploadServer(t)
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
	srv, _, _ := newIconUploadServer(t)
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
