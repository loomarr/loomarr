// Package postgresimage owns the single image reference used by Postgres
// testcontainers and the Make pre-pull that runs before those tests.
package postgresimage

import (
	_ "embed"
	"strings"
)

//go:embed image.txt
var image string

// Name returns the pinned Postgres test image from the Make-readable authority.
func Name() string {
	return strings.TrimSpace(image)
}
