package releaseverify

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const publisherDigest = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestPublisherDoesNotPromoteAfterSigningFailure(t *testing.T) {
	result := runPublisher(t, "sign", false)
	if result.err == nil {
		t.Fatal("publisher succeeded after signing failure")
	}
	if strings.Contains(result.log, "PROMOTE") {
		t.Fatalf("publisher promoted after signing failure:\n%s", result.log)
	}
}

func TestPublisherDoesNotPromoteAfterIdentityVerificationFailure(t *testing.T) {
	result := runPublisher(t, "verify", false)
	if result.err == nil {
		t.Fatal("publisher succeeded after verification failure")
	}
	if !strings.Contains(result.log, "COSIGN sign") {
		t.Fatalf("publisher did not reach signing:\n%s", result.log)
	}
	if strings.Contains(result.log, "PROMOTE") {
		t.Fatalf("publisher promoted after verification failure:\n%s", result.log)
	}
}

func TestPublisherPromotesStableTagsOnlyAfterVerification(t *testing.T) {
	result := runPublisher(t, "", true)
	if result.err != nil {
		t.Fatalf("publisher failed: %v\n%s", result.err, result.output)
	}
	sign := strings.Index(result.log, "COSIGN sign")
	verify := strings.Index(result.log, "COSIGN verify")
	promote := strings.Index(result.log, "PROMOTE")
	if sign < 0 || verify < sign || promote < verify {
		t.Fatalf("unsafe publication order:\n%s", result.log)
	}
	if !strings.Contains(result.log, "--tag ghcr.io/loomarr/loomarr:0.1.0") ||
		!strings.Contains(result.log, "--tag ghcr.io/loomarr/loomarr:latest") {
		t.Fatalf("stable promotion did not name both public tags:\n%s", result.log)
	}
}

func TestPublisherDoesNotMoveLatestForPrerelease(t *testing.T) {
	result := runPublisher(t, "", false)
	if result.err != nil {
		t.Fatalf("publisher failed: %v\n%s", result.err, result.output)
	}
	if strings.Contains(result.log, "loomarr:latest") {
		t.Fatalf("prerelease moved latest:\n%s", result.log)
	}
}

// #661: nothing is signed or published unless both platform images carry an SBOM and a build
// provenance naming the tagged commit of this repository: that is what binds the image to the
// exact release source its notices point at.
func TestPublisherRefusesAnImageWithoutReleaseEvidence(t *testing.T) {
	for _, attestations := range []string{"no-sbom", "no-arm64-sbom", "empty-sbom", "no-provenance", "other-commit", "other-source"} {
		result := runPublisherWith(t, "", false, attestations)
		if result.err == nil {
			t.Errorf("%s: publisher succeeded:\n%s", attestations, result.output)
		}
		if strings.Contains(result.log, "COSIGN sign") || strings.Contains(result.log, "PROMOTE") {
			t.Errorf("%s: publisher signed or promoted an image without release evidence:\n%s", attestations, result.log)
		}
	}
}

type publisherResult struct {
	err    error
	log    string
	output string
}

func runPublisher(t *testing.T, fail string, stable bool) publisherResult {
	t.Helper()
	return runPublisherWith(t, fail, stable, "")
}

// publisherCommit is the tagged commit the fake provenance names unless told otherwise.
const publisherCommit = "0123456789abcdef0123456789abcdef01234567"

func runPublisherWith(t *testing.T, fail string, stable bool, attestations string) publisherResult {
	t.Helper()
	_, source, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(source), "..", ".."))
	temp := t.TempDir()
	bin := filepath.Join(temp, "bin")
	if err := os.Mkdir(bin, 0o700); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(temp, "calls.log")
	writeExecutable(t, filepath.Join(bin, "docker"), fakeDocker)
	writeExecutable(t, filepath.Join(bin, "cosign"), fakeCosign)

	tag := "v0.1.0-beta.1"
	publishLatest := "false"
	if stable {
		tag = "v0.1.0"
		publishLatest = "true"
	}
	command := exec.Command("sh", filepath.Join(root, "scripts", "publish-release-image.sh"))
	command.Dir = root
	command.Env = append(os.Environ(),
		"PATH="+bin+":"+os.Getenv("PATH"),
		"LOG_FILE="+logPath,
		"COSIGN_FAIL="+fail,
		"FAKE_ATTESTATIONS="+attestations,
		"GITHUB_SHA="+publisherCommit,
		"GITHUB_REPOSITORY=loomarr/loomarr",
		"IMAGE=ghcr.io/loomarr/loomarr",
		"DIGEST="+publisherDigest,
		"RELEASE_TAG="+tag,
		"PUBLISH_LATEST="+publishLatest,
		"COSIGN_CERTIFICATE_IDENTITY=https://github.com/loomarr/loomarr/.github/workflows/release.yml@refs/tags/"+tag,
		"COSIGN_CERTIFICATE_OIDC_ISSUER=https://token.actions.githubusercontent.com",
	)
	output, err := command.CombinedOutput()
	log, readErr := os.ReadFile(logPath)
	if readErr != nil && !os.IsNotExist(readErr) {
		t.Fatal(readErr)
	}
	return publisherResult{err: err, log: string(log), output: string(output)}
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
		t.Fatal(err)
	}
}

const fakeDocker = `#!/bin/sh
set -eu
echo "DOCKER $*" >> "$LOG_FILE"
if [ "$1 $2 $3" = "buildx imagetools inspect" ]; then
  if [ "${4:-}" = "--raw" ]; then
    cat <<'JSON'
{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json","manifests":[{"platform":{"os":"linux","architecture":"amd64"}},{"platform":{"os":"linux","architecture":"arm64"}},{"platform":{"os":"unknown","architecture":"unknown"},"annotations":{"vnd.docker.reference.type":"attestation-manifest"}}]}
JSON
    exit 0
  fi
  if [ "${5:-}" = "--format" ]; then
    # The shapes buildx prints for a published release index (v0.2.0-beta.8-rc.5), trimmed.
    rev=` + publisherCommit + `
    src=https://github.com/loomarr/loomarr
    packages='[{"name":"loomarr"}]'
    case "${FAKE_ATTESTATIONS:-}" in
      other-commit) rev=fedcba9876543210fedcba9876543210fedcba98 ;;
      other-source) src=https://github.com/someone/fork ;;
      empty-sbom) packages='[]' ;;
    esac
    sbom='{"SPDX":{"spdxVersion":"SPDX-2.3","packages":'"$packages"'}}'
    slsa='{"SLSA":{"buildDefinition":{"externalParameters":{"request":{"args":{"build-arg:COMMIT":"'"$rev"'","vcs:revision":"'"$rev"'","vcs:source":"'"$src"'"}}}}}}'
    case "$6:${FAKE_ATTESTATIONS:-}" in
      *SBOM*:no-sbom|*Provenance*:no-provenance) echo '{}' ;;
      *SBOM*:no-arm64-sbom) echo '{"linux/amd64":'"$sbom"'}' ;;
      *SBOM*) echo '{"linux/amd64":'"$sbom"',"linux/arm64":'"$sbom"'}' ;;
      *Provenance*) echo '{"linux/amd64":'"$slsa"',"linux/arm64":'"$slsa"'}' ;;
      *) echo "unexpected format: $6" >&2; exit 99 ;;
    esac
    exit 0
  fi
  ref=$4
  case "$ref" in
    *@sha256:*) printf 'Name: %s\nDigest: %s\n' "$ref" "` + publisherDigest + `"; exit 0 ;;
  esac
  if grep -q '^PROMOTE ' "$LOG_FILE" 2>/dev/null; then
    printf 'Name: %s\nDigest: %s\n' "$ref" "` + publisherDigest + `"
    exit 0
  fi
  echo "ERROR: $ref: not found" >&2
  exit 1
fi
if [ "$1 $2 $3" = "buildx imagetools create" ]; then
  echo "PROMOTE $*" >> "$LOG_FILE"
  exit 0
fi
echo "unexpected docker command: $*" >&2
exit 99
`

const fakeCosign = `#!/bin/sh
set -eu
echo "COSIGN $*" >> "$LOG_FILE"
if [ "${COSIGN_FAIL:-}" = "$1" ]; then
  exit 42
fi
exit 0
`
