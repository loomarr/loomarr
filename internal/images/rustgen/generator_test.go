package rustgen_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/mantonx/loomarr/internal/images/rustgen"
	"github.com/mantonx/loomarr/internal/testkit"
)

func TestOpenAcceptsMatchingRequiredWorker(t *testing.T) {
	worker := testkit.Executable(t, "loomarr-image", `#!/bin/sh
printf '%s\n' '{"protocol":1,"release":"test-release","recipe":"loomarr-rendition-v2","formats":["jpeg","webp"],"animation":true,"selfTest":true}'
`)

	gen, err := rustgen.Open(worker, rustgen.Contract{
		Protocol:        1,
		Release:         "test-release",
		Recipe:          "loomarr-rendition-v2",
		RequiredFormats: []string{"jpeg", "webp"},
		Animation:       true,
	})
	if err != nil {
		t.Fatalf("open matching worker: %v", err)
	}
	if gen == nil {
		t.Fatal("open matching worker returned nil generator")
	}
}

func TestOpenRejectsEveryRequiredCapabilityMismatch(t *testing.T) {
	tests := []struct {
		name         string
		capabilities string
	}{
		{"protocol", `{"protocol":2,"release":"test-release","recipe":"loomarr-rendition-v2","formats":["avif","jpeg","webp"],"animation":true,"selfTest":true}`},
		{"release", `{"protocol":1,"release":"other","recipe":"loomarr-rendition-v2","formats":["avif","jpeg","webp"],"animation":true,"selfTest":true}`},
		{"recipe", `{"protocol":1,"release":"test-release","recipe":"other","formats":["avif","jpeg","webp"],"animation":true,"selfTest":true}`},
		{"format", `{"protocol":1,"release":"test-release","recipe":"loomarr-rendition-v2","formats":["jpeg","webp"],"animation":true,"selfTest":true}`},
		{"animation", `{"protocol":1,"release":"test-release","recipe":"loomarr-rendition-v2","formats":["avif","jpeg","webp"],"animation":false,"selfTest":true}`},
		{"self test", `{"protocol":1,"release":"test-release","recipe":"loomarr-rendition-v2","formats":["avif","jpeg","webp"],"animation":true,"selfTest":false}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			worker := testkit.Executable(t, "loomarr-image", "#!/bin/sh\nprintf '%s\\n' '"+tt.capabilities+"'\n")
			_, err := rustgen.Open(worker, rustgen.Contract{
				Protocol: 1, Release: "test-release", Recipe: "loomarr-rendition-v2",
				RequiredFormats: []string{"avif", "jpeg", "webp"}, Animation: true,
			})
			if err == nil {
				t.Fatal("Open accepted an incompatible required worker")
			}
		})
	}
}

func TestGenerateReturnsTypedWorkerRefusal(t *testing.T) {
	worker := testkit.Executable(t, "loomarr-image", `#!/bin/sh
if [ "$1" = capabilities ]; then
  printf '%s\n' '{"protocol":1,"release":"test-release","recipe":"loomarr-rendition-v2","formats":["avif","jpeg","webp"],"animation":true,"selfTest":true}'
  exit 0
fi
cat >/dev/null
printf '%s\n' '{"protocol":1,"status":"error","requestId":"bad-source","error":{"code":"unsupported_input","message":"unsupported image signature"}}'
exit 1
`)
	gen, err := rustgen.Open(worker, rustgen.Contract{
		Protocol: 1, Release: "test-release", Recipe: "loomarr-rendition-v2",
		RequiredFormats: []string{"avif", "jpeg", "webp"}, Animation: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = gen.Generate(context.Background(), rustgen.Request{RequestID: "bad-source"})
	var refusal *rustgen.WorkerError
	if !errors.As(err, &refusal) || refusal.Code != "unsupported_input" {
		t.Fatalf("Generate error = %v, want typed unsupported_input", err)
	}
}

func TestGenerateReturnsOnlyAVerifiedStagedManifest(t *testing.T) {
	staging := t.TempDir()
	bytes := []byte("RIFF\x04\x00\x00\x00WEBP")
	digest := fmt.Sprintf("%x", sha256.Sum256(bytes))
	worker := testkit.Executable(t, "loomarr-image", fmt.Sprintf(`#!/bin/sh
if [ "$1" = capabilities ]; then
  printf '%%s\n' '{"protocol":1,"release":"test-release","recipe":"loomarr-rendition-v2","formats":["jpeg","webp"],"animation":true,"selfTest":true}'
  exit 0
fi
cat >/dev/null
printf 'RIFF\004\000\000\000WEBP' > %q/webp-w2.webp
	printf '%%s\n' '{"protocol":1,"status":"ok","requestId":"request-1","source":{"sha256":"%s","mime":"image/png","width":4,"height":2,"bytes":6,"animated":false,"frameCount":1,"durationMs":0,"loopCount":null,"placeholder":"AQID","dominantHex":"#112233"},"outputs":[{"targetId":"webp-w2","relativePath":"webp-w2.webp","recipeId":"loomarr-rendition-v2","format":"webp","mime":"image/webp","requestedWidth":2,"width":2,"height":1,"bytes":12,"sha256":"%s","animated":false}]}'
`, staging, fmt.Sprintf("%x", sha256.Sum256([]byte("source"))), digest))
	gen, err := rustgen.Open(worker, rustgen.Contract{
		Protocol:        1,
		Release:         "test-release",
		Recipe:          "loomarr-rendition-v2",
		RequiredFormats: []string{"jpeg", "webp"},
		Animation:       true,
	})
	if err != nil {
		t.Fatalf("open worker: %v", err)
	}
	source := filepath.Join(t.TempDir(), "source.png")
	if err := os.WriteFile(source, []byte("source"), 0o600); err != nil {
		t.Fatal(err)
	}

	manifest, err := gen.Generate(context.Background(), rustgen.Request{
		RequestID: "request-1",
		Source: rustgen.Source{
			Path:           source,
			ExpectedSHA256: fmt.Sprintf("%x", sha256.Sum256([]byte("source"))),
		},
		StagingDir: staging,
		Targets: []rustgen.Target{{
			ID: "webp-w2", Format: "webp", Width: 2, Motion: "preserve",
		}},
		Budget: rustgen.Budget{
			MaxInputBytes: 8 << 20, MaxWidth: 16384, MaxHeight: 16384,
			MaxCanvasPixels: 40_000_000, MaxFrames: 600,
			MaxTotalFramePixels: 600_000_000, MaxDurationMS: 60_000,
			MaxOutputBytes: 64 << 20,
		},
	})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(manifest.Outputs) != 1 || manifest.Outputs[0].SHA256 != digest {
		t.Fatalf("manifest outputs = %#v", manifest.Outputs)
	}
}
