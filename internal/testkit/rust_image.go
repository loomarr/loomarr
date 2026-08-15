package testkit

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/mantonx/loomarr/internal/images/rustgen"
)

// RustImageRenderer opens the same required worker used in production. `make check` builds it
// before Go tests; using the real executable here keeps protocol and pixel behavior from drifting
// behind an in-process fake.
func RustImageRenderer(t testing.TB) *rustgen.Generator {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate testkit source")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	worker := filepath.Join(root, "target", "debug", "loomarr-image")
	if override := os.Getenv("LOOMARR_IMAGE_WORKER"); override != "" {
		worker = override
	}
	gen, err := rustgen.Open(worker, rustgen.Contract{
		Protocol: 1, Release: "dev", Recipe: "loomarr-rendition-v1",
		RequiredFormats: []string{"avif", "jpeg", "webp"}, Animation: true,
	})
	if err != nil {
		t.Fatalf("open required Rust image worker (run `cargo build -p loomarr-image` first): %v", err)
	}
	return gen
}
