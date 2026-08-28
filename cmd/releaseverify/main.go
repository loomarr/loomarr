// Command releaseverify checks that the release chain cannot publish something nobody signed.
// `make release-verify` runs it over .github/workflows plus the packaged runtime notices.
//
// It enforces the things the workflow YAML cannot state about itself: actions are pinned to
// immutable references, CI builds the image from the inputs it claims to, every pinned asset
// download uses bounded retries, the required aggregate depends on every CI job, the release job
// runs checkout -> source validation -> tag protection -> digest-only build -> Cosign install ->
// sign/verify/promotion IN THAT ORDER, and the redistribution notices are actually packaged.
//
// ⚠ The ordering check is the load-bearing one, and it is a SEQUENCE not a set: promoting a
// tag before the digest is signed, or signing after promotion, both leave a window where the
// published tag points at something unverified. A test that only asserted "all steps present"
// would pass on either.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/loomarr/loomarr/internal/releaseverify"
)

func main() {
	root := flag.String("root", ".", "repository root")
	flag.Parse()

	if err := releaseverify.VerifyActions(filepath.Join(*root, ".github", "workflows")); err != nil {
		fmt.Fprintf(os.Stderr, "release-verify: action policy: %v\n", err)
		os.Exit(1)
	}
	if err := releaseverify.VerifyCIImageInputs(filepath.Join(*root, ".github", "workflows", "ci.yml")); err != nil {
		fmt.Fprintf(os.Stderr, "release-verify: CI image input policy: %v\n", err)
		os.Exit(1)
	}
	if err := releaseverify.VerifyCIAggregate(filepath.Join(*root, ".github", "workflows", "ci.yml")); err != nil {
		fmt.Fprintf(os.Stderr, "release-verify: CI aggregate policy: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("release-verify: CI aggregate includes every top-level job")
	if err := releaseverify.VerifyCIFamilyWorkflows(filepath.Join(*root, ".github", "workflows", "ci.yml")); err != nil {
		fmt.Fprintf(os.Stderr, "release-verify: CI family workflow policy: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("release-verify: CI product jobs are isolated behind owned reusable workflows")
	if err := releaseverify.VerifyCIContainerDownloads(*root); err != nil {
		fmt.Fprintf(os.Stderr, "release-verify: CI container download policy: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("release-verify: CI container acquisition is cached and bounded before product tests")
	if err := releaseverify.VerifyCIImpactActivation(filepath.Join(*root, ".github", "workflows", "ci.yml")); err != nil {
		fmt.Fprintf(os.Stderr, "release-verify: CI impact activation policy: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("release-verify: activated CI gates consume fail-closed specialized decisions")
	if err := releaseverify.VerifyCINativeAdmission(filepath.Join(*root, ".github", "workflows", "ci.yml")); err != nil {
		fmt.Fprintf(os.Stderr, "release-verify: CI native admission policy: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("release-verify: scarce macOS jobs are admitted by merge queue, main, or manual CI")
	if err := releaseverify.VerifyCIManualScopes(filepath.Join(*root, ".github", "workflows", "ci.yml")); err != nil {
		fmt.Fprintf(os.Stderr, "release-verify: CI manual scope policy: %v\n", err)
		os.Exit(1)
	}
	if err := releaseverify.VerifyReleaseWorkflow(filepath.Join(*root, ".github", "workflows", "release.yml")); err != nil {
		fmt.Fprintf(os.Stderr, "release-verify: workflow policy: %v\n", err)
		os.Exit(1)
	}
	if err := releaseverify.VerifyAndroidReleaseWorkflow(filepath.Join(*root, ".github", "workflows", "android-beta.yml")); err != nil {
		fmt.Fprintf(os.Stderr, "release-verify: Android workflow policy: %v\n", err)
		os.Exit(1)
	}
	if err := releaseverify.VerifyReleaseNotesWorkflow(filepath.Join(*root, ".github", "workflows", "release-notes.yml")); err != nil {
		fmt.Fprintf(os.Stderr, "release-verify: release notes policy: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("release-verify: workflows use immutable actions and keep server, Android, and release-note publication behind audited paths")
	if err := releaseverify.VerifyDockerfileDownloads(filepath.Join(*root, "Dockerfile")); err != nil {
		fmt.Fprintf(os.Stderr, "release-verify: Dockerfile download policy: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("release-verify: pinned asset downloads use bounded retries")
	if err := releaseverify.VerifyNotices(*root); err != nil {
		fmt.Fprintf(os.Stderr, "release-verify: notice policy: %v\n", err)
		os.Exit(1)
	}
	if err := releaseverify.VerifyRedistributionManifest(*root); err != nil {
		fmt.Fprintf(os.Stderr, "release-verify: redistribution manifest: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("release-verify: runtime notices are packaged and unresolved redistribution review stays explicit")
}
