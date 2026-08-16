package app

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
	"net/textproto"
	"strings"
	"testing"

	"github.com/mantonx/loomarr/internal/store"
)

// The image service's wiring test (§22, V52).
//
// ⚠ This exists because of a specific failure mode this repo keeps hitting: a component that is
// built, unit-tested and green, and that NOTHING IMPORTS. `internal/images` and its three routes
// both shipped and passed their own tests while `Server.images` stayed nil, so every one of those
// routes 404'd in a running instance. PROGRESS.md records the same shape against V1, V17a and V23.
//
// No unit test can catch it, because the defect is precisely the absence of a caller. Only driving
// the real composition root can — hence BuildHandler, the same choice backupjob_test.go makes and
// for the same reason.

// imageDTO mirrors the fields of api.ImageDTO this test asserts on.
type imageDTO struct {
	Hash        string `json:"hash"`
	Role        string `json:"role"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	Placeholder string `json:"placeholder"`
	SrcSetWebP  string `json:"srcSetWebp"`
	Src         string `json:"src"`
}

// pngUpload builds a multipart body holding one real PNG.
//
// ⚠ Real encoded bytes, not a stub: images.Ingest DECODES before storing, both as the format
// allowlist and as the only proof the bytes are an image at all. A placeholder byte slice would be
// refused, and the test would be asserting the refusal path while claiming to test the happy one.
func pngUpload(t *testing.T, w, h int) (string, []byte) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 0x40, A: 0xff})
		}
	}
	var raw bytes.Buffer
	if err := png.Encode(&raw, img); err != nil {
		t.Fatal(err)
	}

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	// ⚠ CreateFormFile labels the part `application/octet-stream`, which the operation's
	// `contentType:"image/png,…"` tag rejects with a 422 before the handler is reached. Setting the
	// part's own Content-Type is what a browser does. It is a first-pass filter and NOT the
	// security boundary — the authority is the byte sniff inside images.Ingest, which is exactly
	// why a caller CAN label anything image/png and still be refused on the bytes.
	hdr := make(textproto.MIMEHeader)
	hdr.Set("Content-Disposition", `form-data; name="file"; filename="icon.png"`)
	hdr.Set("Content-Type", "image/png")
	part, err := mw.CreatePart(hdr)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(raw.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	return mw.FormDataContentType(), body.Bytes()
}

// imageServer brings up a real handler with the image directory pointed at a temp dir.
//
// IMAGES_DIR goes through the environment because that is the top of the resolution ladder
// (env > database > default, config-design §3) — so this also exercises the setting actually being
// read, rather than a value handed straight to the constructor.
func imageServer(t *testing.T) *httptest.Server {
	t.Helper()
	t.Setenv("API_TOKEN", "test-app-token")
	t.Setenv("IMAGES_DIR", t.TempDir())
	// Deliberately unreachable from a browser. Machine clients need this configured absolute
	// address, but an ImageDTO is consumed by the in-app browser and must stay on the page's own
	// origin. If the DTO leaks this value, its ThumbHash paints while every real rendition fails.
	t.Setenv("SERVER_PUBLIC_URL", "http://machine-client-only.invalid:8080")

	st, err := store.Open(context.Background(), "sqlite://"+t.TempDir()+"/img.db", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })

	h, err := BuildHandler(context.Background(), st, slog.New(slog.DiscardHandler), Overrides{})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

func TestImageRoutesAreWired(t *testing.T) {
	srv := imageServer(t)

	// --- upload ---------------------------------------------------------------------------
	ctype, body := pngUpload(t, 300, 450) // a 2:3 poster
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/v1/images?role=poster&visibility=member", bytes.NewReader(body))
	req.Header.Set("Content-Type", ctype)
	req.Header.Set("Authorization", "Bearer test-app-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		// ⚠ 501 here is the exact regression this test guards: it is what `s.images == nil`
		// answers, i.e. the service exists but the composition root never handed it over.
		t.Fatalf("POST /v1/images → %d, want 200 (501 means Server.images is nil again)", resp.StatusCode)
	}
	var uploaded imageDTO
	if err := json.NewDecoder(resp.Body).Decode(&uploaded); err != nil {
		t.Fatal(err)
	}
	if len(uploaded.Hash) != 64 {
		t.Fatalf("upload returned hash %q, want a 64-char sha256", uploaded.Hash)
	}
	if uploaded.Width != 300 || uploaded.Height != 450 {
		t.Errorf("upload reported %dx%d, want 300x450 — the DTO must carry the ORIGINAL's dimensions so an <img> can avoid layout shift", uploaded.Width, uploaded.Height)
	}
	if uploaded.Placeholder == "" {
		t.Error("upload returned an empty placeholder; the ThumbHash is what renders before the image loads")
	}
	if !strings.HasPrefix(uploaded.Src, "/v1/images/") {
		t.Errorf("upload src = %q, want a same-origin image path; server.public_url is for machine clients, not the in-app browser", uploaded.Src)
	}
	if strings.Contains(uploaded.SrcSetWebP, "machine-client-only.invalid") {
		t.Errorf("upload srcSetWebp = %q, want same-origin candidates; an off-origin public URL strands every browser image behind its ThumbHash", uploaded.SrcSetWebP)
	}

	// --- read the record ------------------------------------------------------------------
	rec := getJSON[imageDTO](t, srv, "/v1/images/"+uploaded.Hash)
	if rec.Hash != uploaded.Hash {
		t.Errorf("GET /v1/images/{hash} returned %q, want %q", rec.Hash, uploaded.Hash)
	}
	if !strings.Contains(rec.SrcSetWebP, "w") {
		t.Errorf("srcSetWebp = %q, want width descriptors — a srcset was the whole point of §22", rec.SrcSetWebP)
	}

	// --- serve the bytes ------------------------------------------------------------------
	// A width off the ladder on purpose: the service must SNAP it, because honouring arbitrary
	// widths would let an unauthenticated caller drive unbounded encodes.
	byteResp := get(t, srv, "/v1/images/"+uploaded.Hash+"/w301.webp")
	defer func() { _ = byteResp.Body.Close() }()
	if byteResp.StatusCode != http.StatusOK {
		t.Fatalf("GET a webp rendition → %d, want 200", byteResp.StatusCode)
	}
	if ct := byteResp.Header.Get("Content-Type"); ct != "image/webp" {
		t.Errorf("rendition Content-Type = %q, want image/webp", ct)
	}

	// AVIF is job-produced, so a cold one is absent — and absent must be a 404 the browser can
	// fall past, never a 500 (§22: the caller omits the <source> and takes WebP).
	avifResp := get(t, srv, "/v1/images/"+uploaded.Hash+"/w300.avif")
	defer func() { _ = avifResp.Body.Close() }()
	if avifResp.StatusCode != http.StatusNotFound {
		t.Errorf("GET a cold avif rendition → %d, want 404 (it is job-produced, and absent is normal)", avifResp.StatusCode)
	}

	// --- observe the real worker boundary --------------------------------------------------
	metricsResp := get(t, srv, "/metrics")
	defer func() { _ = metricsResp.Body.Close() }()
	metricsBody, err := io.ReadAll(metricsResp.Body)
	if err != nil {
		t.Fatal(err)
	}
	metricsText := string(metricsBody)
	for _, want := range []string{
		`loomarr_image_worker_operations_total{kind="inspect",result="success"}`,
		`loomarr_image_worker_operations_total{kind="render",result="success"}`,
		`loomarr_image_worker_queue_wait_seconds_count{class="interactive"}`,
		`loomarr_image_worker_peak_rss_bytes_count{kind="inspect"}`,
		`loomarr_image_worker_in_flight 0`,
	} {
		if !strings.Contains(metricsText, want) {
			t.Errorf("/metrics missing %q; image worker telemetry is not wired through the composition root", want)
		}
	}
}

// A hash that was never stored must 404 rather than 500 — the serve route is RolePublic, so this
// is an unauthenticated caller's view of the error surface.
func TestImageServeUnknownHashIs404(t *testing.T) {
	srv := imageServer(t)
	resp := get(t, srv, "/v1/images/"+strings.Repeat("a", 64)+"/w300.webp")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET an unknown image → %d, want 404", resp.StatusCode)
	}
}

func get(t *testing.T, srv *httptest.Server, path string) *http.Response {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, srv.URL+path, nil)
	req.Header.Set("Authorization", "Bearer test-app-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func getJSON[T any](t *testing.T, srv *httptest.Server, path string) T {
	t.Helper()
	resp := get(t, srv, path)
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s → %d, want 200", path, resp.StatusCode)
	}
	var out T
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	return out
}
