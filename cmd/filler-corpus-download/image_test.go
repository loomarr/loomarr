package main

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/loomarr/loomarr/internal/fillercorpus"
)

func TestDownloadPublishesOnlyCompleteDecodedPrivateImage(t *testing.T) {
	raw := testPNG(t, 3, 2)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Header.Get("User-Agent") != "Loomarr test" {
			t.Fatalf("user agent = %q", request.Header.Get("User-Agent"))
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(raw)
	}))
	defer server.Close()
	output := filepath.Join(t.TempDir(), "media")
	if err := ensurePrivateDirectory(output); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(output, "case.png")
	item := plannedDownload{candidate: fillercorpus.InventoryCase{
		CaseID: "metmuseum.org/collection/1", AllowedMediaHosts: []string{"127.0.0.1"},
		Representation: fillercorpus.InventoryRepresentation{URL: server.URL + "/case.png", MIMEType: "image/png", Bytes: int64(len(raw))},
	}, path: target}
	hashes, size, verified, err := download(context.Background(), server.Client(), item, "Loomarr test", 100)
	if err != nil {
		t.Fatal(err)
	}
	if size != int64(len(raw)) || hashes.sha256 == "" || verified.mediaType != "image/png" || verified.width != 3 || verified.height != 2 {
		t.Fatalf("size=%d hashes=%+v verified=%+v", size, hashes, verified)
	}
	info, err := os.Lstat(target)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		t.Fatalf("published mode = %v", info.Mode())
	}
}

func TestDownloadRejectsResponseMIMEAndIncompleteImageBeforePublication(t *testing.T) {
	raw := testPNG(t, 2, 2)
	for name, testCase := range map[string]struct {
		contentType string
		body        []byte
	}{
		"wrong MIME":     {contentType: "image/jpeg", body: raw},
		"trailing bytes": {contentType: "image/png", body: append(append([]byte(nil), raw...), []byte("trailing")...)},
	} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", testCase.contentType)
				_, _ = w.Write(testCase.body)
			}))
			defer server.Close()
			output := filepath.Join(t.TempDir(), "media")
			if err := ensurePrivateDirectory(output); err != nil {
				t.Fatal(err)
			}
			target := filepath.Join(output, "case.png")
			item := plannedDownload{candidate: fillercorpus.InventoryCase{Representation: fillercorpus.InventoryRepresentation{URL: server.URL, MIMEType: "image/png", Bytes: int64(len(testCase.body))}}, path: target}
			if _, _, _, err := download(context.Background(), server.Client(), item, "Loomarr test", 100); err == nil {
				t.Fatal("invalid image published")
			}
			if _, err := os.Lstat(target); !os.IsNotExist(err) {
				t.Fatalf("target exists after rejection: %v", err)
			}
		})
	}
}

func TestVerifyDownloadedRepresentationEnforcesPixelCeiling(t *testing.T) {
	path := filepath.Join(t.TempDir(), "case.png")
	if err := os.WriteFile(path, testPNG(t, 2, 2), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := verifyDownloadedRepresentation(path, fillercorpus.InventoryRepresentation{MIMEType: "image/png"}, 3)
	if err == nil || !strings.Contains(err.Error(), "exceeds 3 pixels") {
		t.Fatalf("err = %v", err)
	}
}

func TestEnsurePrivateDirectoryTightensModeAndRejectsSymlink(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "media")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ensurePrivateDirectory(directory); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(directory)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(directory, link); err != nil {
		t.Fatal(err)
	}
	if err := ensurePrivateDirectory(link); err == nil {
		t.Fatal("symlinked output accepted")
	}
}

func testPNG(t *testing.T, width, height int) []byte {
	t.Helper()
	value := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := range height {
		for x := range width {
			value.Set(x, y, color.RGBA{R: uint8(x * 20), G: uint8(y * 20), B: 80, A: 255})
		}
	}
	var raw bytes.Buffer
	if err := png.Encode(&raw, value); err != nil {
		t.Fatal(err)
	}
	return raw.Bytes()
}
