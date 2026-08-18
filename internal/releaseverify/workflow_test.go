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
		{name: "image repository cannot be overridden by the job", workflow: strings.Replace(good, "  build:\n", "  build:\n    env:\n      IMAGE: ghcr.io/example/other\n", 1), wantErr: true},
		{name: "no second publication job", workflow: strings.Replace(good, "jobs:\n", "jobs:\n  bypass:\n    steps:\n      - run: docker push ghcr.io/example/other:latest\n", 1), wantErr: true},
		{name: "all releases serialize globally", workflow: strings.Replace(good, "group: release", "group: release-${{ github.ref_name }}", 1), wantErr: true},
		{name: "source and CI validation is required", workflow: strings.Replace(good, "      - name: Validate\n        shell: bash\n        env:\n          GH_TOKEN: ${{ github.token }}\n        run: ./scripts/validate-release-source.sh\n", "", 1), wantErr: true},
		{name: "unrecognized local action cannot publish", workflow: strings.Replace(good, "      - name: Protect", "      - uses: ./malicious-local-action\n      - name: Protect", 1), wantErr: true},
		{name: "checkout cannot switch away from the tag commit", workflow: strings.Replace(good, "      - uses: actions/checkout@"+testSHA+"\n", "      - uses: actions/checkout@"+testSHA+"\n        with:\n          ref: main\n", 1), wantErr: true},
		{name: "source validation cannot be skipped", workflow: strings.Replace(good, "      - name: Validate\n", "      - name: Validate\n        if: false\n", 1), wantErr: true},
		{name: "tag protection cannot continue after failure", workflow: strings.Replace(good, "      - name: Protect\n", "      - name: Protect\n        continue-on-error: true\n", 1), wantErr: true},
		{name: "publisher cannot replace bash", workflow: strings.Replace(good, "      - name: Publish\n", "      - name: Publish\n        shell: attacker {0}\n", 1), wantErr: true},
		{name: "publisher cannot change working directory", workflow: strings.Replace(good, "      - name: Publish\n", "      - name: Publish\n        working-directory: attacker\n", 1), wantErr: true},
		{name: "job cannot inherit run defaults", workflow: strings.Replace(good, "  build:\n", "  build:\n    defaults:\n      run:\n        working-directory: attacker\n", 1), wantErr: true},
		{name: "root cannot inject bash env", workflow: strings.Replace(good, "  IMAGE: ghcr.io/${{ github.repository }}\n", "  IMAGE: ghcr.io/${{ github.repository }}\n  BASH_ENV: attacker\n", 1), wantErr: true},
		{name: "publisher cannot inject bash env", workflow: strings.Replace(good, "          DIGEST: ${{ steps.merge.outputs.digest }}\n", "          DIGEST: ${{ steps.merge.outputs.digest }}\n          BASH_ENV: attacker\n", 1), wantErr: true},
		{name: "checkout cannot switch repository", workflow: strings.Replace(good, "      - uses: actions/checkout@"+testSHA+"\n", "      - uses: actions/checkout@"+testSHA+"\n        with:\n          repository: attacker/repo\n", 1), wantErr: true},
		{name: "build cannot select another Dockerfile", workflow: strings.Replace(good, "          context: .\n", "          context: .\n          file: attacker/Dockerfile\n", 1), wantErr: true},
		{name: "publication command whitespace cannot bypass", workflow: strings.Replace(good, "      - name: Protect", "      - name: Bypass\n        run: docker buildx imagetools  create ghcr.io/example/other:latest\n      - name: Protect", 1), wantErr: true},
		{name: "shell line continuation cannot bypass", workflow: strings.Replace(good, "      - name: Protect", "      - name: Bypass\n        run: |\n          docker buildx imagetools \\\n            create ghcr.io/example/other:latest\n      - name: Protect", 1), wantErr: true},
		{name: "mutable action", workflow: strings.Replace(good, "actions/checkout@"+testSHA, "actions/checkout@v7", 1), wantErr: true},
		{name: "publication order", workflow: swapPublicationOrder(good), wantErr: true},
		{name: "raw signing bypass", workflow: strings.Replace(good, "      - name: Protect", "      - name: Bypass\n        run: cosign sign ghcr.io/example/app@sha256:bad\n      - name: Protect", 1), wantErr: true},
		{name: "build job cannot sign", workflow: strings.Replace(good, "      - name: Upload digest", "      - name: Install cosign\n        uses: sigstore/cosign-installer@"+testSHA+"\n        with:\n          cosign-release: v2.6.5\n      - name: Upload digest", 1), wantErr: true},
		{name: "publish job cannot build", workflow: strings.Replace(good, "      - name: Protect", "      - name: Build\n        id: bypass\n        uses: docker/build-push-action@"+testSHA+"\n        with:\n          context: .\n          target: runtime\n          platforms: linux/amd64\n          push: true\n          outputs: type=image,name=${{ env.IMAGE }},push-by-digest=true,name-canonical=true,push=true\n          build-args: |\n            VERSION=${{ github.ref_name }}\n            COMMIT=${{ github.sha }}\n          provenance: mode=max\n          sbom: true\n          cache-from: type=gha,scope=release-bypass\n          cache-to: type=gha,scope=release-bypass,mode=max\n      - name: Protect", 1), wantErr: true},
		{name: "setup-qemu is rejected", workflow: strings.Replace(good, "      - uses: docker/setup-buildx-action@"+testSHA+"\n      - name: Login", "      - uses: docker/setup-qemu-action@"+testSHA+"\n      - uses: docker/setup-buildx-action@"+testSHA+"\n      - name: Login", 1), wantErr: true},
		{name: "merge step cannot be replaced", workflow: strings.Replace(good, "run: ./scripts/merge-release-digests.sh", "run: docker buildx imagetools create ghcr.io/example/other:latest", 1), wantErr: true},
		{name: "a third job fails", workflow: strings.Replace(good, "        run: ./scripts/publish-release-image.sh\n", "        run: ./scripts/publish-release-image.sh\n  bypass:\n    runs-on: ubuntu-latest\n    steps:\n      - run: docker push ghcr.io/example/other:latest\n", 1), wantErr: true},
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

// goodWorkflow mirrors .github/workflows/release.yml: a native per-arch build matrix that pushes
// digest-only images with no signing identity, followed by a publish job — the only job carrying
// id-token — that merges the two digests, signs the merged index, and promotes the public tags
// through the one audited helper. Third-party actions are pinned to testSHA instead of the real
// commit SHAs; everything else matches the shipped file.
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
  build:
    name: Build image (${{ matrix.platform }})
    runs-on: ${{ matrix.runner }}
    strategy:
      fail-fast: false
      matrix:
        include:
          - platform: linux/amd64
            runner: ubuntu-24.04
            arch: amd64
          - platform: linux/arm64
            runner: ubuntu-24.04-arm
            arch: arm64
    steps:
      - uses: actions/checkout@` + testSHA + `
      - name: Validate
        shell: bash
        env:
          GH_TOKEN: ${{ github.token }}
        run: ./scripts/validate-release-source.sh
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
          platforms: ${{ matrix.platform }}
          push: true
          outputs: type=image,name=${{ env.IMAGE }},push-by-digest=true,name-canonical=true,push=true
          build-args: |
            VERSION=${{ github.ref_name }}
            COMMIT=${{ github.sha }}
          provenance: mode=max
          sbom: true
          cache-from: type=gha,scope=release-${{ matrix.arch }}
          cache-to: type=gha,scope=release-${{ matrix.arch }},mode=max
      - name: Record digest
        shell: bash
        env:
          DIGEST: ${{ steps.build.outputs.digest }}
        run: |
          set -euo pipefail
          mkdir -p /tmp/digests
          printf '%s' "$DIGEST" > "/tmp/digests/${{ matrix.arch }}"
      - name: Upload digest
        uses: actions/upload-artifact@` + testSHA + `
        with:
          name: release-digest-${{ matrix.arch }}
          path: /tmp/digests/${{ matrix.arch }}
          if-no-files-found: error
          retention-days: 1
  publish:
    name: Sign and publish image
    needs: build
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@` + testSHA + `
      - name: Validate
        shell: bash
        env:
          GH_TOKEN: ${{ github.token }}
        run: ./scripts/validate-release-source.sh
      - uses: docker/setup-buildx-action@` + testSHA + `
      - name: Login
        uses: docker/login-action@` + testSHA + `
        with:
          registry: ${{ env.REGISTRY }}
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}
      - name: Download digests
        uses: actions/download-artifact@` + testSHA + `
        with:
          path: /tmp/digests
          pattern: release-digest-*
          merge-multiple: true
      - name: Protect
        shell: bash
        run: ./scripts/check-release-image-absence.sh "${IMAGE}:${GITHUB_REF_NAME#v}"
      - name: Merge
        id: merge
        shell: bash
        run: ./scripts/merge-release-digests.sh
      - name: Install cosign
        uses: sigstore/cosign-installer@` + testSHA + `
        with:
          cosign-release: v2.6.5
      - name: Publish
        shell: bash
        env:
          DIGEST: ${{ steps.merge.outputs.digest }}
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
          DIGEST: ${{ steps.merge.outputs.digest }}
          RELEASE_TAG: ${{ github.ref_name }}
          PUBLISH_LATEST: ${{ !contains(github.ref_name, '-') }}
          COSIGN_CERTIFICATE_IDENTITY: https://github.com/${{ github.repository }}/.github/workflows/release.yml@${{ github.ref }}
          COSIGN_CERTIFICATE_OIDC_ISSUER: https://token.actions.githubusercontent.com
        run: ./scripts/publish-release-image.sh
`
	return strings.Replace(workflow, cosign+publish, publish+cosign, 1)
}
