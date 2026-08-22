// Command releaseverify checks that the release chain cannot publish something nobody signed.
// `make release-verify` runs it over .github/workflows plus the packaged runtime notices.
//
// It enforces four things the workflow YAML cannot state about itself: actions are pinned to
// immutable references, CI builds the image from the inputs it claims to, the release job runs
// checkout -> source validation -> tag protection -> digest-only build -> Cosign install ->
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

	"github.com/mantonx/loomarr/internal/releaseverify"
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
	if err := releaseverify.VerifyReleaseWorkflow(filepath.Join(*root, ".github", "workflows", "release.yml")); err != nil {
		fmt.Fprintf(os.Stderr, "release-verify: workflow policy: %v\n", err)
		os.Exit(1)
	}
	if err := releaseverify.VerifyAndroidReleaseWorkflow(filepath.Join(*root, ".github", "workflows", "android-beta.yml")); err != nil {
		fmt.Fprintf(os.Stderr, "release-verify: Android workflow policy: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("release-verify: workflows use immutable actions and keep server and Android publication behind audited signing paths")
	if err := releaseverify.VerifyNotices(*root); err != nil {
		fmt.Fprintf(os.Stderr, "release-verify: notice policy: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("release-verify: runtime notices are packaged and unresolved redistribution review stays explicit")
}
