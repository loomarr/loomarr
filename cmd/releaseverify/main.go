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
	fmt.Println("release-verify: workflows use immutable actions and sign verified digests before tag promotion")
	if err := releaseverify.VerifyNotices(*root); err != nil {
		fmt.Fprintf(os.Stderr, "release-verify: notice policy: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("release-verify: runtime notices are packaged and unresolved redistribution review stays explicit")
}
