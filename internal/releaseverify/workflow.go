// Package releaseverify validates the repository's release publication policy.
package releaseverify

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

var actionPin = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+(?:/[A-Za-z0-9_.-]+)?@[0-9a-f]{40}$`)

func releaseRunAuthorities() map[string]struct{} {
	return map[string]struct{}{
		"./scripts/validate-release-source.sh":                                     {},
		`./scripts/check-release-image-absence.sh "${IMAGE}:${GITHUB_REF_NAME#v}"`: {},
		"./scripts/merge-release-digests.sh":                                       {},
		"./scripts/publish-release-image.sh":                                       {},
	}
}

// digestRecordRun matches the build leg's own step that writes its digest to
// /tmp/digests/<arch> for the publish job to read back. It is intentionally NOT a
// fixed string like the other audited helpers above: the shell body legitimately
// varies only in the constant `set -euo pipefail` / `mkdir` / `printf` shape, so it
// is matched structurally instead of being added to allowedReleaseRuns verbatim.
var digestRecordRun = regexp.MustCompile(`^set -euo pipefail\nmkdir -p /tmp/digests\nprintf '%s' "\$DIGEST" > "/tmp/digests/\$\{\{ matrix\.arch \}\}"\n?$`)

// VerifyActions rejects mutable third-party action references in every workflow.
func VerifyActions(workflowsDir string) error {
	var paths []string
	err := filepath.WalkDir(workflowsDir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("workflow path must not be a symlink: %s", path)
		}
		if entry.IsDir() {
			return nil
		}
		if ext := filepath.Ext(path); ext == ".yml" || ext == ".yaml" {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		return fmt.Errorf("no workflow YAML files found in %s", workflowsDir)
	}
	sort.Strings(paths)
	for _, path := range paths {
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		root, parseErr := parseYAML(data)
		if parseErr != nil {
			return fmt.Errorf("%s: %w", path, parseErr)
		}
		if pinErr := verifyUses(root); pinErr != nil {
			return fmt.Errorf("%s: %w", path, pinErr)
		}
	}
	return nil
}

// VerifyReleaseWorkflow validates digest-only build, identity-bound signing, and
// post-verification promotion through the one audited publication helper.
func VerifyReleaseWorkflow(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	root, err := parseYAML(data)
	if err != nil {
		return err
	}
	if err := verifyUses(root); err != nil {
		return err
	}
	if err := verifyOnlyKeys(root, "release workflow", "name", "on", "concurrency", "permissions", "env", "jobs"); err != nil {
		return err
	}
	if scalarValue(root, "name") != "Release" {
		return errors.New("release workflow name must be Release")
	}
	if err := verifyReleaseTrigger(root); err != nil {
		return err
	}
	if err := verifyReleasePermissions(root); err != nil {
		return err
	}
	if err := verifyReleaseConcurrency(root); err != nil {
		return err
	}
	if err := verifyReleaseEnvironment(root); err != nil {
		return err
	}
	jobs, err := requiredMap(root, "jobs")
	if err != nil {
		return err
	}
	// Exactly two jobs, in this order, and no others. This is the load-bearing "no bypass job"
	// invariant: the build legs never carry id-token, so a third job cannot smuggle in a second,
	// unaudited path to a public tag or a signature.
	if len(jobs.Content) != 4 || jobs.Content[0].Value != "build" || jobs.Content[2].Value != "publish" {
		return errors.New("release workflow must contain only the audited build and publish jobs, in that order")
	}
	build, err := requiredMap(jobs, "build")
	if err != nil {
		return err
	}
	if err := verifyReleaseBuildJob(build); err != nil {
		return err
	}
	publish, err := requiredMap(jobs, "publish")
	if err != nil {
		return err
	}
	if err := verifyReleasePublishJob(publish); err != nil {
		return err
	}
	return nil
}

// verifyReleaseBuildJob validates the per-architecture matrix leg. Each leg pushes exactly one
// digest-only image for its own platform and carries no signing identity (no id-token, no cosign,
// no public tag) — only the publish job, which needs this job, is trusted to promote anything.
func verifyReleaseBuildJob(build *yaml.Node) error {
	if err := verifyOnlyKeys(build, "release build job", "name", "runs-on", "strategy", "steps"); err != nil {
		return err
	}
	if scalarValue(build, "runs-on") != "${{ matrix.runner }}" {
		return errors.New("release build job must run on the per-arch matrix runner")
	}
	if err := verifyReleaseBuildMatrix(build); err != nil {
		return err
	}
	steps, err := requiredSequence(build, "steps")
	if err != nil {
		return err
	}

	checkoutIndex, validateIndex, buildIndex, recordIndex, uploadIndex := -1, -1, -1, -1, -1
	checkoutCount, validateCount, buildCount, recordCount, uploadCount := 0, 0, 0, 0, 0
	var actionSequence []string
	for index, step := range steps.Content {
		if step.Kind != yaml.MappingNode {
			return fmt.Errorf("build step %d must be a mapping", index+1)
		}
		if _, ok := mappingValue(step, "if"); ok {
			return fmt.Errorf("release build step %d must not be conditional", index+1)
		}
		if _, ok := mappingValue(step, "continue-on-error"); ok {
			return fmt.Errorf("release build step %d must fail closed", index+1)
		}
		uses := scalarValue(step, "uses")
		action := strings.SplitN(uses, "@", 2)[0]
		if action != "" {
			actionSequence = append(actionSequence, action)
		}
		run := scalarValue(step, "run")
		if (action == "") == (strings.TrimSpace(run) == "") {
			return fmt.Errorf("release build step %d must contain exactly one of uses or run", index+1)
		}
		switch action {
		case "actions/checkout":
			if err := verifyOnlyKeys(step, "checkout step", "uses"); err != nil {
				return err
			}
			checkoutCount++
			checkoutIndex = index
		case "docker/metadata-action":
			return errors.New("release build job must not derive public tags before signing")
		case "docker/setup-qemu-action":
			// Native per-arch runners replace QEMU emulation entirely. A build leg that still
			// sets up QEMU is a regression back to emulated cross-builds, not an addition.
			return errors.New("release build job must not use QEMU; each leg runs on its native architecture")
		case "docker/setup-buildx-action":
			if err := verifyOnlyKeys(step, "Buildx setup step", "uses"); err != nil {
				return err
			}
		case "docker/login-action":
			if err := verifyRegistryLogin(step); err != nil {
				return err
			}
		case "docker/build-push-action":
			if err := verifyOnlyKeys(step, "release image build step", "name", "id", "uses", "with"); err != nil {
				return err
			}
			buildCount++
			buildIndex = index
			if err := verifyDigestOnlyBuild(step); err != nil {
				return err
			}
		case "sigstore/cosign-installer":
			// Signing belongs solely to the publish job, which alone carries id-token. A build
			// leg that installs cosign is reaching for a signing identity it must not have.
			return errors.New("release build job must not install Cosign; only the publish job signs")
		case "actions/upload-artifact":
			if err := verifyOnlyKeys(step, "digest upload step", "name", "uses", "with"); err != nil {
				return err
			}
			uploadCount++
			uploadIndex = index
			if err := verifyDigestUpload(step); err != nil {
				return err
			}
		}
		trimmedRun := strings.TrimSpace(run)
		isDigestRecord := digestRecordRun.MatchString(run)
		if trimmedRun != "" && !isDigestRecord {
			if err := verifyReleaseRunStep(step, trimmedRun); err != nil {
				return err
			}
		}
		if trimmedRun == "./scripts/validate-release-source.sh" {
			validateCount++
			validateIndex = index
		}
		if isDigestRecord {
			recordCount++
			recordIndex = index
			if err := verifyDigestRecordStep(step); err != nil {
				return err
			}
		}
		if trimmedRun == "./scripts/publish-release-image.sh" {
			return errors.New("release build job must not call the publication helper; only the publish job promotes")
		}
	}
	expectedActions := []string{
		"actions/checkout",
		"docker/setup-buildx-action",
		"docker/login-action",
		"docker/build-push-action",
		"actions/upload-artifact",
	}
	if len(actionSequence) != len(expectedActions) {
		return fmt.Errorf("release build job action sequence = %v, want %v", actionSequence, expectedActions)
	}
	for index := range expectedActions {
		if actionSequence[index] != expectedActions[index] {
			return fmt.Errorf("release build job action sequence = %v, want %v", actionSequence, expectedActions)
		}
	}
	if buildCount != 1 {
		return fmt.Errorf("release build job must have exactly one image build, found %d", buildCount)
	}
	if checkoutCount != 1 {
		return fmt.Errorf("release build job must check out the triggering commit exactly once, found %d", checkoutCount)
	}
	if validateCount != 1 {
		return fmt.Errorf("release build job must validate source and CI exactly once, found %d", validateCount)
	}
	if recordCount != 1 {
		return fmt.Errorf("release build job must record its digest exactly once, found %d", recordCount)
	}
	if uploadCount != 1 {
		return fmt.Errorf("release build job must upload its digest exactly once, found %d", uploadCount)
	}
	if checkoutIndex >= validateIndex || validateIndex >= buildIndex || buildIndex >= recordIndex || recordIndex >= uploadIndex {
		return errors.New("release build order must be checkout, source validation, digest-only build, digest record, then digest upload")
	}
	return nil
}

// verifyReleaseBuildMatrix pins the matrix to exactly the two audited native legs: no extra
// architecture can be smuggled in, and neither leg can be pointed at an emulated runner.
func verifyReleaseBuildMatrix(build *yaml.Node) error {
	strategy, err := requiredMap(build, "strategy")
	if err != nil {
		return err
	}
	if err := verifyOnlyKeys(strategy, "release build strategy", "fail-fast", "matrix"); err != nil {
		return err
	}
	if scalarValue(strategy, "fail-fast") != "false" {
		return errors.New("release build strategy must not fail-fast: one arch's failure must not hide the other's result")
	}
	matrix, err := requiredMap(strategy, "matrix")
	if err != nil {
		return err
	}
	if err := verifyOnlyKeys(matrix, "release build matrix", "include"); err != nil {
		return err
	}
	include, err := requiredSequence(matrix, "include")
	if err != nil {
		return err
	}
	required := []map[string]string{
		{"platform": "linux/amd64", "runner": "ubuntu-24.04", "arch": "amd64"},
		{"platform": "linux/arm64", "runner": "ubuntu-24.04-arm", "arch": "arm64"},
	}
	if len(include.Content) != len(required) {
		return fmt.Errorf("release build matrix must contain exactly %d native legs, found %d", len(required), len(include.Content))
	}
	for index, want := range required {
		leg := include.Content[index]
		if err := verifyOnlyKeys(leg, "release build matrix leg", "platform", "runner", "arch"); err != nil {
			return err
		}
		for key, wantValue := range want {
			if got := scalarValue(leg, key); got != wantValue {
				return fmt.Errorf("release build matrix leg %d %s = %q, want %q", index, key, got, wantValue)
			}
		}
	}
	return nil
}

// verifyReleasePublishJob validates the sole job that carries the signing identity: it needs the
// build job's two digests, merges them into one multi-arch index, signs and identity-verifies
// that index, and only then promotes the public SemVer/latest tags — all through the one audited
// helper script.
func verifyReleasePublishJob(publish *yaml.Node) error {
	if err := verifyOnlyKeys(publish, "release publish job", "name", "needs", "runs-on", "steps"); err != nil {
		return err
	}
	if scalarValue(publish, "needs") != "build" {
		return errors.New("release publish job must need the build job's digests")
	}
	if scalarValue(publish, "runs-on") != "ubuntu-latest" {
		return errors.New("release publish job must run on ubuntu-latest")
	}
	steps, err := requiredSequence(publish, "steps")
	if err != nil {
		return err
	}

	checkoutIndex, validateIndex, downloadIndex, protectIndex, mergeIndex, cosignIndex, publishIndex := -1, -1, -1, -1, -1, -1, -1
	checkoutCount, validateCount, downloadCount, protectCount, mergeCount, cosignCount, publishCount := 0, 0, 0, 0, 0, 0, 0
	var actionSequence []string
	for index, step := range steps.Content {
		if step.Kind != yaml.MappingNode {
			return fmt.Errorf("publish step %d must be a mapping", index+1)
		}
		if _, ok := mappingValue(step, "if"); ok {
			return fmt.Errorf("release publish step %d must not be conditional", index+1)
		}
		if _, ok := mappingValue(step, "continue-on-error"); ok {
			return fmt.Errorf("release publish step %d must fail closed", index+1)
		}
		uses := scalarValue(step, "uses")
		action := strings.SplitN(uses, "@", 2)[0]
		if action != "" {
			actionSequence = append(actionSequence, action)
		}
		run := scalarValue(step, "run")
		if (action == "") == (strings.TrimSpace(run) == "") {
			return fmt.Errorf("release publish step %d must contain exactly one of uses or run", index+1)
		}
		switch action {
		case "actions/checkout":
			if err := verifyOnlyKeys(step, "checkout step", "uses"); err != nil {
				return err
			}
			checkoutCount++
			checkoutIndex = index
		case "docker/metadata-action":
			return errors.New("release publish job must not derive public tags before signing")
		case "docker/setup-qemu-action":
			return errors.New("release publish job must not use QEMU")
		case "docker/setup-buildx-action":
			if err := verifyOnlyKeys(step, "Buildx setup step", "uses"); err != nil {
				return err
			}
		case "docker/login-action":
			if err := verifyRegistryLogin(step); err != nil {
				return err
			}
		case "docker/build-push-action":
			return errors.New("release publish job must not build; it only merges the build job's digests")
		case "actions/download-artifact":
			if err := verifyOnlyKeys(step, "digest download step", "name", "uses", "with"); err != nil {
				return err
			}
			downloadCount++
			downloadIndex = index
			if err := verifyDigestDownload(step); err != nil {
				return err
			}
		case "sigstore/cosign-installer":
			if err := verifyOnlyKeys(step, "Cosign installer step", "name", "uses", "with"); err != nil {
				return err
			}
			cosignCount++
			cosignIndex = index
			with, mapErr := requiredMap(step, "with")
			if mapErr != nil || len(with.Content) != 2 || scalarValue(with, "cosign-release") != "v2.6.5" {
				return errors.New("cosign installer must select exactly v2.6.5")
			}
		}
		trimmedRun := strings.TrimSpace(run)
		if trimmedRun != "" {
			if err := verifyReleaseRunStep(step, trimmedRun); err != nil {
				return err
			}
		}
		if trimmedRun == `./scripts/check-release-image-absence.sh "${IMAGE}:${GITHUB_REF_NAME#v}"` {
			protectCount++
			protectIndex = index
		}
		if trimmedRun == "./scripts/validate-release-source.sh" {
			validateCount++
			validateIndex = index
		}
		if trimmedRun == "./scripts/merge-release-digests.sh" {
			mergeCount++
			mergeIndex = index
			if scalarValue(step, "id") != "merge" {
				return errors.New("digest merge step must have id merge")
			}
		}
		if trimmedRun == "./scripts/publish-release-image.sh" {
			publishCount++
			publishIndex = index
			if err := verifyPublisherEnvironment(step); err != nil {
				return err
			}
			continue
		}
	}
	expectedActions := []string{
		"actions/checkout",
		"docker/setup-buildx-action",
		"docker/login-action",
		"actions/download-artifact",
		"sigstore/cosign-installer",
	}
	if len(actionSequence) != len(expectedActions) {
		return fmt.Errorf("release publish job action sequence = %v, want %v", actionSequence, expectedActions)
	}
	for index := range expectedActions {
		if actionSequence[index] != expectedActions[index] {
			return fmt.Errorf("release publish job action sequence = %v, want %v", actionSequence, expectedActions)
		}
	}
	if checkoutCount != 1 {
		return fmt.Errorf("release publish job must check out the triggering commit exactly once, found %d", checkoutCount)
	}
	if validateCount != 1 {
		return fmt.Errorf("release publish job must validate source and CI exactly once, found %d", validateCount)
	}
	if downloadCount != 1 {
		return fmt.Errorf("release publish job must download the per-arch digests exactly once, found %d", downloadCount)
	}
	if protectCount != 1 {
		return fmt.Errorf("release publish job must protect the immutable SemVer tag exactly once, found %d", protectCount)
	}
	if mergeCount != 1 {
		return fmt.Errorf("release publish job must merge the per-arch digests exactly once, found %d", mergeCount)
	}
	if cosignCount != 1 {
		return fmt.Errorf("release publish job must install Cosign exactly once, found %d", cosignCount)
	}
	if publishCount != 1 {
		return fmt.Errorf("release publish job must call the publication helper exactly once, found %d", publishCount)
	}
	if checkoutIndex >= validateIndex || validateIndex >= downloadIndex || downloadIndex >= protectIndex ||
		protectIndex >= mergeIndex || mergeIndex >= cosignIndex || cosignIndex >= publishIndex {
		return errors.New("release publish order must be checkout, source validation, digest download, tag protection, digest merge, Cosign install, then sign/verify/promotion")
	}
	return nil
}

func verifyReleaseConcurrency(root *yaml.Node) error {
	concurrency, err := requiredMap(root, "concurrency")
	if err != nil {
		return err
	}
	if err := verifyOnlyKeys(concurrency, "release concurrency", "group", "cancel-in-progress"); err != nil {
		return err
	}
	if scalarValue(concurrency, "group") != "release" {
		return errors.New("release workflow must serialize every tag in one global release group")
	}
	if scalarValue(concurrency, "cancel-in-progress") != "false" {
		return errors.New("release workflow must queue rather than cancel publication")
	}
	return nil
}

func verifyReleaseTrigger(root *yaml.Node) error {
	trigger, err := requiredMap(root, "on")
	if err != nil {
		return err
	}
	if err := verifyOnlyKeys(trigger, "release trigger", "push"); err != nil {
		return err
	}
	push, err := requiredMap(trigger, "push")
	if err != nil {
		return err
	}
	if err := verifyOnlyKeys(push, "release push trigger", "tags"); err != nil {
		return err
	}
	tags, err := requiredSequence(push, "tags")
	if err != nil || len(tags.Content) != 1 || tags.Content[0].Kind != yaml.ScalarNode || tags.Content[0].Value != "v*" {
		return errors.New("release workflow must trigger only on v* tags")
	}
	return nil
}

func verifyReleasePermissions(root *yaml.Node) error {
	permissions, err := requiredMap(root, "permissions")
	if err != nil {
		return err
	}
	required := map[string]string{
		"contents": "read",
		"actions":  "read",
		"packages": "write",
		"id-token": "write",
	}
	if len(permissions.Content) != len(required)*2 {
		return errors.New("release permissions must contain only the audited four grants")
	}
	for key, want := range required {
		if got := scalarValue(permissions, key); got != want {
			return fmt.Errorf("release permission %s = %q, want %q", key, got, want)
		}
	}
	return nil
}

func verifyReleaseEnvironment(root *yaml.Node) error {
	env, err := requiredMap(root, "env")
	if err != nil {
		return err
	}
	if len(env.Content) != 4 {
		return errors.New("release root environment must contain only REGISTRY and IMAGE")
	}
	required := map[string]string{
		"REGISTRY": "ghcr.io",
		"IMAGE":    "ghcr.io/${{ github.repository }}",
	}
	for key, want := range required {
		if got := scalarValue(env, key); got != want {
			return fmt.Errorf("release environment %s = %q, want %q", key, got, want)
		}
	}
	return nil
}

func verifyRegistryLogin(step *yaml.Node) error {
	if err := verifyOnlyKeys(step, "registry login step", "name", "uses", "with"); err != nil {
		return err
	}
	with, err := requiredMap(step, "with")
	if err != nil {
		return err
	}
	required := map[string]string{
		"registry": "${{ env.REGISTRY }}",
		"username": "${{ github.actor }}",
		"password": "${{ secrets.GITHUB_TOKEN }}",
	}
	if len(with.Content) != len(required)*2 {
		return errors.New("registry login contains unaudited inputs")
	}
	for key, want := range required {
		if got := scalarValue(with, key); got != want {
			return fmt.Errorf("registry login %s = %q, want %q", key, got, want)
		}
	}
	return nil
}

func verifyDigestOnlyBuild(step *yaml.Node) error {
	if scalarValue(step, "id") != "build" {
		return errors.New("release image build must have id build")
	}
	with, err := requiredMap(step, "with")
	if err != nil {
		return err
	}
	if _, ok := mappingValue(with, "tags"); ok {
		return errors.New("release image build must not publish public tags")
	}
	required := map[string]string{
		"context":    ".",
		"target":     "runtime",
		"platforms":  "${{ matrix.platform }}",
		"push":       "true",
		"outputs":    "type=image,name=${{ env.IMAGE }},push-by-digest=true,name-canonical=true,push=true",
		"build-args": "VERSION=${{ github.ref_name }}\nCOMMIT=${{ github.sha }}",
		"provenance": "mode=max",
		"sbom":       "true",
		"cache-from": "type=gha,scope=release-${{ matrix.arch }}",
		"cache-to":   "type=gha,scope=release-${{ matrix.arch }},mode=max",
	}
	if len(with.Content) != len(required)*2 {
		return errors.New("release image build contains unaudited inputs")
	}
	for key, want := range required {
		if got := strings.TrimSpace(scalarValue(with, key)); got != want {
			return fmt.Errorf("release image build %s = %q, want %q", key, got, want)
		}
	}
	return nil
}

// verifyDigestUpload pins the build leg's artifact upload so the digest handed to the publish
// job cannot be swapped for an attacker's path, given a foreign name the publish job's pattern
// match would miss, or silently dropped on a transient failure (if-no-files-found: error).
func verifyDigestUpload(step *yaml.Node) error {
	with, err := requiredMap(step, "with")
	if err != nil {
		return err
	}
	required := map[string]string{
		"name":              "release-digest-${{ matrix.arch }}",
		"path":              "/tmp/digests/${{ matrix.arch }}",
		"if-no-files-found": "error",
		"retention-days":    "1",
	}
	if len(with.Content) != len(required)*2 {
		return errors.New("digest upload contains unaudited inputs")
	}
	for key, want := range required {
		if got := scalarValue(with, key); got != want {
			return fmt.Errorf("digest upload %s = %q, want %q", key, got, want)
		}
	}
	return nil
}

// verifyDigestDownload pins the publish job's artifact download to exactly the two per-arch
// digest artifacts the build job produced, merged into one directory the merge helper reads.
func verifyDigestDownload(step *yaml.Node) error {
	with, err := requiredMap(step, "with")
	if err != nil {
		return err
	}
	required := map[string]string{
		"path":           "/tmp/digests",
		"pattern":        "release-digest-*",
		"merge-multiple": "true",
	}
	if len(with.Content) != len(required)*2 {
		return errors.New("digest download contains unaudited inputs")
	}
	for key, want := range required {
		if got := scalarValue(with, key); got != want {
			return fmt.Errorf("digest download %s = %q, want %q", key, got, want)
		}
	}
	return nil
}

// verifyDigestRecordStep locks down the build leg's own step that writes its digest to disk for
// upload. Only name/shell/env/run may appear, the shell must be exact bash, and its only
// environment input is the build step's own digest output — nothing an attacker controls.
func verifyDigestRecordStep(step *yaml.Node) error {
	if err := verifyOnlyKeys(step, "digest record step", "name", "shell", "env", "run"); err != nil {
		return err
	}
	if scalarValue(step, "shell") != "bash" {
		return errors.New("digest record step must use exact bash semantics")
	}
	env, err := requiredMap(step, "env")
	if err != nil || len(env.Content) != 2 || scalarValue(env, "DIGEST") != "${{ steps.build.outputs.digest }}" {
		return errors.New("digest record environment must contain only DIGEST from the build step's output")
	}
	return nil
}

func verifyReleaseRunStep(step *yaml.Node, run string) error {
	if _, allowed := releaseRunAuthorities()[run]; !allowed {
		return fmt.Errorf("release workflow run step is not an audited helper: %q", run)
	}
	if scalarValue(step, "shell") != "bash" {
		return errors.New("release helper must use exact bash semantics")
	}
	switch run {
	case "./scripts/validate-release-source.sh":
		if err := verifyOnlyKeys(step, "source validation step", "name", "shell", "env", "run"); err != nil {
			return err
		}
		env, err := requiredMap(step, "env")
		if err != nil || len(env.Content) != 2 || scalarValue(env, "GH_TOKEN") != "${{ github.token }}" {
			return errors.New("source validation environment must contain only GH_TOKEN from github.token")
		}
	case `./scripts/check-release-image-absence.sh "${IMAGE}:${GITHUB_REF_NAME#v}"`:
		if err := verifyOnlyKeys(step, "tag protection step", "name", "shell", "run"); err != nil {
			return err
		}
	case "./scripts/merge-release-digests.sh":
		if err := verifyOnlyKeys(step, "digest merge step", "name", "id", "shell", "run"); err != nil {
			return err
		}
	case "./scripts/publish-release-image.sh":
		if err := verifyOnlyKeys(step, "publication step", "name", "shell", "env", "run"); err != nil {
			return err
		}
	}
	return nil
}

func verifyPublisherEnvironment(step *yaml.Node) error {
	env, err := requiredMap(step, "env")
	if err != nil {
		return err
	}
	required := map[string]string{
		// The publisher signs and promotes the MERGED multi-arch index, not either build leg's
		// single-arch digest — so this must read the merge step's output, not the build step's.
		"DIGEST":                         "${{ steps.merge.outputs.digest }}",
		"RELEASE_TAG":                    "${{ github.ref_name }}",
		"PUBLISH_LATEST":                 "${{ !contains(github.ref_name, '-') }}",
		"COSIGN_CERTIFICATE_IDENTITY":    "https://github.com/${{ github.repository }}/.github/workflows/release.yml@${{ github.ref }}",
		"COSIGN_CERTIFICATE_OIDC_ISSUER": "https://token.actions.githubusercontent.com",
	}
	for key, want := range required {
		if got := scalarValue(env, key); got != want {
			return fmt.Errorf("publication helper %s = %q, want %q", key, got, want)
		}
	}
	if len(env.Content) != len(required)*2 {
		return errors.New("publication helper environment contains unaudited keys")
	}
	return nil
}

func verifyOnlyKeys(node *yaml.Node, context string, allowed ...string) error {
	wanted := make(map[string]struct{}, len(allowed))
	for _, key := range allowed {
		wanted[key] = struct{}{}
	}
	return verifyOnlyKeySet(node, context, wanted)
}

func verifyOnlyKeySet(node *yaml.Node, context string, wanted map[string]struct{}) error {
	if node == nil || node.Kind != yaml.MappingNode || len(node.Content)%2 != 0 {
		return fmt.Errorf("%s must be a mapping", context)
	}
	for index := 0; index < len(node.Content); index += 2 {
		keyNode := node.Content[index]
		if keyNode.Kind != yaml.ScalarNode || keyNode.Value == "" {
			return fmt.Errorf("%s contains a malformed key", context)
		}
		key := keyNode.Value
		if _, ok := wanted[key]; !ok {
			return fmt.Errorf("%s contains unaudited key %q", context, key)
		}
	}
	return nil
}

func parseYAML(data []byte) (*yaml.Node, error) {
	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("parse YAML: %w", err)
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, errors.New("workflow must contain exactly one YAML document")
		}
		return nil, fmt.Errorf("parse trailing YAML: %w", err)
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New("workflow root must be a mapping")
	}
	if err := validateNode(document.Content[0]); err != nil {
		return nil, err
	}
	return document.Content[0], nil
}

func validateNode(node *yaml.Node) error {
	if node.Kind == yaml.AliasNode || node.Anchor != "" {
		return errors.New("YAML aliases and anchors are forbidden in release policy files")
	}
	allowedTag := node.Tag == "" || strings.HasPrefix(node.Tag, "tag:yaml.org,2002:") || strings.HasPrefix(node.Tag, "!!")
	if !allowedTag {
		return fmt.Errorf("custom YAML tag %q is forbidden", node.Tag)
	}
	if node.Kind == yaml.MappingNode {
		if len(node.Content)%2 != 0 {
			return errors.New("malformed YAML mapping")
		}
		seen := make(map[string]struct{}, len(node.Content)/2)
		for i := 0; i < len(node.Content); i += 2 {
			key := node.Content[i]
			if key.Kind != yaml.ScalarNode {
				return errors.New("workflow mapping keys must be scalars")
			}
			if _, exists := seen[key.Value]; exists {
				return fmt.Errorf("duplicate YAML key %q", key.Value)
			}
			seen[key.Value] = struct{}{}
		}
	}
	for _, child := range node.Content {
		if err := validateNode(child); err != nil {
			return err
		}
	}
	return nil
}

func verifyUses(node *yaml.Node) error {
	if node.Kind == yaml.MappingNode {
		for i := 0; i < len(node.Content); i += 2 {
			key, value := node.Content[i], node.Content[i+1]
			if key.Value == "uses" {
				if value.Kind != yaml.ScalarNode {
					return errors.New("uses must be a scalar")
				}
				uses := strings.TrimSpace(value.Value)
				if strings.HasPrefix(uses, "./") {
					continue
				}
				if !actionPin.MatchString(uses) {
					return fmt.Errorf("third-party action must use a full lowercase commit SHA: %q", uses)
				}
			}
		}
	}
	for _, child := range node.Content {
		if err := verifyUses(child); err != nil {
			return err
		}
	}
	return nil
}

func mappingValue(node *yaml.Node, key string) (*yaml.Node, bool) {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil, false
	}
	for i := 0; i < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1], true
		}
	}
	return nil, false
}

func requiredMap(node *yaml.Node, key string) (*yaml.Node, error) {
	value, ok := mappingValue(node, key)
	if !ok || value.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s must be a mapping", key)
	}
	return value, nil
}

func requiredSequence(node *yaml.Node, key string) (*yaml.Node, error) {
	value, ok := mappingValue(node, key)
	if !ok || value.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("%s must be a sequence", key)
	}
	return value, nil
}

func scalarValue(node *yaml.Node, key string) string {
	value, ok := mappingValue(node, key)
	if !ok || value.Kind != yaml.ScalarNode {
		return ""
	}
	return value.Value
}
