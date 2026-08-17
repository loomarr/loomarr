package releaseverify

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testSHA = "0123456789abcdef0123456789abcdef01234567"

func TestVerifyReleaseWorkflow(t *testing.T) {
	good := goodWorkflow()
	tests := []struct {
		name     string
		workflow string
		wantErr  bool
	}{
		{name: "digest sign verify promote", workflow: good},
		{name: "duplicate keys fail closed", workflow: strings.Replace(good, "          push: true\n", "          push: true\n          push: true\n", 1), wantErr: true},
		{name: "aliases fail closed", workflow: strings.Replace(good, "        with:\n          context:", "        with: &build\n          context:", 1), wantErr: true},
		{name: "custom tags fail closed", workflow: strings.Replace(good, "          push: true", "          push: !hidden true", 1), wantErr: true},
		{name: "public tags cannot hide in a block", workflow: strings.Replace(good, "          push: true\n", "          push: true\n          tags: |\n            ghcr.io/mantonx/loomarr:latest\n", 1), wantErr: true},
		{name: "image repository is fixed", workflow: strings.Replace(good, "IMAGE: ghcr.io/${{ github.repository }}", "IMAGE: ghcr.io/example/other", 1), wantErr: true},
		{name: "image repository cannot be overridden by the job", workflow: strings.Replace(good, "  images:\n", "  images:\n    env:\n      IMAGE: ghcr.io/example/other\n", 1), wantErr: true},
		{name: "no second publication job", workflow: strings.Replace(good, "jobs:\n", "jobs:\n  bypass:\n    steps:\n      - run: docker push ghcr.io/example/other:latest\n", 1), wantErr: true},
		{name: "all releases serialize globally", workflow: strings.Replace(good, "group: release", "group: release-${{ github.ref_name }}", 1), wantErr: true},
		{name: "source and CI validation is required", workflow: strings.Replace(good, "      - name: Validate\n        shell: bash\n        env:\n          GH_TOKEN: ${{ github.token }}\n        run: ./scripts/validate-release-source.sh\n", "", 1), wantErr: true},
		{name: "unrecognized local action cannot publish", workflow: strings.Replace(good, "      - name: Protect", "      - uses: ./malicious-local-action\n      - name: Protect", 1), wantErr: true},
		{name: "checkout cannot switch away from the tag commit", workflow: strings.Replace(good, "      - uses: actions/checkout@"+testSHA+"\n", "      - uses: actions/checkout@"+testSHA+"\n        with:\n          ref: main\n", 1), wantErr: true},
		{name: "source validation cannot be skipped", workflow: strings.Replace(good, "      - name: Validate\n", "      - name: Validate\n        if: false\n", 1), wantErr: true},
		{name: "tag protection cannot continue after failure", workflow: strings.Replace(good, "      - name: Protect\n", "      - name: Protect\n        continue-on-error: true\n", 1), wantErr: true},
		{name: "publisher cannot replace bash", workflow: strings.Replace(good, "      - name: Publish\n", "      - name: Publish\n        shell: attacker {0}\n", 1), wantErr: true},
		{name: "publisher cannot change working directory", workflow: strings.Replace(good, "      - name: Publish\n", "      - name: Publish\n        working-directory: attacker\n", 1), wantErr: true},
		{name: "job cannot inherit run defaults", workflow: strings.Replace(good, "  images:\n", "  images:\n    defaults:\n      run:\n        working-directory: attacker\n", 1), wantErr: true},
		{name: "root cannot inject bash env", workflow: strings.Replace(good, "  IMAGE: ghcr.io/${{ github.repository }}\n", "  IMAGE: ghcr.io/${{ github.repository }}\n  BASH_ENV: attacker\n", 1), wantErr: true},
		{name: "publisher cannot inject bash env", workflow: strings.Replace(good, "          DIGEST: ${{ steps.build.outputs.digest }}\n", "          DIGEST: ${{ steps.build.outputs.digest }}\n          BASH_ENV: attacker\n", 1), wantErr: true},
		{name: "checkout cannot switch repository", workflow: strings.Replace(good, "      - uses: actions/checkout@"+testSHA+"\n", "      - uses: actions/checkout@"+testSHA+"\n        with:\n          repository: attacker/repo\n", 1), wantErr: true},
		{name: "build cannot select another Dockerfile", workflow: strings.Replace(good, "          context: .\n", "          context: .\n          file: attacker/Dockerfile\n", 1), wantErr: true},
		{name: "publication command whitespace cannot bypass", workflow: strings.Replace(good, "      - name: Protect", "      - name: Bypass\n        run: docker buildx imagetools  create ghcr.io/example/other:latest\n      - name: Protect", 1), wantErr: true},
		{name: "shell line continuation cannot bypass", workflow: strings.Replace(good, "      - name: Protect", "      - name: Bypass\n        run: |\n          docker buildx imagetools \\\n            create ghcr.io/example/other:latest\n      - name: Protect", 1), wantErr: true},
		{name: "mutable action", workflow: strings.Replace(good, "actions/checkout@"+testSHA, "actions/checkout@v7", 1), wantErr: true},
		{name: "publication order", workflow: swapPublicationOrder(good), wantErr: true},
		{name: "raw signing bypass", workflow: strings.Replace(good, "      - name: Protect", "      - name: Bypass\n        run: cosign sign ghcr.io/example/app@sha256:bad\n      - name: Protect", 1), wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "release.yml")
			if err := os.WriteFile(path, []byte(tc.workflow), 0o600); err != nil {
				t.Fatal(err)
			}
			err := VerifyReleaseWorkflow(path)
			if tc.wantErr && err == nil {
				t.Fatal("VerifyReleaseWorkflow accepted an unsafe workflow")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("VerifyReleaseWorkflow: %v", err)
			}
		})
	}
}

func TestVerifyActionsRequiresCommitPins(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ci.yml")
	if err := os.WriteFile(path, []byte("jobs:\n  test:\n    steps:\n      - uses: actions/checkout@"+testSHA+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := VerifyActions(dir); err != nil {
		t.Fatalf("pinned action: %v", err)
	}
	if err := os.WriteFile(path, []byte("jobs:\n  test:\n    steps:\n      - uses: actions/checkout@v7\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := VerifyActions(dir); err == nil {
		t.Fatal("VerifyActions accepted a mutable major-version ref")
	}
}

func goodWorkflow() string {
	return `name: Release
on:
  push:
    tags: ["v*"]
permissions:
  contents: read
  actions: read
  packages: write
  id-token: write
env:
  REGISTRY: ghcr.io
  IMAGE: ghcr.io/${{ github.repository }}
concurrency:
  group: release
  cancel-in-progress: false
jobs:
  images:
    name: Build & publish image
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@` + testSHA + `
      - name: Validate
        shell: bash
        env:
          GH_TOKEN: ${{ github.token }}
        run: ./scripts/validate-release-source.sh
      - name: Protect
        shell: bash
        run: ./scripts/check-release-image-absence.sh "${IMAGE}:${GITHUB_REF_NAME#v}"
      - uses: docker/setup-qemu-action@` + testSHA + `
      - uses: docker/setup-buildx-action@` + testSHA + `
      - name: Login
        uses: docker/login-action@` + testSHA + `
        with:
          registry: ${{ env.REGISTRY }}
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}
      - name: Build
        id: build
        uses: docker/build-push-action@` + testSHA + `
        with:
          context: .
          target: runtime
          platforms: linux/amd64,linux/arm64
          push: true
          outputs: type=image,name=${{ env.IMAGE }},push-by-digest=true,name-canonical=true,push=true
          build-args: |
            VERSION=${{ github.ref_name }}
            COMMIT=${{ github.sha }}
          provenance: mode=max
          sbom: true
          cache-from: type=gha,scope=release
          cache-to: type=gha,scope=release,mode=max
      - name: Install cosign
        uses: sigstore/cosign-installer@` + testSHA + `
        with:
          cosign-release: v2.6.5
      - name: Publish
        shell: bash
        env:
          DIGEST: ${{ steps.build.outputs.digest }}
          RELEASE_TAG: ${{ github.ref_name }}
          PUBLISH_LATEST: ${{ !contains(github.ref_name, '-') }}
          COSIGN_CERTIFICATE_IDENTITY: https://github.com/${{ github.repository }}/.github/workflows/release.yml@${{ github.ref }}
          COSIGN_CERTIFICATE_OIDC_ISSUER: https://token.actions.githubusercontent.com
        run: ./scripts/publish-release-image.sh
`
}

func swapPublicationOrder(workflow string) string {
	cosign := `      - name: Install cosign
        uses: sigstore/cosign-installer@` + testSHA + `
        with:
          cosign-release: v2.6.5
`
	publish := `      - name: Publish
        shell: bash
        env:
          DIGEST: ${{ steps.build.outputs.digest }}
          RELEASE_TAG: ${{ github.ref_name }}
          PUBLISH_LATEST: ${{ !contains(github.ref_name, '-') }}
          COSIGN_CERTIFICATE_IDENTITY: https://github.com/${{ github.repository }}/.github/workflows/release.yml@${{ github.ref }}
          COSIGN_CERTIFICATE_OIDC_ISSUER: https://token.actions.githubusercontent.com
        run: ./scripts/publish-release-image.sh
`
	return strings.Replace(workflow, cosign+publish, publish+cosign, 1)
}
